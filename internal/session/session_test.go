package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// eventually polls cond every 50ms until it returns true or the timeout
// elapses, failing the test with msg on timeout. No fixed sleeps over 100ms
// granularity anywhere in this suite.
func eventually(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, msg)
}

// spawnForTest spawns a default-options session and guarantees the bash
// process tree is gone when the test finishes, even if the test fails midway.
func spawnForTest(t *testing.T, m *Manager) *Session {
	t.Helper()
	return spawnForTestOpts(t, m, SpawnOpts{})
}

// spawnForTestOpts is spawnForTest with explicit SpawnOpts (cwd / task
// association), sharing the same teardown guarantee.
func spawnForTestOpts(t *testing.T, m *Manager, opts SpawnOpts) *Session {
	t.Helper()
	s, err := m.Spawn(opts)
	if err != nil {
		t.Fatalf("Spawn(%+v): %v", opts, err)
	}
	t.Cleanup(func() {
		// Best-effort teardown: SIGKILL the whole session (leader group plus
		// any job-control groups), then wait for the exit watcher.
		for _, pgid := range sessionPGIDs(s.cmd.Process.Pid) {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-s.Done():
		case <-time.After(5 * time.Second):
			t.Errorf("session %s did not exit during cleanup", s.Info().ID)
		}
	})
	return s
}

// waitForQuiescentSnapshot waits until the ring contains want and output has
// stopped growing (no change for 300ms), then returns the stable snapshot.
func waitForQuiescentSnapshot(t *testing.T, s *Session, want string) []byte {
	t.Helper()
	eventually(t, 5*time.Second, "snapshot to contain "+want, func() bool {
		return bytes.Contains(s.Snapshot(), []byte(want))
	})
	var snap []byte
	eventually(t, 5*time.Second, "output to quiesce", func() bool {
		a := s.Snapshot()
		time.Sleep(100 * time.Millisecond)
		b := s.Snapshot()
		if bytes.Equal(a, b) {
			snap = b
			return true
		}
		return false
	})
	return snap
}

// drainUntil reads from q until the accumulated bytes contain want, the
// channel closes, or the timeout fires. Returns accumulated bytes and whether
// want was seen.
func drainUntil(t *testing.T, q <-chan []byte, want string, timeout time.Duration) ([]byte, bool) {
	t.Helper()
	var acc []byte
	deadline := time.After(timeout)
	for {
		select {
		case chunk, ok := <-q:
			if !ok {
				return acc, bytes.Contains(acc, []byte(want))
			}
			acc = append(acc, chunk...)
			if bytes.Contains(acc, []byte(want)) {
				return acc, true
			}
		case <-deadline:
			return acc, bytes.Contains(acc, []byte(want))
		}
	}
}

func TestSpawnLabelsAndStatus(t *testing.T) {
	m := NewManager()
	s1 := spawnForTest(t, m)
	s2 := spawnForTest(t, m)

	tests := []struct {
		name      string
		s         *Session
		wantLabel string
	}{
		{"first spawn", s1, "bash #1"},
		{"second spawn", s2, "bash #2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tt.s.Info()
			if info.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", info.Label, tt.wantLabel)
			}
			if info.Status != StatusRunning {
				t.Errorf("Status = %q, want %q", info.Status, StatusRunning)
			}
			if info.ID == "" {
				t.Error("ID is empty")
			}
			if info.ExitCode != nil {
				t.Errorf("ExitCode = %v, want nil while running", *info.ExitCode)
			}
		})
	}
	if s1.Info().ID == s2.Info().ID {
		t.Error("two spawns share the same ID")
	}
}

func TestSpawnPumpFillsRing(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	if err := s.WriteInput([]byte("echo kangent-marker\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	eventually(t, 2*time.Second, "snapshot to contain kangent-marker", func() bool {
		return bytes.Contains(s.Snapshot(), []byte("kangent-marker"))
	})
}

func TestAttachReplayFirstThenLive(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	if err := s.WriteInput([]byte("echo kangent-marker\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	snap := waitForQuiescentSnapshot(t, s, "kangent-marker")

	q := s.Attach("conn-1")
	select {
	case first := <-q:
		if !bytes.Equal(first, snap) {
			t.Errorf("first queued item != pre-attach snapshot\nfirst: %q\nsnap:  %q", first, snap)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no replay item queued on attach")
	}

	// Live output arrives on the same channel after the replay.
	if err := s.WriteInput([]byte("echo live-marker\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	if _, ok := drainUntil(t, q, "live-marker", 3*time.Second); !ok {
		t.Error("live output after attach never arrived on the attach channel")
	}
}

func TestAttachAfterExitReturnsReplay(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	if err := s.WriteInput([]byte("echo kangent-marker\nexit\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}

	q := s.Attach("late-conn")
	acc, ok := drainUntil(t, q, "kangent-marker", 2*time.Second)
	if !ok {
		t.Errorf("late attach replay missing marker; got %q", acc)
	}
}

func TestDetachLeavesSessionRunning(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	q := s.Attach("conn-1")
	<-q // replay
	s.Detach("conn-1")

	if got := s.Info().Status; got != StatusRunning {
		t.Fatalf("Status after Detach = %q, want %q (PTY must outlive connections)", got, StatusRunning)
	}
	if err := s.WriteInput([]byte("echo after-detach\n")); err != nil {
		t.Fatalf("WriteInput after Detach: %v", err)
	}
	eventually(t, 2*time.Second, "output after detach to reach ring", func() bool {
		return bytes.Contains(s.Snapshot(), []byte("after-detach"))
	})

	// Detaching an unknown conn must not panic.
	s.Detach("never-attached")
}

func TestExitCapturesCodeAndNotifies(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)
	q := s.Attach("conn-1")

	if err := s.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}

	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done() not closed within 5s of exit")
	}

	info := s.Info()
	if info.Status != StatusExited {
		t.Errorf("Status = %q, want %q", info.Status, StatusExited)
	}
	if info.ExitCode == nil {
		t.Fatal("ExitCode = nil, want 0")
	}
	if *info.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", *info.ExitCode)
	}

	// Attached channel must be closed after the exit notification fires.
	eventuallyClosed := func() bool {
		for {
			select {
			case _, ok := <-q:
				if !ok {
					return true
				}
			default:
				return false
			}
		}
	}
	eventually(t, 2*time.Second, "attach channel to close after exit", eventuallyClosed)

	// WriteInput on an exited session must error.
	if err := s.WriteInput([]byte("echo nope\n")); err == nil {
		t.Error("WriteInput after exit returned nil error")
	}
}

func TestRemoveSemantics(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)
	id := s.Info().ID

	if err := m.Remove(id); !errors.Is(err, ErrStillRunning) {
		t.Errorf("Remove(running) = %v, want ErrStillRunning", err)
	}
	if _, ok := m.Get(id); !ok {
		t.Fatal("Get lost the session after rejected Remove")
	}

	if err := s.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}

	if err := m.Remove(id); err != nil {
		t.Fatalf("Remove(exited) = %v, want nil", err)
	}
	if _, ok := m.Get(id); ok {
		t.Error("session still present after Remove")
	}
	for _, info := range m.List() {
		if info.ID == id {
			t.Error("removed session still in List()")
		}
	}
}

// setTestGrace shortens the SIGTERM→SIGKILL grace window so escalation tests
// stay well under the suite timeout. Must be called before Stop.
func setTestGrace(s *Session, d time.Duration) {
	s.mu.Lock()
	s.termGrace = d
	s.mu.Unlock()
}

// waitForSessionGroups polls until the shell's session contains at least n
// distinct process groups. Interactive bash runs every pipeline (foreground
// or background) in its own group, so a spawned `sleep` shows up as a second
// group in the session.
func waitForSessionGroups(t *testing.T, s *Session, n int) {
	t.Helper()
	eventually(t, 5*time.Second, "session to contain child process group", func() bool {
		return len(sessionPGIDs(s.cmd.Process.Pid)) >= n
	})
}

// TestStopZeroDescendants is THE TERM-06 test: Stop must terminate the entire
// process tree, including background children like `sleep 300 &`.
func TestStopZeroDescendants(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)
	setTestGrace(s, 200*time.Millisecond)
	pgid := s.cmd.Process.Pid // session leader => PGID == PID

	if err := s.WriteInput([]byte("sleep 300 &\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	waitForSessionGroups(t, s, 2)

	s.Stop()

	// Leader's process group must be entirely gone.
	if err := syscall.Kill(-pgid, 0); err != syscall.ESRCH {
		t.Errorf("Kill(-pgid, 0) = %v, want ESRCH (group must be empty)", err)
	}
	// And the whole session — including the background sleep's own process
	// group — must be empty (zero orphaned subprocesses).
	eventually(t, 2*time.Second, "session to have zero surviving processes", func() bool {
		return len(sessionPGIDs(pgid)) == 0
	})

	info := s.Info()
	if info.Status != StatusExited {
		t.Errorf("Status after Stop = %q, want %q", info.Status, StatusExited)
	}
	if info.ExitCode == nil || *info.ExitCode != 137 {
		t.Errorf("ExitCode after SIGKILL = %v, want 137 (128+SIGKILL)", info.ExitCode)
	}
}

func TestStopOnExitedSessionIsIdempotent(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	if err := s.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}

	// Stop on an exited session must return immediately, repeatedly, and
	// concurrently, without panicking or hanging.
	for i := 0; i < 2; i++ {
		done := make(chan struct{})
		go func() {
			s.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Stop on exited session did not return immediately")
		}
	}
}

func TestStopEscalatesToSigkill(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)
	setTestGrace(s, 200*time.Millisecond)
	pgid := s.cmd.Process.Pid

	// Interactive bash already ignores SIGTERM; trap '' TERM makes the child
	// sleep inherit SIG_IGN too, so NOTHING in the tree dies from SIGTERM.
	if err := s.WriteInput([]byte("trap '' TERM; sleep 300\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	waitForSessionGroups(t, s, 2)

	start := time.Now()
	s.Stop()
	elapsed := time.Since(start)

	if elapsed < 200*time.Millisecond {
		t.Errorf("Stop returned in %s, before the grace window — SIGTERM cannot have killed a trap-protected tree", elapsed)
	}
	info := s.Info()
	if info.Status != StatusExited {
		t.Errorf("Status = %q, want %q", info.Status, StatusExited)
	}
	if info.ExitCode == nil || *info.ExitCode != 137 {
		t.Errorf("ExitCode = %v, want 137 (proves SIGKILL escalation)", info.ExitCode)
	}
	eventually(t, 2*time.Second, "trap-protected tree to be fully gone", func() bool {
		return len(sessionPGIDs(pgid)) == 0
	})
}

func TestStopConcurrentCallsAllReturn(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)
	setTestGrace(s, 200*time.Millisecond)

	results := make(chan struct{}, 2)
	go func() { s.Stop(); results <- struct{}{} }()
	go func() { s.Stop(); results <- struct{}{} }()
	for i := 0; i < 2; i++ {
		select {
		case <-results:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent Stop call never returned")
		}
	}
}

func TestResizeAndJiggle(t *testing.T) {
	m := NewManager()
	s := spawnForTest(t, m)

	calls := func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.setsizeCalls
	}
	size := func() (uint16, uint16) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lastWinsize.Cols, s.lastWinsize.Rows
	}

	steps := []struct {
		name        string
		cols, rows  uint16
		forceRedraw bool
		wantDelta   int // expected change in recorded Setsize call count
	}{
		{"real size change", 100, 40, false, 1},
		{"same size without force is a no-op", 100, 40, false, 0},
		{"same size with force jiggles (rows-1 then rows)", 100, 40, true, 2},
		{"changed size with force is a single set", 120, 50, true, 1},
	}
	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			before := calls()
			if err := s.Resize(tt.cols, tt.rows, tt.forceRedraw); err != nil {
				t.Fatalf("Resize(%d, %d, %v): %v", tt.cols, tt.rows, tt.forceRedraw, err)
			}
			if got := calls() - before; got != tt.wantDelta {
				t.Errorf("setsize calls delta = %d, want %d", got, tt.wantDelta)
			}
			gotCols, gotRows := size()
			if gotCols != tt.cols || gotRows != tt.rows {
				t.Errorf("lastWinsize = %dx%d, want %dx%d", gotCols, gotRows, tt.cols, tt.rows)
			}
		})
	}
}

func TestListNewestFirst(t *testing.T) {
	m := NewManager()
	s1 := spawnForTest(t, m)
	s2 := spawnForTest(t, m)

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	if list[0].ID != s2.Info().ID || list[1].ID != s1.Info().ID {
		t.Errorf("List order = [%s, %s], want newest first [%s, %s]",
			list[0].Label, list[1].Label, s2.Info().Label, s1.Info().Label)
	}
}

// TestTaskScopedLabels covers the UI-SPEC label contract: task-scoped
// sessions get "Bash 1", "Bash 2"... per task, monotonic and never reused
// even after stops; task-less sessions keep the global "bash #N" counter.
func TestTaskScopedLabels(t *testing.T) {
	m := NewManager()

	s1 := spawnForTestOpts(t, m, SpawnOpts{TaskID: 7})
	if got := s1.Info(); got.Label != "Bash 1" || got.TaskID != 7 {
		t.Errorf("task 7 first spawn: Label=%q TaskID=%d, want Label=%q TaskID=7", got.Label, got.TaskID, "Bash 1")
	}

	s2 := spawnForTestOpts(t, m, SpawnOpts{TaskID: 7})
	if got := s2.Info().Label; got != "Bash 2" {
		t.Errorf("task 7 second spawn: Label=%q, want %q", got, "Bash 2")
	}

	s3 := spawnForTestOpts(t, m, SpawnOpts{TaskID: 8})
	if got := s3.Info(); got.Label != "Bash 1" || got.TaskID != 8 {
		t.Errorf("task 8 first spawn: Label=%q TaskID=%d, want Label=%q TaskID=8 (counters are per-task)", got.Label, got.TaskID, "Bash 1")
	}

	// Stop task 7's "Bash 1": the counter is monotonic, never reused —
	// the next task-7 spawn must be "Bash 3", not "Bash 1".
	setTestGrace(s1, 200*time.Millisecond)
	s1.Stop()
	s4 := spawnForTestOpts(t, m, SpawnOpts{TaskID: 7})
	if got := s4.Info().Label; got != "Bash 3" {
		t.Errorf("task 7 spawn after stop: Label=%q, want %q (counter never reused)", got, "Bash 3")
	}

	// The global dev counter is independent of task counters.
	dev := spawnForTest(t, m)
	if got := dev.Info(); got.Label != "bash #1" || got.TaskID != 0 {
		t.Errorf("dev spawn: Label=%q TaskID=%d, want Label=%q TaskID=0", got.Label, got.TaskID, "bash #1")
	}
}

// TestSpawnCwd proves the shell really starts in the requested directory.
// The mar''ker quote-split means the echoed COMMAND can never satisfy the
// assertion — only the shell's expansion of $PWD can.
func TestSpawnCwd(t *testing.T) {
	dir := t.TempDir()
	// bash reports the physical cwd (getcwd); t.TempDir may sit behind a
	// symlink (e.g. /tmp on some hosts), so compare against the resolved path.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}

	m := NewManager()
	s := spawnForTestOpts(t, m, SpawnOpts{Cwd: dir, TaskID: 1})

	if err := s.WriteInput([]byte("echo mar''ker:$PWD\r")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	want := "marker:" + resolved
	eventually(t, 5*time.Second, "snapshot to contain "+want, func() bool {
		return bytes.Contains(s.Snapshot(), []byte(want))
	})
}

// TestSpawnInvalidCwdFailsBeforeRegistration: a bad cwd (deleted worktree)
// must produce a clean error mentioning the path BEFORE any PTY is allocated
// or session registered — not a confusing shell error.
func TestSpawnInvalidCwdFailsBeforeRegistration(t *testing.T) {
	m := NewManager()
	_ = spawnForTest(t, m) // pre-existing session; List length must not change

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"nonexistent path", "/nonexistent/path-xyz"},
		{"path is a file", file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(m.List())
			_, err := m.Spawn(SpawnOpts{Cwd: tt.cwd, TaskID: 3})
			if err == nil {
				t.Fatalf("Spawn(Cwd=%q) = nil error, want failure", tt.cwd)
			}
			if !strings.Contains(err.Error(), tt.cwd) {
				t.Errorf("error %q does not mention the path %q", err, tt.cwd)
			}
			if after := len(m.List()); after != before {
				t.Errorf("List len changed %d -> %d: failed spawn must not register a session", before, after)
			}
		})
	}
}

// TestListByTask: filtering by task, newest first; ListByTask(0) returns
// only unscoped dev sessions.
func TestListByTask(t *testing.T) {
	m := NewManager()
	a := spawnForTestOpts(t, m, SpawnOpts{TaskID: 7})
	b := spawnForTestOpts(t, m, SpawnOpts{TaskID: 7})
	_ = spawnForTestOpts(t, m, SpawnOpts{TaskID: 8})
	d := spawnForTest(t, m) // unscoped

	task7 := m.ListByTask(7)
	if len(task7) != 2 {
		t.Fatalf("ListByTask(7) len = %d, want 2", len(task7))
	}
	if task7[0].ID != b.Info().ID || task7[1].ID != a.Info().ID {
		t.Errorf("ListByTask(7) order = [%s, %s], want newest first [%s, %s]",
			task7[0].Label, task7[1].Label, b.Info().Label, a.Info().Label)
	}
	for _, info := range task7 {
		if info.TaskID != 7 {
			t.Errorf("ListByTask(7) returned session with TaskID=%d", info.TaskID)
		}
	}

	dev := m.ListByTask(0)
	if len(dev) != 1 || dev[0].ID != d.Info().ID {
		t.Errorf("ListByTask(0) = %v, want exactly the unscoped session %s", dev, d.Info().ID)
	}

	if got := m.ListByTask(99); len(got) != 0 {
		t.Errorf("ListByTask(99) len = %d, want 0", len(got))
	}
}

// TestStopAllForTask: stops every running session of exactly the given task,
// blocking until all have exited; other tasks' sessions are untouched.
// Repeated calls are safe (Stop is idempotent).
func TestStopAllForTask(t *testing.T) {
	m := NewManager()
	a := spawnForTestOpts(t, m, SpawnOpts{TaskID: 5})
	b := spawnForTestOpts(t, m, SpawnOpts{TaskID: 5})
	other := spawnForTestOpts(t, m, SpawnOpts{TaskID: 6})
	// Interactive bash ignores SIGTERM, so each Stop consumes the full grace
	// window — shorten it to keep the test fast (Phase 2 convention).
	setTestGrace(a, 200*time.Millisecond)
	setTestGrace(b, 200*time.Millisecond)

	m.StopAllForTask(5)

	// StopAllForTask blocks until done: statuses must be final on return.
	if got := a.Info().Status; got != StatusExited {
		t.Errorf("task-5 session a Status = %q after StopAllForTask, want %q", got, StatusExited)
	}
	if got := b.Info().Status; got != StatusExited {
		t.Errorf("task-5 session b Status = %q after StopAllForTask, want %q", got, StatusExited)
	}
	if got := other.Info().Status; got != StatusRunning {
		t.Errorf("task-6 session Status = %q, want %q (non-matching task must be untouched)", got, StatusRunning)
	}

	// Calling again on the same (now fully exited) task is safe and fast.
	done := make(chan struct{})
	go func() {
		m.StopAllForTask(5)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second StopAllForTask(5) did not return promptly")
	}
}

// TestStopAllForTaskNoSessions: a task with no sessions returns immediately
// with no panic and no block.
func TestStopAllForTaskNoSessions(t *testing.T) {
	m := NewManager()
	_ = spawnForTestOpts(t, m, SpawnOpts{TaskID: 6}) // unrelated task

	done := make(chan struct{})
	go func() {
		m.StopAllForTask(99)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopAllForTask on a session-less task did not return immediately")
	}
}

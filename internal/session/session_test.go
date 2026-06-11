package session

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
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
// The mar”ker quote-split means the echoed COMMAND can never satisfy the
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

// ---------------------------------------------------------------------------
// Agent status state machine (D-47): working / idle / waiting
// ---------------------------------------------------------------------------

// spawnAgentForTest spawns an agent session against a fresh fake-claude stub
// with the standard test AgentConfig and teardown guarantee.
func spawnAgentForTest(t *testing.T, m *Manager) *Session {
	t.Helper()
	stub := writeFakeClaude(t, filepath.Join(t.TempDir(), "args"))
	m.SetAgentConfig(testAgentConfig(stub))
	return spawnForTestOpts(t, m, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1})
}

// injectOutputForTest routes chunk through the exact locked mutations the
// pump performs for agent output (BEL scan + settle-gated activity), without
// the flakiness of driving bytes through the real PTY.
func injectOutputForTest(s *Session, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteAgentOutputLocked(chunk)
}

// setLastActivityForTest overrides the activity timestamp (quiet-threshold
// manipulation).
func setLastActivityForTest(s *Session, ts time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = ts
}

// setStopHookAtForTest moves the settle-window anchor (so tests need not
// sleep through the real 2s window).
func setStopHookAtForTest(s *Session, ts time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopHookAt = ts
}

func agentStatus(s *Session) string { return s.Info().AgentStatus }

// TestAgentStatusFreshIsWorking: spawn -> working (lastActivity set at spawn,
// startup output flows immediately).
func TestAgentStatusFreshIsWorking(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)
	if got := agentStatus(s); got != "working" {
		t.Errorf("fresh agent AgentStatus = %q, want %q", got, "working")
	}
}

// TestAgentWaitingIsSticky: Notification -> waiting; output (prompt redraws)
// must NEVER clear it; only a Stop hook (SetIdle) does here.
func TestAgentWaitingIsSticky(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)

	s.SetWaiting()
	if got := agentStatus(s); got != "waiting" {
		t.Fatalf("after SetWaiting AgentStatus = %q, want %q", got, "waiting")
	}

	injectOutputForTest(s, []byte("permission prompt redraw"))
	injectOutputForTest(s, []byte("spinner frame"))
	if got := agentStatus(s); got != "waiting" {
		t.Errorf("after output AgentStatus = %q, want %q (waiting is sticky)", got, "waiting")
	}

	s.SetIdle()
	if got := agentStatus(s); got != "idle" {
		t.Errorf("after SetIdle AgentStatus = %q, want %q", got, "idle")
	}
}

// TestAgentSettleWindow: SetIdle is immediate even with recent output, and
// output inside the 2s settle window must not flip idle -> working (Pitfall 1
// — claude paints the final response around the Stop hook). Output after the
// window does flip to working.
func TestAgentSettleWindow(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)
	waitForQuiescentSnapshot(t, s, "fake claude ready") // startup output done

	// Output arrived 1s ago — SetIdle must still flip to idle IMMEDIATELY.
	setLastActivityForTest(s, time.Now().Add(-time.Second))
	s.SetIdle()
	if got := agentStatus(s); got != "idle" {
		t.Fatalf("right after SetIdle AgentStatus = %q, want %q", got, "idle")
	}

	// Output within the settle window must NOT flip to working.
	injectOutputForTest(s, []byte("final response paint"))
	if got := agentStatus(s); got != "idle" {
		t.Errorf("output within settle window: AgentStatus = %q, want %q", got, "idle")
	}

	// Move the settle anchor past the window: output now flips to working.
	setStopHookAtForTest(s, time.Now().Add(-3*time.Second))
	injectOutputForTest(s, []byte("new turn output"))
	if got := agentStatus(s); got != "working" {
		t.Errorf("output after settle window: AgentStatus = %q, want %q", got, "working")
	}
}

// TestAgentStdinClearsWaiting: the user typing into a waiting agent answers
// the prompt — waiting clears and the session is working.
func TestAgentStdinClearsWaiting(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)

	s.SetWaiting()
	if err := s.WriteInput([]byte("y")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	if got := agentStatus(s); got != "working" {
		t.Errorf("after stdin AgentStatus = %q, want %q", got, "working")
	}
}

// TestClearWaitingOnAttach: attaching to a waiting agent clears it to idle
// (D-45); bash sessions are a strict no-op.
func TestClearWaitingOnAttach(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)

	s.SetWaiting()
	s.ClearWaitingOnAttach()
	if got := agentStatus(s); got != "idle" {
		t.Errorf("after ClearWaitingOnAttach AgentStatus = %q, want %q", got, "idle")
	}

	// A non-waiting agent is untouched (working stays working).
	s2 := spawnAgentForTest(t, m)
	s2.ClearWaitingOnAttach()
	if got := agentStatus(s2); got != "working" {
		t.Errorf("non-waiting agent after ClearWaitingOnAttach = %q, want %q", got, "working")
	}

	// Bash: no-op, no panic, no agent status.
	b := spawnForTest(t, m)
	b.ClearWaitingOnAttach()
	if got := b.Info().AgentStatus; got != "" {
		t.Errorf("bash AgentStatus = %q, want empty", got)
	}
}

// TestAgentBelFallbackGating: a bare BEL marks waiting ONLY in hooks-dead
// mode (no SessionStart received); OSC-terminated BELs never count (Pitfall 2
// + research anti-pattern).
func TestAgentBelFallbackGating(t *testing.T) {
	m := NewManager()

	t.Run("hooks dead: bare BEL -> waiting", func(t *testing.T) {
		s := spawnAgentForTest(t, m)
		injectOutputForTest(s, []byte{0x07})
		if got := agentStatus(s); got != "waiting" {
			t.Errorf("AgentStatus = %q, want %q", got, "waiting")
		}
	})

	t.Run("hooks alive: BEL ignored", func(t *testing.T) {
		s := spawnAgentForTest(t, m)
		s.MarkHooksAlive()
		injectOutputForTest(s, []byte{0x07})
		if got := agentStatus(s); got == "waiting" {
			t.Errorf("AgentStatus = %q; hooks-alive BEL must never mark waiting", got)
		}
	})

	t.Run("OSC-terminated BEL never triggers", func(t *testing.T) {
		s := spawnAgentForTest(t, m)
		injectOutputForTest(s, []byte("\x1b]0;title\x07"))
		if got := agentStatus(s); got == "waiting" {
			t.Errorf("AgentStatus = %q; OSC terminator BEL must not mark waiting (hooks dead)", got)
		}
		s.MarkHooksAlive()
		injectOutputForTest(s, []byte("\x1b]0;title\x07"))
		if got := agentStatus(s); got == "waiting" {
			t.Errorf("AgentStatus = %q; OSC terminator BEL must not mark waiting (hooks alive)", got)
		}
	})
}

// TestAgentStopRequestedDistinguishesExit: server-initiated Stop records
// stopRequested so exit 143 can render gray; a natural exit stays false
// (red stays reserved for exits Kangent didn't ask for).
func TestAgentStopRequestedDistinguishesExit(t *testing.T) {
	m := NewManager()

	s := spawnAgentForTest(t, m)
	setTestGrace(s, 200*time.Millisecond)
	s.Stop()
	info := s.Info()
	if !info.StopRequested {
		t.Error("StopRequested = false after server Stop, want true")
	}
	if info.AgentStatus != "exited" {
		t.Errorf("AgentStatus = %q after Stop, want %q", info.AgentStatus, "exited")
	}
	if info.ExitCode == nil || *info.ExitCode != 143 {
		t.Errorf("ExitCode = %v, want 143 (stub traps TERM)", info.ExitCode)
	}

	// Natural exit: a stub that exits on its own.
	exitStub := filepath.Join(t.TempDir(), "fake-claude-exits")
	if err := os.WriteFile(exitStub, []byte("#!/usr/bin/env bash\necho done\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write exiting stub: %v", err)
	}
	m.SetAgentConfig(testAgentConfig(exitStub))
	s2 := spawnForTestOpts(t, m, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 2})
	select {
	case <-s2.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("exiting stub never exited")
	}
	info2 := s2.Info()
	if info2.StopRequested {
		t.Error("StopRequested = true after natural exit, want false")
	}
	if info2.AgentStatus != "exited" {
		t.Errorf("AgentStatus = %q after natural exit, want %q", info2.AgentStatus, "exited")
	}
}

// TestAgentQuietThresholdIdle: no activity for longer than the 10s threshold
// computes idle (lazy computation, no ticker).
func TestAgentQuietThresholdIdle(t *testing.T) {
	m := NewManager()
	s := spawnAgentForTest(t, m)
	waitForQuiescentSnapshot(t, s, "fake claude ready") // startup output done

	setLastActivityForTest(s, time.Now().Add(-(agentQuietThreshold + time.Second)))
	if got := agentStatus(s); got != "idle" {
		t.Errorf("AgentStatus = %q after >10s quiet, want %q", got, "idle")
	}
}

// ---------------------------------------------------------------------------
// Settings-driven spawn (Phase 6): SpawnOpts.Shell + SpawnOpts.ExtraArgs
// ---------------------------------------------------------------------------

// shellEnvOf returns the value of the child's SHELL= env entry.
func shellEnvOf(s *Session) string {
	for _, e := range s.cmd.Env {
		if strings.HasPrefix(e, "SHELL=") {
			return strings.TrimPrefix(e, "SHELL=")
		}
	}
	return ""
}

// TestSpawnBashShellFromSettings: a non-empty SpawnOpts.Shell is resolved via
// LookPath; the resolved path is BOTH the exec'd binary and the SHELL env
// entry (SHELL-02 — the settings value drives the spawn, not $SHELL).
func TestSpawnBashShellFromSettings(t *testing.T) {
	resolved, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not on PATH: %v", err)
	}

	m := NewManager()
	s := spawnForTestOpts(t, m, SpawnOpts{Shell: "bash"})

	if got := s.cmd.Path; got != resolved {
		t.Errorf("exec path = %q, want LookPath-resolved %q", got, resolved)
	}
	if got := shellEnvOf(s); got != resolved {
		t.Errorf("SHELL env = %q, want the resolved path %q", got, resolved)
	}
}

// TestSpawnBashShellEmptyKeepsFallback: Shell == "" preserves the Phase 2
// $SHELL-fallback path byte-for-byte (back-compat for direct-Spawn callers).
func TestSpawnBashShellEmptyKeepsFallback(t *testing.T) {
	want := os.Getenv("SHELL")
	if want == "" {
		want = "/bin/bash"
	}

	m := NewManager()
	s := spawnForTest(t, m) // zero-value Shell

	if got := shellEnvOf(s); got != want {
		t.Errorf("SHELL env = %q, want the $SHELL fallback %q", got, want)
	}
}

// TestSpawnBashShellNotFound: an unresolvable shell fails cleanly BEFORE any
// PTY allocation and never registers a session (same posture as bad cwds).
func TestSpawnBashShellNotFound(t *testing.T) {
	m := NewManager()
	_ = spawnForTest(t, m) // pre-existing session; List length must not change

	before := len(m.List())
	_, err := m.Spawn(SpawnOpts{Shell: "no-such-shell-xyz"})
	if err == nil {
		t.Fatal("Spawn(Shell=no-such-shell-xyz) = nil error, want failure")
	}
	if !strings.Contains(err.Error(), "no-such-shell-xyz") {
		t.Errorf("error %q does not mention the shell name", err)
	}
	if after := len(m.List()); after != before {
		t.Errorf("List len changed %d -> %d: failed spawn must not register a session", before, after)
	}
}

// TestSpawnAgentExtraArgsAppendedAfterFixedFlags: ExtraArgs land strictly
// AFTER the fixed --session-id/--settings flags, in order (AGENT-01; insertion
// point verified composing on claude v2.1.173).
func TestSpawnAgentExtraArgsAppendedAfterFixedFlags(t *testing.T) {
	extras := []string{"--dangerously-skip-permissions", "--append-system-prompt", "be terse"}
	_, args := agentArgv(t, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1, ExtraArgs: extras})

	if len(args) != 4+len(extras) {
		t.Fatalf("argv = %q, want %d args (4 fixed + %d extras)", args, 4+len(extras), len(extras))
	}
	if args[0] != "--session-id" {
		t.Errorf("argv[0] = %q, want --session-id", args[0])
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	for i, want := range extras {
		if args[4+i] != want {
			t.Errorf("argv[%d] = %q, want extra %q (extras must follow the fixed flags in order)", 4+i, args[4+i], want)
		}
	}
}

// TestSpawnAgentResumeExtraArgsAppended: the --resume variant carries the same
// extras after its fixed flags — AGENT-01 says EVERY claude spawn.
func TestSpawnAgentResumeExtraArgsAppended(t *testing.T) {
	const resumeID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0002"
	_, args := agentArgv(t, SpawnOpts{
		Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1,
		ResumeSessionID: resumeID,
		ExtraArgs:       []string{"--dangerously-skip-permissions"},
	})

	if len(args) != 5 {
		t.Fatalf("argv = %q, want 5 args (--resume <uuid> --settings <json> <extra>)", args)
	}
	if args[0] != "--resume" {
		t.Errorf("argv[0] = %q, want --resume", args[0])
	}
	if args[1] != resumeID {
		t.Errorf("argv[1] = %q, want the resume uuid %q", args[1], resumeID)
	}
	if args[2] != "--settings" {
		t.Errorf("argv[2] = %q, want --settings", args[2])
	}
	if args[4] != "--dangerously-skip-permissions" {
		t.Errorf("argv[4] = %q, want the extra after the fixed flags", args[4])
	}
}

// TestSpawnAgentNoExtraArgsUnchanged: nil ExtraArgs keeps the exact 4-arg v1.0
// argv — a stored empty settings value must restore interactive prompts
// (AGENT-02 removability).
func TestSpawnAgentNoExtraArgsUnchanged(t *testing.T) {
	_, args := agentArgv(t, SpawnOpts{Kind: KindAgent, Cwd: t.TempDir(), TaskID: 1})

	if len(args) != 4 {
		t.Fatalf("argv = %q, want exactly 4 args with no extras", args)
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

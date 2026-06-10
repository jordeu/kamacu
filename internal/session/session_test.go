package session

import (
	"bytes"
	"errors"
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

// spawnForTest spawns a session and guarantees the bash process tree is gone
// when the test finishes, even if the test fails midway.
func spawnForTest(t *testing.T, m *Manager) *Session {
	t.Helper()
	s, err := m.Spawn()
	if err != nil {
		t.Fatalf("Spawn: %v", err)
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

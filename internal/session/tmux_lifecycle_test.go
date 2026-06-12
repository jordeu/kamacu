package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"kangent/internal/tmux"
)

// ---------------------------------------------------------------------------
// tmux lifecycle (Phase 8, plan 03): TMUX-04 kill, TMUX-06 exit-vs-detach,
// TMUX-07 nil-killer regression guard, D-84 honest tmux-missing error.
//
// Integration tests run against REAL tmux on per-test sockets (never the
// production "kangent" socket) and skip when tmux is not on PATH — the same
// posture as internal/tmux's tests.
// ---------------------------------------------------------------------------

// newTestTmuxClient returns a tmux.Client on a unique per-test socket and
// registers a kill-server cleanup BEFORE any spawn, so the server dies after
// the session's own spawn cleanup runs (t.Cleanup is LIFO). Skips when tmux
// is not installed.
func newTestTmuxClient(t *testing.T) tmux.Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	sock := fmt.Sprintf("ktest-s-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	c := tmux.Client{Socket: sock, ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	return c
}

// spawnTmuxForTest spawns a tmux-backed bash tab on the given client's socket
// with the standard teardown guarantee, then waits for the attach client to
// paint something (the inner shell is up) before returning.
func spawnTmuxForTest(t *testing.T, m *Manager, c tmux.Client, name string) *Session {
	t.Helper()
	m.SetTmuxClient(c)
	s := spawnForTestOpts(t, m, SpawnOpts{TaskID: 1, Cwd: t.TempDir(), TmuxName: name})
	// Readiness: poll for ANY output rather than sleeping blind — tmux drew
	// the pane, so the inner shell exists and input will be delivered.
	eventually(t, 10*time.Second, "tmux attach client to produce output", func() bool {
		return len(bytes.TrimSpace(s.Snapshot())) > 0
	})
	return s
}

// awaitDone waits for the session's exit watcher with a generous deadline
// (tmux server start + shell init can take a moment).
func awaitDone(t *testing.T, s *Session, timeout time.Duration) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(timeout):
		t.Fatal("session did not exit within deadline")
	}
}

// TestTmuxRegressionNilKiller is THE TMUX-07 regression guard: plain bash and
// agent spawns must get a nil killer and no tmux name — their Stop path is the
// byte-identical pre-Phase-8 signal path. No tmux required: this asserts the
// DEFAULT, so it runs everywhere.
func TestTmuxRegressionNilKiller(t *testing.T) {
	m := NewManager()

	t.Run("bash default shell", func(t *testing.T) {
		s := spawnForTest(t, m) // Shell ""
		if s.killer != nil {
			t.Error("plain bash session got a non-nil killer; Stop path must be the default signal path")
		}
		if s.tmuxName != "" {
			t.Errorf("plain bash session tmuxName = %q, want empty", s.tmuxName)
		}
		if s.tmuxClient != nil {
			t.Error("plain bash session got a non-nil tmuxClient")
		}
	})

	t.Run("bash settings shell", func(t *testing.T) {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skipf("bash not on PATH: %v", err)
		}
		s := spawnForTestOpts(t, m, SpawnOpts{Shell: "bash"})
		if s.killer != nil {
			t.Error("settings-shell bash session got a non-nil killer")
		}
		if s.tmuxName != "" {
			t.Errorf("settings-shell bash session tmuxName = %q, want empty", s.tmuxName)
		}
	})

	t.Run("agent", func(t *testing.T) {
		s := spawnAgentForTest(t, m) // fake-claude stub harness from agent tests
		if s.killer != nil {
			t.Error("agent session got a non-nil killer; agents never run under tmux")
		}
		if s.tmuxName != "" {
			t.Errorf("agent session tmuxName = %q, want empty", s.tmuxName)
		}
	})
}

// TestTmuxStopKillsSession is TMUX-04: Stop on a tmux-backed session kills the
// tmux session ITSELF (kill-session), not just the attach client — has-session
// must say dead afterward.
func TestTmuxStopKillsSession(t *testing.T) {
	c := newTestTmuxClient(t)
	m := NewManager()
	s := spawnTmuxForTest(t, m, c, "kangent-1-1")

	if got := s.Info().Status; got != StatusRunning {
		t.Fatalf("Status before Stop = %q, want %q", got, StatusRunning)
	}
	if s.killer == nil {
		t.Fatal("tmux-backed session has a nil killer; Stop would only signal the attach client")
	}

	s.Stop()

	info := s.Info()
	if info.Status != StatusExited {
		t.Errorf("Status after Stop = %q, want %q", info.Status, StatusExited)
	}
	if !info.StopRequested {
		t.Error("StopRequested = false after server Stop, want true")
	}
	alive, err := c.HasSession(context.Background(), "kangent-1-1")
	if err != nil {
		t.Fatalf("HasSession after Stop: %v", err)
	}
	if alive {
		t.Error("tmux session still alive after Stop — kill-session must remove the session, not just the client")
	}
}

// TestTmuxInnerExit is TMUX-06's exit half: the shell exiting INSIDE tmux ends
// the tmux session, the attach client exits, and the session flows through the
// existing waitExit→markExited path — with detachedAlive false.
func TestTmuxInnerExit(t *testing.T) {
	c := newTestTmuxClient(t)
	m := NewManager()
	s := spawnTmuxForTest(t, m, c, "kangent-1-2")

	if err := s.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	awaitDone(t, s, 15*time.Second)

	if got := s.Info().Status; got != StatusExited {
		t.Errorf("Status after inner exit = %q, want %q", got, StatusExited)
	}
	if s.DetachedAlive() {
		t.Error("DetachedAlive = true after inner-shell exit, want false (the session really ended)")
	}
	alive, err := c.HasSession(context.Background(), "kangent-1-2")
	if err != nil {
		t.Fatalf("HasSession after inner exit: %v", err)
	}
	if alive {
		t.Error("tmux session still alive after inner-shell exit")
	}
}

// TestTmuxDetachAlive is TMUX-06's detach half (Open Q1): a manual detach
// makes the attach client exit while has-session stays alive — recorded as
// detachedAlive and shown as the HONEST exited state in Phase 8, never a
// silent disappearance, never an auto-reattach.
func TestTmuxDetachAlive(t *testing.T) {
	c := newTestTmuxClient(t)
	m := NewManager()
	s := spawnTmuxForTest(t, m, c, "kangent-1-3")

	if err := exec.Command("tmux", "-L", c.Socket, "-f", "/dev/null",
		"detach-client", "-s", "=kangent-1-3").Run(); err != nil {
		t.Fatalf("detach-client: %v", err)
	}
	awaitDone(t, s, 15*time.Second)

	if got := s.Info().Status; got != StatusExited {
		t.Errorf("Status after detach = %q, want %q (honest exited banner)", got, StatusExited)
	}
	if !s.DetachedAlive() {
		t.Error("DetachedAlive = false after manual detach, want true (Phase 9 resume reconcile reads this)")
	}
	alive, err := c.HasSession(context.Background(), "kangent-1-3")
	if err != nil {
		t.Fatalf("HasSession after detach: %v", err)
	}
	if !alive {
		t.Error("tmux session dead after detach-client — it must survive the attach client")
	}
	// The per-test KillServer cleanup reaps the still-alive session.
}

// TestTmuxSpawnMissingBinary is the D-84 seam: with tmux gone from PATH, a
// tmux-backed Spawn fails with ErrTmuxNotFound BEFORE any PTY allocation and
// never registers a session. Uses t.Setenv, so no t.Parallel().
func TestTmuxSpawnMissingBinary(t *testing.T) {
	m := NewManager()
	m.SetTmuxClient(tmux.Client{Socket: "ktest-unused", ConfPath: "/dev/null"})
	t.Setenv("PATH", t.TempDir()) // empty dir: LookPath("tmux") must fail

	_, err := m.Spawn(SpawnOpts{TaskID: 1, Cwd: t.TempDir(), TmuxName: "x"})
	if !errors.Is(err, ErrTmuxNotFound) {
		t.Fatalf("Spawn with tmux off PATH = %v, want ErrTmuxNotFound", err)
	}
	if got := len(m.List()); got != 0 {
		t.Errorf("List len = %d after failed spawn, want 0 (no session registered)", got)
	}
}

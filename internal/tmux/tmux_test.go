package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// newTestClient returns a Client on a unique per-test socket, skipping the
// test when tmux is unavailable (Pitfall 8). KillServer cleanup is registered
// BEFORE any session can be created so no test leaks a tmux server.
func newTestClient(t *testing.T) Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	socket := fmt.Sprintf("ktest-%d-%s", os.Getpid(), t.Name())
	c := Client{Socket: socket, ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	return c
}

// newDetachedSession creates a session directly with -d: tests have no tty,
// so production's `new-session -A` (no -d) would fail "open terminal failed:
// not a terminal" (Empirical Finding 1). Production never uses -d.
func newDetachedSession(t *testing.T, c Client, name string) {
	t.Helper()
	args := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", t.TempDir())
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("create detached session %q: %v: %s", name, err, out)
	}
}

// --- Pure-function tests (no tmux needed, run unconditionally) ---

func TestWriteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kangent-tmux.conf")
	if err := WriteConfig(path); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != Config {
		t.Errorf("config content mismatch:\ngot:  %q\nwant: %q", got, Config)
	}
	// Overwrites a previous file.
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(path); err != nil {
		t.Fatalf("WriteConfig overwrite: %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != Config {
		t.Errorf("overwrite left stale content: %q", got)
	}
}

func TestBaseArgs(t *testing.T) {
	c := Client{Socket: "sock", ConfPath: "conf"}
	want := []string{"-L", "sock", "-f", "conf"}
	if got := c.BaseArgs(); !slices.Equal(got, want) {
		t.Errorf("BaseArgs() = %v, want %v", got, want)
	}
}

func TestNewSessionArgs(t *testing.T) {
	c := Client{Socket: "sock", ConfPath: "conf"}
	want := []string{"-L", "sock", "-f", "conf", "new-session", "-A", "-s", "n", "-c", "/d"}
	if got := c.NewSessionArgs("n", "/d"); !slices.Equal(got, want) {
		t.Errorf("NewSessionArgs() = %v, want %v", got, want)
	}
}

// --- Exec-error discrimination (no sessions created; runs even without tmux
// on the real PATH because PATH is scrubbed anyway) ---

func TestHasSessionTmuxMissingReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // tmux unresolvable
	c := Client{Socket: "ktest-no-such-socket", ConfPath: "/dev/null"}
	alive, err := c.HasSession(context.Background(), "whatever")
	if err == nil {
		t.Fatal("HasSession with tmux missing: want err != nil, got nil (would misread 'tmux missing' as 'session dead')")
	}
	if alive {
		t.Error("HasSession with tmux missing: alive = true, want false")
	}
}

// --- Real-tmux integration tests (per-test sockets) ---

func TestHasSessionLive(t *testing.T) {
	c := newTestClient(t)
	newDetachedSession(t, c, "kangent-1-1")
	alive, err := c.HasSession(context.Background(), "kangent-1-1")
	if err != nil {
		t.Fatalf("HasSession: %v", err)
	}
	if !alive {
		t.Error("HasSession(live session) = false, want true")
	}
}

func TestHasSessionDead(t *testing.T) {
	c := newTestClient(t)
	// Never-created name on a socket whose server isn't even running:
	// dead session and dead server both exit 1 -> (false, nil).
	alive, err := c.HasSession(context.Background(), "kangent-9-9")
	if err != nil {
		t.Fatalf("HasSession(dead server): want nil err, got %v", err)
	}
	if alive {
		t.Error("HasSession(never-created) = true, want false")
	}
	// Now with a live server but a different session name.
	newDetachedSession(t, c, "kangent-1-1")
	alive, err = c.HasSession(context.Background(), "kangent-9-9")
	if err != nil {
		t.Fatalf("HasSession(missing session, live server): want nil err, got %v", err)
	}
	if alive {
		t.Error("HasSession(missing session, live server) = true, want false")
	}
}

func TestKillSessionIdempotent(t *testing.T) {
	c := newTestClient(t)
	newDetachedSession(t, c, "kangent-1-1")

	if err := c.KillSession(context.Background(), "kangent-1-1"); err != nil {
		t.Fatalf("KillSession(live): %v", err)
	}
	alive, err := c.HasSession(context.Background(), "kangent-1-1")
	if err != nil {
		t.Fatalf("HasSession after kill: %v", err)
	}
	if alive {
		t.Error("session still alive after KillSession")
	}
	// Second kill: already dead (and server gone -- it was the last session).
	if err := c.KillSession(context.Background(), "kangent-1-1"); err != nil {
		t.Errorf("KillSession(already dead) = %v, want nil (idempotent)", err)
	}
	// Kill against a dead server is also nil.
	if err := c.KillSession(context.Background(), "never-existed"); err != nil {
		t.Errorf("KillSession(dead server) = %v, want nil (idempotent)", err)
	}
}

// TestPrefixCollision guards the Empirical Finding 2 trap: WITHOUT "=",
// tmux prefix-matches session names, so kill-session -t kangent-1-1 could
// hit kangent-1-10. The "=" exact match must prevent that.
func TestPrefixCollision(t *testing.T) {
	c := newTestClient(t)
	newDetachedSession(t, c, "kangent-1-1")
	newDetachedSession(t, c, "kangent-1-10")

	// Exact-match probe of a strict prefix of both names must be dead,
	// despite both prefix-matching it.
	alive, err := c.HasSession(context.Background(), "kangent-1")
	if err != nil {
		t.Fatalf("HasSession(kangent-1): %v", err)
	}
	if alive {
		t.Error(`HasSession("kangent-1") = true with only kangent-1-1/kangent-1-10 alive: "=" exact match is broken`)
	}

	if err := c.KillSession(context.Background(), "kangent-1-1"); err != nil {
		t.Fatalf("KillSession(kangent-1-1): %v", err)
	}
	alive, err = c.HasSession(context.Background(), "kangent-1-10")
	if err != nil {
		t.Fatalf("HasSession(kangent-1-10): %v", err)
	}
	if !alive {
		t.Error("killing kangent-1-1 also killed kangent-1-10: prefix-collision guard failed")
	}
	alive, err = c.HasSession(context.Background(), "kangent-1-1")
	if err != nil {
		t.Fatalf("HasSession(kangent-1-1): %v", err)
	}
	if alive {
		t.Error("kangent-1-1 still alive after KillSession")
	}
}

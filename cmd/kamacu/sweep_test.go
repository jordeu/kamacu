package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kamacu/internal/store"
	"kamacu/internal/tmux"
)

// newSweepClient returns a tmux.Client on a unique per-test socket, skipping
// the test when tmux is unavailable (the house host-gate convention). KillServer
// cleanup is registered BEFORE any session can be created so no test leaks a
// tmux server. Local re-creation of internal/tmux's unexported newTestClient —
// test-file helpers are not importable across packages.
func newSweepClient(t *testing.T) tmux.Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	socket := fmt.Sprintf("ktest-%d-%s", os.Getpid(), t.Name())
	c := tmux.Client{Socket: socket, ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	return c
}

// newDetachedSession creates a session directly with -d: tests have no tty,
// so production's `new-session -A` (no -d) would fail "open terminal failed:
// not a terminal". Local re-creation of internal/tmux's unexported helper.
func newDetachedSession(t *testing.T, c tmux.Client, name string) {
	t.Helper()
	args := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", t.TempDir())
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("create detached session %q: %v: %s", name, err, out)
	}
}

// awaitHasSession polls the tmux client until the named session's liveness
// matches want — kills and probes are async (mirrors the internal/api
// sessions_test.go helper shape).
func awaitHasSession(t *testing.T, c tmux.Client, name string, want bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		alive, err := c.HasSession(context.Background(), name)
		if err == nil && alive == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("HasSession(%q) never became %v (last: alive=%v err=%v)", name, want, alive, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSweepOrphanTmuxScopeAware proves the GDATA-03 co-phasing fix: the
// startup sweep never kills a live global tab (scope='global', task-less by
// design) while unchanged orphan killing is preserved. The global row is
// seeded DIRECTLY via SQL — no global spawn path exists until Phase 15; the
// row is the sweep's only input.
//
//	RED (pre-fix, this regression): the INNER-JOIN known-set drops NULL-task
//	rows, so the live kamacu-global-1 session looks orphaned and the sweep
//	KILLS it — the empirically demonstrated killer bug the co-phasing mandate
//	exists for (11 known rows on the real install -> 10 under the INNER JOIN).
//	GREEN (post-fix): kamacu-global-1 ALIVE, task-backed kamacu-1-1 ALIVE,
//	true orphan kamacu-42-1 (no row, no task) DEAD.
func TestSweepOrphanTmuxScopeAware(t *testing.T) {
	client := newSweepClient(t)

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	// Seed: one project + one task (post-00012 shape), a task-backed tmux row,
	// and a scope='global' row with task_id NULL. NO row for kamacu-42-1 and
	// no task 42 — a true task-orphan.
	projRes, err := db.Exec(
		`INSERT INTO projects (name, repo_path, workspace_id) VALUES (?, ?, 1)`,
		"P", filepath.Join(t.TempDir(), "repo"),
	)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	projID, _ := projRes.LastInsertId()
	taskRes, err := db.Exec(
		`INSERT INTO tasks (project_id, title, position) VALUES (?, 'T', 1.0)`, projID)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	taskID, _ := taskRes.LastInsertId()
	taskName := fmt.Sprintf("kamacu-%d-1", taskID)
	if _, err := db.Exec(
		`INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (?, 'task', 1, ?, '')`,
		taskID, taskName,
	); err != nil {
		t.Fatalf("seed task-backed tmux row: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (NULL, 'global', 1, 'kamacu-global-1', '')`,
	); err != nil {
		t.Fatalf("seed global tmux row: %v", err)
	}

	// Three live sessions on the per-test socket, confirmed alive before the
	// sweep runs.
	newDetachedSession(t, client, taskName)
	newDetachedSession(t, client, "kamacu-global-1")
	newDetachedSession(t, client, "kamacu-42-1")
	awaitHasSession(t, client, taskName, true)
	awaitHasSession(t, client, "kamacu-global-1", true)
	awaitHasSession(t, client, "kamacu-42-1", true)

	sweepOrphanTmux(context.Background(), db, client)

	awaitHasSession(t, client, "kamacu-global-1", true) // SC4 first half: a live global tab survives
	awaitHasSession(t, client, taskName, true)          // unchanged: task-backed rows are known
	awaitHasSession(t, client, "kamacu-42-1", false)    // SC4 second half: true orphans still die
}

package migrate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIntegrationSetup skips when git is absent (mirrors internal/worktree's
// skip-if-no-git harness) and isolates every git call this test makes — the
// fixture commands AND the package-under-test's `worktree repair` exec, which
// inherits the test-scoped env — from the host's global/system git config.
// Returns a throwaway HOME-like root for the ~/.kangent/-shaped layout.
func gitIntegrationSetup(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping git-integration repair test")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	return t.TempDir()
}

// gitTest runs `git -C dir args...` with a throwaway identity, failing the test
// on any nonzero exit. Returns combined output.
func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// buildRepoWithWorktree mirrors the live install: a main repo at
// root/repos/o/n with one commit, plus a linked worktree at
// root/worktrees/p/slug-1 added with an ABSOLUTE path so git stores absolute
// two-way link pointers (the exact state that breaks when the root moves).
func buildRepoWithWorktree(t *testing.T, root string) (repoPath, wtPath string) {
	t.Helper()
	repoPath = filepath.Join(root, "repos", "o", "n")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repoPath, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repoPath, "add", "file.txt")
	gitTest(t, repoPath, "commit", "-m", "initial")

	wtPath = filepath.Join(root, "worktrees", "p", "slug-1")
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repoPath, "worktree", "add", "-b", "task/slug-1", wtPath)
	return repoPath, wtPath
}

// TestCompleteRepairsMovedWorktrees is the headline git-integration test: build a
// real repos/+worktrees/ layout under OldRoot, rename the whole tree to NewRoot
// (the Part-1 os.Rename commit point), then run Complete and assert the moved
// worktree is a usable git repo (worktree list clean, git status succeeds), the
// kangent-* tmux row was deleted, and a second Complete is a clean no-op.
func TestCompleteRepairsMovedWorktrees(t *testing.T) {
	root := gitIntegrationSetup(t)
	oldRoot := filepath.Join(root, ".kangent")
	newRoot := filepath.Join(root, ".kamacu")

	repoPath, wtPath := buildRepoWithWorktree(t, oldRoot)

	db := newMigrateTestDB(t)
	res, err := db.Exec(`INSERT INTO projects (name, repo_path, managed) VALUES ('m', ?, 1)`, repoPath)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 't', 1, ?)`, projID, wtPath)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	taskID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO tmux_sessions (task_id, n, name) VALUES (?, 1, 'kangent-1-1')`, taskID); err != nil {
		t.Fatalf("insert tmux row: %v", err)
	}

	// COMMIT POINT: move the entire tree (breaks the absolute worktree pointers).
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("rename tree: %v", err)
	}

	cfg := Config{OldRoot: oldRoot, NewRoot: newRoot}
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	newRepoPath := filepath.Join(newRoot, "repos", "o", "n")
	newWtPath := filepath.Join(newRoot, "worktrees", "p", "slug-1")

	list := gitTest(t, newRepoPath, "worktree", "list", "--porcelain")
	if strings.Contains(list, "prunable") {
		t.Errorf("worktree still prunable after repair:\n%s", list)
	}
	if !strings.Contains(list, newWtPath) {
		t.Errorf("repaired worktree list missing new path %q:\n%s", newWtPath, list)
	}
	// The moved worktree must be a usable git repo (not "not a git repository").
	if out, err := exec.Command("git", "-C", newWtPath, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("git status in moved worktree failed: %v\n%s", err, out)
	}

	// Orchestration: Complete also deleted the kangent-* tmux row.
	var kangentRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name LIKE 'kangent-%'`).Scan(&kangentRows); err != nil {
		t.Fatalf("count tmux rows: %v", err)
	}
	if kangentRows != 0 {
		t.Errorf("Complete left %d kangent-* tmux rows, want 0", kangentRows)
	}

	// Idempotent: a second Complete on the healthy tree is a clean exit-0 no-op.
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("second Complete (idempotency): %v", err)
	}
	if out, err := exec.Command("git", "-C", newWtPath, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("git status after second Complete failed: %v\n%s", err, out)
	}
}

// TestCompleteStalePathFiltered asserts a task whose worktree dir does not exist
// on disk is os.Stat-filtered OUT of the repair arg list, so git never sees a
// stale path (which would make it exit 1) — Complete still succeeds and the valid
// worktree is repaired.
func TestCompleteStalePathFiltered(t *testing.T) {
	root := gitIntegrationSetup(t)
	oldRoot := filepath.Join(root, ".kangent")
	newRoot := filepath.Join(root, ".kamacu")

	repoPath, wtPath := buildRepoWithWorktree(t, oldRoot)

	db := newMigrateTestDB(t)
	res, err := db.Exec(`INSERT INTO projects (name, repo_path, managed) VALUES ('m', ?, 1)`, repoPath)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 'valid', 1, ?)`, projID, wtPath); err != nil {
		t.Fatalf("insert valid task: %v", err)
	}
	// A task whose worktree dir was never created on disk (user-deleted tree).
	ghost := filepath.Join(oldRoot, "worktrees", "p", "ghost-2")
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 'ghost', 2, ?)`, projID, ghost); err != nil {
		t.Fatalf("insert ghost task: %v", err)
	}

	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("rename tree: %v", err)
	}

	cfg := Config{OldRoot: oldRoot, NewRoot: newRoot}
	// If the ghost path were passed to `git worktree repair`, git would exit 1
	// ("not a valid path") and Complete would error. A nil error proves the filter.
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("Complete with a stale worktree path (filter failed?): %v", err)
	}

	newWtPath := filepath.Join(newRoot, "worktrees", "p", "slug-1")
	if out, err := exec.Command("git", "-C", newWtPath, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("valid worktree not repaired: %v\n%s", err, out)
	}
}

// TestCompleteToleratesUnregisteredWorktree closes Gap 1 (21-VERIFICATION.md): a
// DB-referenced worktree dir that EXISTS on disk but is NOT registered in the
// repo's .git/worktrees/ (its .git gitfile dangles) — faithfully reproducing the
// real sched repo's 4 stale dirs. `git worktree repair <that path>` exits 1
// ("does not reference a repository"). Before the fix, the single batch repair
// invocation exits 1, repairWorktrees returns a fatal error, Complete aborts,
// deleteOldTmuxRows never runs, and the app refuses to boot on every RollForward.
// After the fix, per-path repair slog.Warn-logs and SKIPS the straggler: Complete
// returns nil, the VALID worktree is still repaired, and the kangent-% tmux rows
// are still deleted.
func TestCompleteToleratesUnregisteredWorktree(t *testing.T) {
	root := gitIntegrationSetup(t)
	oldRoot := filepath.Join(root, ".kangent")
	newRoot := filepath.Join(root, ".kamacu")

	repoPath, wtPath := buildRepoWithWorktree(t, oldRoot)

	// A SECOND worktree, then delete its .git/worktrees/<id> registration entry so
	// the checkout dir is left with a dangling .git gitfile — existing on disk +
	// in the DB but absent from .git/worktrees/, exactly like the 4 real sched dirs.
	ghostPath := filepath.Join(oldRoot, "worktrees", "p", "ghost-2")
	gitTest(t, repoPath, "worktree", "add", "-b", "task/ghost-2", ghostPath)
	if err := os.RemoveAll(filepath.Join(repoPath, ".git", "worktrees", "ghost-2")); err != nil {
		t.Fatalf("unregister ghost worktree: %v", err)
	}
	// Fixture sanity: git can no longer repair the ghost (it exits non-zero). If
	// this ever passes, the test would no longer reproduce the abort it guards.
	if out, err := exec.Command("git", "-C", repoPath, "worktree", "repair", ghostPath).CombinedOutput(); err == nil {
		t.Fatalf("ghost worktree unexpectedly repairable; fixture no longer reproduces Gap 1:\n%s", out)
	}

	db := newMigrateTestDB(t)
	res, err := db.Exec(`INSERT INTO projects (name, repo_path, managed) VALUES ('m', ?, 1)`, repoPath)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 'valid', 1, ?)`, projID, wtPath)
	if err != nil {
		t.Fatalf("insert valid task: %v", err)
	}
	validTaskID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 'ghost', 2, ?)`, projID, ghostPath); err != nil {
		t.Fatalf("insert ghost task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tmux_sessions (task_id, n, name) VALUES (?, 1, 'kangent-1-1')`, validTaskID); err != nil {
		t.Fatalf("insert tmux row: %v", err)
	}

	// COMMIT POINT: move the whole tree (Part-1 os.Rename) — breaks the absolute
	// pointers of BOTH the valid and the already-unregistered ghost worktree.
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("rename tree: %v", err)
	}

	cfg := Config{OldRoot: oldRoot, NewRoot: newRoot}
	// The unregistered ghost straggler must NOT abort the migration.
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("Complete aborted on an unregistered worktree straggler: %v", err)
	}

	newRepoPath := filepath.Join(newRoot, "repos", "o", "n")
	newValidWt := filepath.Join(newRoot, "worktrees", "p", "slug-1")

	// The VALID worktree is still repaired: usable git status, listed, not prunable.
	list := gitTest(t, newRepoPath, "worktree", "list", "--porcelain")
	if strings.Contains(list, "prunable") {
		t.Errorf("valid worktree prunable after repair:\n%s", list)
	}
	if !strings.Contains(list, newValidWt) {
		t.Errorf("repaired worktree list missing valid path %q:\n%s", newValidWt, list)
	}
	if out, err := exec.Command("git", "-C", newValidWt, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("git status in repaired valid worktree failed: %v\n%s", err, out)
	}

	// deleteOldTmuxRows ran AFTER repair — the MIGRATE-03 step the abort used to skip.
	var kangentRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name LIKE 'kangent-%'`).Scan(&kangentRows); err != nil {
		t.Fatalf("count tmux rows: %v", err)
	}
	if kangentRows != 0 {
		t.Errorf("Complete left %d kangent-* tmux rows, want 0 (deleteOldTmuxRows was skipped)", kangentRows)
	}

	// A second Complete on the same state is a clean no-op (returns nil, valid tree usable).
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("second Complete (idempotency) errored: %v", err)
	}
	if out, err := exec.Command("git", "-C", newValidWt, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("git status after second Complete failed: %v\n%s", err, out)
	}
}

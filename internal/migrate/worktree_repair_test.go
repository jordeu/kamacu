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

// TestCompleteRepairsFolderPointedWorktreesAndSurvivesPrune closes CR-01: a
// folder-pointed (managed=0) project whose main repo lives OUTSIDE the data root
// (and so does NOT move on the Part-1 rename) still has its task worktrees under
// the global worktree_base — provisionWorktree places worktrees under worktree_base
// for EVERY project, the managed flag only gates a best-effort fetch. When the root
// moves, the external repo's .git/worktrees/<id>/gitdir pointers go stale. Before
// the fix, repairWorktrees filtered managed=1 and SKIPPED this project, leaving the
// stale pointers — a later `git worktree prune` (worktree.go:378, run after every
// task removal) then DEREGISTERS the migrated worktrees, orphaning live checkouts
// while the DB still points at them (sibling worktree loss). After the fix, repair
// is gated on under-new-root worktree OWNERSHIP (not the managed flag), so the
// external repo's moved worktrees are repaired and survive a subsequent prune.
func TestCompleteRepairsFolderPointedWorktreesAndSurvivesPrune(t *testing.T) {
	root := gitIntegrationSetup(t)
	oldRoot := filepath.Join(root, ".kangent")
	newRoot := filepath.Join(root, ".kamacu")

	// An EXTERNAL repo — a SIBLING of the data root, NOT under it, so os.Rename of
	// oldRoot does not touch it (the folder-pointed shape the managed harness omits).
	extRepo := filepath.Join(root, "external", "o", "n")
	if err := os.MkdirAll(extRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, extRepo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(extRepo, "file.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, extRepo, "add", "file.txt")
	gitTest(t, extRepo, "commit", "-m", "initial")

	// TWO linked worktrees UNDER oldRoot with ABSOLUTE paths (git bakes absolute
	// two-way pointers). Two make the sibling-loss regression concrete: prune of
	// one stale entry drops the other.
	wt1 := filepath.Join(oldRoot, "worktrees", "p", "slug-1")
	wt2 := filepath.Join(oldRoot, "worktrees", "p", "slug-2")
	if err := os.MkdirAll(filepath.Dir(wt1), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, extRepo, "worktree", "add", "-b", "task/slug-1", wt1)
	gitTest(t, extRepo, "worktree", "add", "-b", "task/slug-2", wt2)

	db := newMigrateTestDB(t)
	// managed=0: the external, folder-pointed project. repo_path is the UNMOVED
	// external checkout (it is NOT under the data root, so rewriteManagedPaths
	// leaves repo_path alone — only the tasks' worktree_path values are rewritten).
	res, err := db.Exec(`INSERT INTO projects (name, repo_path, managed) VALUES ('folder', ?, 0)`, extRepo)
	if err != nil {
		t.Fatalf("insert folder-pointed project: %v", err)
	}
	projID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 't1', 1, ?)`, projID, wt1)
	if err != nil {
		t.Fatalf("insert task 1: %v", err)
	}
	taskID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 't2', 2, ?)`, projID, wt2); err != nil {
		t.Fatalf("insert task 2: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tmux_sessions (task_id, n, name) VALUES (?, 1, 'kangent-1-1')`, taskID); err != nil {
		t.Fatalf("insert tmux row: %v", err)
	}

	// COMMIT POINT: move BOTH worktrees; extRepo (a sibling of oldRoot) stays put
	// and now holds stale .git/worktrees/*/gitdir pointers at the vanished paths.
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("rename tree: %v", err)
	}

	cfg := Config{OldRoot: oldRoot, NewRoot: newRoot}
	if err := Complete(context.Background(), db, cfg); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	newWt1 := filepath.Join(newRoot, "worktrees", "p", "slug-1")
	newWt2 := filepath.Join(newRoot, "worktrees", "p", "slug-2")

	// The external repo's worktree list now points at the NEW paths, none prunable.
	list := gitTest(t, extRepo, "worktree", "list", "--porcelain")
	if strings.Contains(list, "prunable") {
		t.Errorf("folder-pointed worktree still prunable after repair:\n%s", list)
	}
	if !strings.Contains(list, newWt1) {
		t.Errorf("repaired worktree list missing new path %q:\n%s", newWt1, list)
	}
	if !strings.Contains(list, newWt2) {
		t.Errorf("repaired worktree list missing new path %q:\n%s", newWt2, list)
	}

	// DECISIVE regression: a subsequent prune must NOT deregister the migrated
	// worktrees (pre-fix, repair was skipped, the stale gitdir survived, and prune
	// dropped the worktrees — sibling loss).
	gitTest(t, extRepo, "worktree", "prune")
	listAfter := gitTest(t, extRepo, "worktree", "list", "--porcelain")
	if !strings.Contains(listAfter, newWt1) {
		t.Errorf("worktree %q deregistered by prune (CR-01 sibling loss):\n%s", newWt1, listAfter)
	}
	if !strings.Contains(listAfter, newWt2) {
		t.Errorf("worktree %q deregistered by prune (CR-01 sibling loss):\n%s", newWt2, listAfter)
	}
	if out, err := exec.Command("git", "-C", newWt1, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Errorf("git status in migrated worktree failed after prune: %v\n%s", err, out)
	}

	// MIGRATE-03: Complete proceeded to deleteOldTmuxRows for the folder-pointed
	// project too — the kangent-* row is gone regardless of the managed flag.
	var kangentRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name LIKE 'kangent-%'`).Scan(&kangentRows); err != nil {
		t.Fatalf("count tmux rows: %v", err)
	}
	if kangentRows != 0 {
		t.Errorf("Complete left %d kangent-* tmux rows, want 0", kangentRows)
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

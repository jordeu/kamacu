package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// newMigrateTestDB opens + migrates a throwaway SQLite DB in t.TempDir() and
// returns it (closed on cleanup). Mirrors the store_test temp-DB harness.
func newMigrateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// fixtureRoots is the concrete OldRoot/NewRoot pair used by the path-scope tests.
// OldRoot/NewRoot are siblings (as ~/.kangent and ~/.kamacu are), so a NewRoot
// path can never match the OldRoot||'/%' LIKE gate — the idempotency guarantee.
const (
	fxOldRoot = "/home/u/.kangent"
	fxNewRoot = "/home/u/.kamacu"
	// fxUnmanaged deliberately CONTAINS the substring "kangent" but lives OUTSIDE
	// the old data root — a blind string replace would corrupt it (D-15 guard).
	fxUnmanaged = "/home/u/dev/kangent-thing"
)

// seedScopeFixture inserts the plan's fixture: a managed project under OldRoot, an
// unmanaged (managed=0) project outside it, a task worktree under OldRoot, a raw
// worktree_base setting, and two kangent-* tmux rows. Returns the managed project
// id and the task id.
func seedScopeFixture(t *testing.T, db *sql.DB) (projID, taskID int64) {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO projects (name, repo_path, managed) VALUES ('managed', ?, 1)`,
		fxOldRoot+"/repos/o/n")
	if err != nil {
		t.Fatalf("insert managed project: %v", err)
	}
	projID, _ = res.LastInsertId()

	if _, err := db.Exec(
		`INSERT INTO projects (name, repo_path, managed) VALUES ('external', ?, 0)`,
		fxUnmanaged); err != nil {
		t.Fatalf("insert unmanaged project: %v", err)
	}

	res, err = db.Exec(
		`INSERT INTO tasks (project_id, title, position, worktree_path) VALUES (?, 'task', 1, ?)`,
		projID, fxOldRoot+"/worktrees/p/slug-1")
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	taskID, _ = res.LastInsertId()

	if _, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)`,
		settings.KeyWorktreeBase, "~/.kangent/worktrees/"); err != nil {
		t.Fatalf("insert worktree_base setting: %v", err)
	}

	for _, row := range []struct {
		n    int
		name string
	}{{1, "kangent-1-1"}, {2, "kangent-1-2"}} {
		if _, err := db.Exec(
			`INSERT INTO tmux_sessions (task_id, n, name) VALUES (?, ?, ?)`,
			taskID, row.n, row.name); err != nil {
			t.Fatalf("insert tmux row %s: %v", row.name, err)
		}
	}
	return projID, taskID
}

func scanRepoPath(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var p string
	if err := db.QueryRow(`SELECT repo_path FROM projects WHERE id = ?`, id).Scan(&p); err != nil {
		t.Fatalf("scan repo_path: %v", err)
	}
	return p
}

func scanWorktreePath(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var p sql.NullString
	if err := db.QueryRow(`SELECT worktree_path FROM tasks WHERE id = ?`, id).Scan(&p); err != nil {
		t.Fatalf("scan worktree_path: %v", err)
	}
	return p.String
}

func scanSetting(t *testing.T, db *sql.DB, key string) string {
	t.Helper()
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		t.Fatalf("scan setting %s: %v", key, err)
	}
	return v
}

// TestRewriteManagedPaths asserts managed repo_path + task worktree_path + the
// raw worktree_base setting are all prefix-swapped OldRoot -> NewRoot.
func TestRewriteManagedPaths(t *testing.T) {
	db := newMigrateTestDB(t)
	projID, taskID := seedScopeFixture(t, db)
	cfg := Config{OldRoot: fxOldRoot, NewRoot: fxNewRoot}

	if err := rewriteManagedPaths(db, cfg); err != nil {
		t.Fatalf("rewriteManagedPaths: %v", err)
	}

	if got, want := scanRepoPath(t, db, projID), fxNewRoot+"/repos/o/n"; got != want {
		t.Errorf("managed repo_path = %q, want %q", got, want)
	}
	if got, want := scanWorktreePath(t, db, taskID), fxNewRoot+"/worktrees/p/slug-1"; got != want {
		t.Errorf("task worktree_path = %q, want %q", got, want)
	}
	if got, want := scanSetting(t, db, settings.KeyWorktreeBase), "~/.kamacu/worktrees/"; got != want {
		t.Errorf("worktree_base = %q, want %q", got, want)
	}
}

// TestPathScopeUnmanagedUntouched is the D-15 data-loss guard: an unmanaged
// (managed=0) project OUTSIDE the old root is byte-for-byte unchanged even though
// its path contains the substring "kangent".
func TestPathScopeUnmanagedUntouched(t *testing.T) {
	db := newMigrateTestDB(t)
	seedScopeFixture(t, db)
	cfg := Config{OldRoot: fxOldRoot, NewRoot: fxNewRoot}

	var before string
	if err := db.QueryRow(`SELECT repo_path FROM projects WHERE managed = 0`).Scan(&before); err != nil {
		t.Fatalf("scan unmanaged before: %v", err)
	}
	if before != fxUnmanaged {
		t.Fatalf("fixture unmanaged path = %q, want %q", before, fxUnmanaged)
	}

	if err := rewriteManagedPaths(db, cfg); err != nil {
		t.Fatalf("rewriteManagedPaths: %v", err)
	}

	var after string
	if err := db.QueryRow(`SELECT repo_path FROM projects WHERE managed = 0`).Scan(&after); err != nil {
		t.Fatalf("scan unmanaged after: %v", err)
	}
	if after != fxUnmanaged {
		t.Errorf("unmanaged repo_path was rewritten: got %q, want %q (D-15 violation)", after, fxUnmanaged)
	}
}

// TestRewriteIdempotent asserts a second rewrite is a clean no-op: the LIKE
// OldRoot||'/%' gate no longer matches once the rows are under NewRoot.
func TestRewriteIdempotent(t *testing.T) {
	db := newMigrateTestDB(t)
	projID, taskID := seedScopeFixture(t, db)
	cfg := Config{OldRoot: fxOldRoot, NewRoot: fxNewRoot}

	if err := rewriteManagedPaths(db, cfg); err != nil {
		t.Fatalf("first rewriteManagedPaths: %v", err)
	}
	repoAfter1 := scanRepoPath(t, db, projID)
	wtAfter1 := scanWorktreePath(t, db, taskID)
	setAfter1 := scanSetting(t, db, settings.KeyWorktreeBase)

	if err := rewriteManagedPaths(db, cfg); err != nil {
		t.Fatalf("second rewriteManagedPaths: %v", err)
	}
	if got := scanRepoPath(t, db, projID); got != repoAfter1 {
		t.Errorf("repo_path changed on second run: %q -> %q", repoAfter1, got)
	}
	if got := scanWorktreePath(t, db, taskID); got != wtAfter1 {
		t.Errorf("worktree_path changed on second run: %q -> %q", wtAfter1, got)
	}
	if got := scanSetting(t, db, settings.KeyWorktreeBase); got != setAfter1 {
		t.Errorf("worktree_base changed on second run: %q -> %q", setAfter1, got)
	}
}

// TestRewriteAlreadyUnderNewRootLeftAlone asserts a managed project whose
// repo_path is ALREADY under NewRoot is not touched (self-gate no-match).
func TestRewriteAlreadyUnderNewRootLeftAlone(t *testing.T) {
	db := newMigrateTestDB(t)
	already := fxNewRoot + "/repos/x/y"
	res, err := db.Exec(`INSERT INTO projects (name, repo_path, managed) VALUES ('done', ?, 1)`, already)
	if err != nil {
		t.Fatalf("insert already-migrated project: %v", err)
	}
	id, _ := res.LastInsertId()
	cfg := Config{OldRoot: fxOldRoot, NewRoot: fxNewRoot}

	if err := rewriteManagedPaths(db, cfg); err != nil {
		t.Fatalf("rewriteManagedPaths: %v", err)
	}
	if got := scanRepoPath(t, db, id); got != already {
		t.Errorf("already-migrated repo_path changed: got %q, want %q", got, already)
	}
}

// TestDeleteOldTmuxRows asserts every kangent-* tmux_sessions row is removed
// while a new-socket kamacu-* row survives.
func TestDeleteOldTmuxRows(t *testing.T) {
	db := newMigrateTestDB(t)
	_, taskID := seedScopeFixture(t, db)

	// A kamacu-* row must survive (only kangent-* are retired).
	if _, err := db.Exec(`INSERT INTO tmux_sessions (task_id, n, name) VALUES (?, 3, 'kamacu-1-3')`, taskID); err != nil {
		t.Fatalf("insert kamacu row: %v", err)
	}

	if err := deleteOldTmuxRows(context.Background(), db); err != nil {
		t.Fatalf("deleteOldTmuxRows: %v", err)
	}

	var oldCount, newCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name LIKE 'kangent-%'`).Scan(&oldCount); err != nil {
		t.Fatalf("count kangent rows: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmux_sessions WHERE name LIKE 'kamacu-%'`).Scan(&newCount); err != nil {
		t.Fatalf("count kamacu rows: %v", err)
	}
	if oldCount != 0 {
		t.Errorf("kangent-* rows remaining = %d, want 0", oldCount)
	}
	if newCount != 1 {
		t.Errorf("kamacu-* rows = %d, want 1 (must not delete new-socket rows)", newCount)
	}
}

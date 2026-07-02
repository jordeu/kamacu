package api

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"kamacu/internal/session"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// cleanupTestEnv wires the minimal collaborators CleanupWorktreeGated needs for
// a DIRECT (non-HTTP) unit test: a migrated temp DB, a worktree.Service rooted
// at a temp dir, an empty session.Manager, and a zero-value tmux.Client (no
// live tmux). Every git command runs against a real repo in t.TempDir().
type cleanupTestEnv struct {
	db  *sql.DB
	wt  *worktree.Service
	mgr *session.Manager
}

func newCleanupEnv(t *testing.T) *cleanupTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &cleanupTestEnv{
		db:  db,
		wt:  worktree.NewService(t.TempDir()),
		mgr: session.NewManager(),
	}
}

// makeRepoAndWorktree builds a real repo with one commit plus a linked worktree
// on `feature`, returning (repo, worktreePath). Isolated git config + throwaway
// identity, matching the package's test hygiene.
func makeRepoAndWorktree(t *testing.T) (repo, wtPath string) {
	t.Helper()
	repo = gitRepoWithCommit(t)
	wtPath = filepath.Join(t.TempDir(), "wt")
	run := append([]string{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@test"},
		"worktree", "add", wtPath, "-b", "feature")
	cmd := exec.Command("git", run...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	return repo, wtPath
}

// TestCleanupWorktreeGatedOrphanSkipsUpdate is Extension B: taskID==0 (an orphan
// worktree with no DB task row) removes the worktree without running the
// null-columns UPDATE. We seed a task row and assert its columns are UNTOUCHED
// after an orphan (taskID=0) cleanup.
func TestCleanupWorktreeGatedOrphanSkipsUpdate(t *testing.T) {
	ctx := context.Background()
	env := newCleanupEnv(t)
	repo, wtPath := makeRepoAndWorktree(t)

	// A DB task row that must be left untouched (an orphan clean must not run any
	// UPDATE against arbitrary rows). Its columns are set; taskID=0 targets no row.
	pid := seedProject(t, env.db, repo)
	otherTask := seedTask(t, env.db, pid, "branch-x", "/some/path")

	removed, reason, err := CleanupWorktreeGated(
		ctx, env.db, env.wt, env.mgr, tmux.Client{}, nil,
		0 /*orphan taskID*/, repo, wtPath, 0 /*sessionCount*/, false, false)
	if err != nil || reason != "" || !removed {
		t.Fatalf("orphan cleanup = (removed=%v, reason=%q, err=%v), want (true, \"\", nil)", removed, reason, err)
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir still present after orphan cleanup: %v", statErr)
	}
	// The unrelated task row's columns must be exactly as seeded (no UPDATE ran).
	br, wp := taskColsFor(t, env.db, otherTask)
	if br != "branch-x" || wp != "/some/path" {
		t.Errorf("orphan cleanup mutated a task row: branch=%q worktree_path=%q, want branch-x / /some/path", br, wp)
	}
}

// TestCleanupWorktreeGatedReferencedRunsUpdate is the D-04 regression parity for
// the direct helper: taskID>0, clean worktree, remove succeeds → (true,"",nil)
// AND the null-columns UPDATE runs (byte-identical to today's behavior).
func TestCleanupWorktreeGatedReferencedRunsUpdate(t *testing.T) {
	ctx := context.Background()
	env := newCleanupEnv(t)
	repo, wtPath := makeRepoAndWorktree(t)
	pid := seedProject(t, env.db, repo)
	id := seedTask(t, env.db, pid, "feature", wtPath)

	removed, reason, err := CleanupWorktreeGated(
		ctx, env.db, env.wt, env.mgr, tmux.Client{}, nil,
		id, repo, wtPath, 0, false, false)
	if err != nil || reason != "" || !removed {
		t.Fatalf("referenced cleanup = (removed=%v, reason=%q, err=%v), want (true, \"\", nil)", removed, reason, err)
	}
	br, wp := taskColsFor(t, env.db, id)
	if br != "" || wp != "" {
		t.Errorf("referenced cleanup did NOT null the columns: branch=%q worktree_path=%q, want both NULL", br, wp)
	}
}

// TestCleanupWorktreeGatedBlockedOnStderr is Extension A via the REAL git path:
// a worktree containing a mode-000 subdir cannot be deleted by the unprivileged
// process; git's plain remove fails with "Permission denied" / "Directory not
// empty" (verified). CleanupWorktreeGated must return reason=="blocked" with a
// *BlockedError, and MUST NOT retry --force (Pitfall 1).
func TestCleanupWorktreeGatedBlockedOnStderr(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode-000 does not block removal")
	}
	ctx := context.Background()
	env := newCleanupEnv(t)
	repo, wtPath := makeRepoAndWorktree(t)

	blocked := filepath.Join(wtPath, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir blocked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "x"), []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write blocked/x: %v", err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) }) // let t.TempDir cleanup succeed

	removed, reason, err := CleanupWorktreeGated(
		ctx, env.db, env.wt, env.mgr, tmux.Client{}, nil,
		0, repo, wtPath, 0, false, false)
	if removed {
		t.Errorf("blocked cleanup reported removed=true, want false")
	}
	if reason != "blocked" {
		t.Errorf("reason = %q, want %q", reason, "blocked")
	}
	var be *BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v (%T), want a *BlockedError", err, err)
	}
	if be.Path == "" {
		t.Errorf("BlockedError.Path is empty, want the offending path")
	}
	// Pitfall 1: no --force retry — the tree survives (blocked shell), NOT deregistered-and-gone.
	if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
		t.Errorf("worktree dir vanished — a --force retry was attempted on the blocked case (Pitfall 1)")
	}
}

// TestCleanupWorktreeGatedUnrelatedErrorUnchanged: a non-permission remove error
// (e.g. a bogus repo) returns (false, "", err) UNCHANGED — never classified as
// blocked.
func TestCleanupWorktreeGatedUnrelatedErrorUnchanged(t *testing.T) {
	ctx := context.Background()
	env := newCleanupEnv(t)
	// A path that exists (so the dirty gate stats it clean-ish) but a repo that is
	// not a git repo → wt.Remove fails with a non-permission git error.
	notARepo := t.TempDir()
	wtPath := filepath.Join(t.TempDir(), "present")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir wtPath: %v", err)
	}

	removed, reason, err := CleanupWorktreeGated(
		ctx, env.db, env.wt, env.mgr, tmux.Client{}, nil,
		0, notARepo, wtPath, 0, false, false)
	if removed {
		t.Errorf("unrelated-error cleanup reported removed=true, want false")
	}
	if reason != "" {
		t.Errorf("reason = %q, want \"\" (an unrelated error is NOT blocked)", reason)
	}
	if err == nil {
		t.Fatalf("err = nil, want the raw git error surfaced unchanged")
	}
	var be *BlockedError
	if errors.As(err, &be) {
		t.Errorf("unrelated error was classified as *BlockedError: %v", err)
	}
}

// TestClassifyRemoveBlockedPathError asserts the fs.ErrPermission / *fs.PathError
// branch of Extension A directly (RESEARCH Option B, verified): os.RemoveAll on a
// dir containing a mode-000 subtree returns an error that is fs.ErrPermission AND
// exposes the exact blocked .Path via *fs.PathError. wt.Remove shells out to git
// (a string error), so this Go-native path only fires defensively — assert the
// classifier handles it and carries pe.Path.
func TestClassifyRemoveBlockedPathError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode-000 does not block removal")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "blocked")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "x"), []byte("y"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	rmErr := os.RemoveAll(dir)
	if rmErr == nil {
		t.Fatal("os.RemoveAll on a mode-000 subtree unexpectedly succeeded")
	}
	// Sanity: the verified properties the classifier relies on.
	if !errors.Is(rmErr, fs.ErrPermission) || !errors.Is(rmErr, syscall.EACCES) {
		t.Fatalf("os.RemoveAll error %v is not fs.ErrPermission/EACCES", rmErr)
	}

	be, ok := classifyRemoveBlocked(rmErr, "/fallback")
	if !ok {
		t.Fatalf("classifyRemoveBlocked did not classify an fs.ErrPermission *fs.PathError as blocked")
	}
	var pe *fs.PathError
	if !errors.As(rmErr, &pe) {
		t.Fatalf("test precondition: error is not a *fs.PathError")
	}
	if be.Path != pe.Path {
		t.Errorf("BlockedError.Path = %q, want the PathError path %q (not the fallback)", be.Path, pe.Path)
	}
}

// TestClassifyRemoveBlockedStderr asserts the string fallback: a plain error whose
// message contains git's "Permission denied" (no wrapped PathError) still yields a
// blocked outcome; the offending dir is parsed from "could not open directory
// '<dir>'" when present, else the passed worktree path is used.
func TestClassifyRemoveBlockedStderr(t *testing.T) {
	cases := []struct {
		name     string
		errMsg   string
		fallback string
		wantPath string
	}{
		{
			name:     "permission denied with could-not-open-directory dir",
			errMsg:   "warning: could not open directory '.db/': Permission denied\nerror: failed to delete '/wt': Directory not empty",
			fallback: "/wt",
			wantPath: ".db/",
		},
		{
			name:     "directory not empty + failed to delete, no dir in message",
			errMsg:   "error: failed to delete '/wt': Directory not empty",
			fallback: "/wt",
			wantPath: "/wt",
		},
		{
			name:     "permission denied without a directory hint falls back to worktree path",
			errMsg:   "some other Permission denied text",
			fallback: "/wt-path",
			wantPath: "/wt-path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be, ok := classifyRemoveBlocked(errors.New(tc.errMsg), tc.fallback)
			if !ok {
				t.Fatalf("classifyRemoveBlocked(%q) not classified as blocked", tc.errMsg)
			}
			if be.Path != tc.wantPath {
				t.Errorf("BlockedError.Path = %q, want %q", be.Path, tc.wantPath)
			}
		})
	}

	// A genuinely unrelated error is NOT blocked.
	if _, ok := classifyRemoveBlocked(errors.New("fatal: bad object HEAD"), "/wt"); ok {
		t.Errorf("classifyRemoveBlocked classified an unrelated error as blocked")
	}
}

// seedProject inserts a minimal project row and returns its id.
func seedProject(t *testing.T, db *sql.DB, repo string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO projects (name, repo_path) VALUES (?, ?)`, "proj", repo)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedTask inserts a task row with the given branch + worktree_path and returns
// its id.
func seedTask(t *testing.T, db *sql.DB, projectID int64, branch, wtPath string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO tasks (project_id, title, position, branch, worktree_path) VALUES (?, ?, 1.0, ?, ?)`,
		projectID, "task", branch, wtPath)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// taskColsFor returns (branch, worktree_path) for a task, empty strings when NULL.
func taskColsFor(t *testing.T, db *sql.DB, id int64) (branch, wtPath string) {
	t.Helper()
	var b, w sql.NullString
	if err := db.QueryRow(`SELECT branch, worktree_path FROM tasks WHERE id = ?`, id).Scan(&b, &w); err != nil {
		t.Fatalf("select task %d: %v", id, err)
	}
	return b.String, w.String
}

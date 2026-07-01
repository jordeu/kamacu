package reaper

// Spy-driven matrix for the reaper's PR reconcile pass (13-02, GHCLN-01/02).
// Real throwaway git repos + worktrees (no network, no gh — the gh read is the
// injected spyPRState) so the four conservative gates (dirty / unpushed / stash /
// session) run for real against actual git state. The clean-remove case wires a
// bare `origin` carrying refs/pull/<n>/head at the worktree HEAD so FetchRef
// succeeds and UnpushedCount("FETCH_HEAD")==0.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/session"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// spyPRState implements PRStateGetter from an injected per-(repo,n) state map.
// It records every call so a test can assert that a source='manual' task
// triggers ZERO PRState reads (the D-87 guard).
type spyPRState struct {
	states map[string]string // key prKey(repo, n) -> state
	calls  []string          // every queried key, in order
}

func (s *spyPRState) PRState(_ context.Context, repo string, n int) (string, error) {
	key := prKey(repo, int64(n))
	s.calls = append(s.calls, key)
	return s.states[key], nil
}

func (s *spyPRState) called(repo string, n int64) bool {
	key := prKey(repo, n)
	for _, c := range s.calls {
		if c == key {
			return true
		}
	}
	return false
}

func prKey(repo string, n int64) string { return fmt.Sprintf("%s#%d", repo, n) }

// gitT runs a git command in dir with a hermetic config, failing the test on
// error. Mirrors the worktree package's gitCmd helper.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// prFixture is a real repo with an `origin` bare remote carrying
// refs/pull/<n>/head, plus a worktree checked out on that PR head. It is exactly
// the shape reconcilePRsOnce's unpushed gate expects (FetchRef origin
// refs/pull/<n>/head then rev-list FETCH_HEAD..HEAD).
type prFixture struct {
	repoDir string // the working repo (the fetch + the "github_repo" stand-in); has origin
	wtPath  string // the PR worktree
	prN     int64
}

// makePRFixture builds the fixture: a bare origin, a working clone of it with one
// commit, a refs/pull/<n>/head ref on origin at that commit, and a worktree
// checked out on it. UnpushedCount("FETCH_HEAD") will be 0 on this clean tree.
func makePRFixture(t *testing.T, prN int64) prFixture {
	t.Helper()

	// Bare origin.
	originDir := filepath.Join(t.TempDir(), "origin.git")
	gitT(t, t.TempDir(), "init", "--bare", "-b", "main", originDir)

	// Working repo: init, one commit, point origin at the bare, push main.
	repoDir := t.TempDir()
	gitT(t, repoDir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	gitT(t, repoDir, "add", "file.txt")
	gitT(t, repoDir, "commit", "-m", "initial")
	gitT(t, repoDir, "remote", "add", "origin", originDir)
	gitT(t, repoDir, "push", "origin", "main")

	// Create refs/pull/<n>/head on origin pointing at the HEAD commit (this is
	// what gh would expose for a real PR). The worktree is then checked out on
	// that same commit, so a fresh fetch gives FETCH_HEAD == worktree HEAD.
	headSha := strings.TrimSpace(gitT(t, repoDir, "rev-parse", "HEAD"))
	gitT(t, repoDir, "push", "origin", fmt.Sprintf("%s:refs/pull/%d/head", headSha, prN))

	// Worktree on a named PR branch at the PR head (CheckoutPR shape).
	wtPath := filepath.Join(t.TempDir(), fmt.Sprintf("pr-%d", prN))
	gitT(t, repoDir, "worktree", "add", "-b", fmt.Sprintf("pr/%d", prN), wtPath, headSha)

	return prFixture{repoDir: repoDir, wtPath: wtPath, prN: prN}
}

// linkProjectRepo points the seeded project (id=1) at the fixture's repo via
// github_repo (the value PRState is keyed on) + repo_path (the fetch dir) so
// reconcilePRsOnce's SELECT picks up its tasks. github_repo is set to repoDir so
// the spy key matches what reconcilePRsOnce passes (t.repo = github_repo).
func linkProjectRepo(t *testing.T, db *sql.DB, repoDir string) {
	t.Helper()
	if _, err := db.Exec(
		`UPDATE projects SET github_repo = ?, repo_path = ? WHERE id = 1`, repoDir, repoDir); err != nil {
		t.Fatalf("link project repo: %v", err)
	}
}

// insertPRTask inserts a source='github_pr' task with pr_number + worktree_path
// set, owned by project 1.
func insertPRTask(t *testing.T, db *sql.DB, id, prNumber int64, worktreePath string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO tasks (id, project_id, title, status, position, source, pr_number, worktree_path)
		 VALUES (?, 1, 'pr', 'in_review', ?, 'github_pr', ?, ?)`,
		id, float64(id), prNumber, worktreePath); err != nil {
		t.Fatalf("insert pr task %d: %v", id, err)
	}
}

// insertManualTask inserts a source='manual' task WITH a worktree — the D-87
// guard subject: the PR pass must never select it.
func insertManualTask(t *testing.T, db *sql.DB, id int64, worktreePath string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO tasks (id, project_id, title, status, position, source, worktree_path)
		 VALUES (?, 1, 'manual', 'todo', ?, 'manual', ?)`,
		id, float64(id), worktreePath); err != nil {
		t.Fatalf("insert manual task %d: %v", id, err)
	}
}

func countTasks(t *testing.T, db *sql.DB, id int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count tasks %d: %v", id, err)
	}
	return n
}

// newReconciler builds the production-shaped reaper (NewWithPR) with a real empty
// *session.Manager (no PTYs — the clean case has no sessions), a real worktree
// service, a zero tmux.Client (Socket=="" -> liveTmuxNames returns nil), and the
// spy.
func newReconciler(db *sql.DB, spy *spyPRState) *Reaper {
	return NewWithPR(db, session.NewManager(), worktree.NewService(""), tmux.Client{}, spy)
}

// --- TestReconcileMergedCleanRemovesAndDeletes ---------------------------------

// A MERGED PR with a clean, idle worktree -> worktree removed AND task row
// DELETED (GHCLN-01, D-07).
func TestReconcileMergedCleanRemovesAndDeletes(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 42)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "MERGED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if _, err := os.Stat(fx.wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still present after clean MERGED reconcile: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 0 {
		t.Errorf("task row not deleted after clean MERGED reconcile: count=%d", n)
	}
}

// --- TestReconcileClosedRemovesAndDeletes --------------------------------------

// Same as the merged-clean case but state CLOSED also triggers removal+delete.
func TestReconcileClosedRemovesAndDeletes(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 7)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "CLOSED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if _, err := os.Stat(fx.wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree still present after clean CLOSED reconcile: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 0 {
		t.Errorf("task row not deleted after clean CLOSED reconcile: count=%d", n)
	}
}

// --- TestReconcileMergedDirtySkips ---------------------------------------------

// A MERGED PR with an UNCOMMITTED change -> the dirty gate trips: worktree AND
// row both REMAIN (GHCLN-02).
func TestReconcileMergedDirtySkips(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 11)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	// Uncommitted change -> DirtyCount > 0.
	if err := os.WriteFile(filepath.Join(fx.wtPath, "dirty.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "MERGED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if _, err := os.Stat(fx.wtPath); err != nil {
		t.Errorf("worktree removed despite dirty gate: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 1 {
		t.Errorf("task row deleted despite dirty gate: count=%d", n)
	}
}

// --- TestReconcileMergedUnpushedSkips ------------------------------------------

// A MERGED PR with a LOCAL-ONLY commit (FETCH_HEAD..HEAD > 0, invisible to
// porcelain) -> the unpushed gate trips: worktree AND row both REMAIN. This is
// Pitfall 1 — the gate exists precisely because status --porcelain is clean.
func TestReconcileMergedUnpushedSkips(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 23)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	// A committed-but-unpushed fixup: clean porcelain, non-zero FETCH_HEAD..HEAD.
	if err := os.WriteFile(filepath.Join(fx.wtPath, "fixup.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write fixup.txt: %v", err)
	}
	gitT(t, fx.wtPath, "add", "fixup.txt")
	gitT(t, fx.wtPath, "commit", "-m", "local fixup")

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "MERGED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if _, err := os.Stat(fx.wtPath); err != nil {
		t.Errorf("worktree removed despite unpushed-commit gate: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 1 {
		t.Errorf("task row deleted despite unpushed-commit gate: count=%d", n)
	}
}

// --- TestReconcileMergedStashSkips ---------------------------------------------

// A MERGED PR with a stash in the repo -> the (repo-global) stash gate trips:
// worktree AND row both REMAIN (GHCLN-02, Pitfall 2).
func TestReconcileMergedStashSkips(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 31)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	// Create a stash IN THE WORKTREE (stash is repo-global so this is visible to
	// StashCount). Modify a tracked file, then stash it -> clean tree + a stash
	// entry.
	if err := os.WriteFile(filepath.Join(fx.wtPath, "file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}
	gitT(t, fx.wtPath, "stash", "push", "-m", "wip")

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "MERGED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if _, err := os.Stat(fx.wtPath); err != nil {
		t.Errorf("worktree removed despite stash gate: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 1 {
		t.Errorf("task row deleted despite stash gate: count=%d", n)
	}
}

// --- TestReconcileOpenUntouched ------------------------------------------------

// An OPEN PR is never cleaned: PRState is read but only MERGED/CLOSED proceed
// (D-03). Worktree + row both remain.
func TestReconcileOpenUntouched(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 5)
	linkProjectRepo(t, db, fx.repoDir)
	insertPRTask(t, db, 1, fx.prN, fx.wtPath)

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, fx.prN): "OPEN"}}
	r := newReconciler(db, spy)
	r.reconcilePRsOnce(context.Background())

	if !spy.called(fx.repoDir, fx.prN) {
		t.Errorf("PRState was not read for the OPEN PR task")
	}
	if _, err := os.Stat(fx.wtPath); err != nil {
		t.Errorf("OPEN PR worktree was removed: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 1 {
		t.Errorf("OPEN PR task row was deleted: count=%d", n)
	}
}

// --- TestReconcileManualNeverTouched -------------------------------------------

// A source='manual' task with a worktree is NEVER selected by the PR pass: it
// triggers ZERO PRState calls and is never cleaned (D-87 guard).
func TestReconcileManualNeverTouched(t *testing.T) {
	db := newReaperDB(t)
	fx := makePRFixture(t, 99)
	linkProjectRepo(t, db, fx.repoDir)
	insertManualTask(t, db, 1, fx.wtPath)

	spy := &spyPRState{states: map[string]string{prKey(fx.repoDir, 99): "MERGED"}}
	newReconciler(db, spy).reconcilePRsOnce(context.Background())

	if len(spy.calls) != 0 {
		t.Errorf("manual task triggered PRState calls: %v (D-87 violated)", spy.calls)
	}
	if _, err := os.Stat(fx.wtPath); err != nil {
		t.Errorf("manual task worktree was removed: stat err=%v", err)
	}
	if n := countTasks(t, db, 1); n != 1 {
		t.Errorf("manual task row was deleted: count=%d", n)
	}
}

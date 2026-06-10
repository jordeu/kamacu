package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kangent/internal/session"
	"kangent/internal/store"
	"kangent/internal/worktree"
)

// newWorktreeServer wires the full backend surface the worktree endpoints
// need: task routes (provisioning), session routes, and the worktree routes,
// all sharing one DB, one worktree.Service, and one session.Manager.
func newWorktreeServer(t *testing.T) (*httptest.Server, *testWorktreeEnv) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wt := worktree.NewService(t.TempDir())
	mgr := session.NewManager()
	mux := http.NewServeMux()
	Routes(mux, db, wt)
	WorktreeRoutes(mux, db, wt, mgr)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		for _, info := range mgr.List() {
			if s, ok := mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
		db.Close()
	})
	return srv, &testWorktreeEnv{db: db, wt: wt, mgr: mgr}
}

type testWorktreeEnv struct {
	db  *sql.DB
	wt  *worktree.Service
	mgr *session.Manager
}

// gitIn runs git in dir with throwaway identity + isolated config.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// provisionedTask creates a project on a healthy repo plus a task, returning
// (taskID, repoPath, worktreePath, branch). Fails the test if provisioning
// did not succeed.
func provisionedTask(t *testing.T, srv *httptest.Server, title string) (int64, string, string, string) {
	t.Helper()
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)
	body := createTask(t, srv, pid, title)
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	branch, _ := body["branch"].(string)
	if wtPath == "" || branch == "" {
		t.Fatalf("task provisioning failed: %v", body)
	}
	return id, repo, wtPath, branch
}

func wtURL(srv *httptest.Server, id int64) string {
	return fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, id)
}

func TestWorktreeCreateRetryAfterFailure(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	repo := gitRepo(t) // unborn HEAD — provisioning fails at creation
	pid := createProject(t, srv, repo)
	body := createTask(t, srv, pid, "Retry Me")
	id := taskID(t, body)
	if body["worktree_error"] == nil {
		t.Fatalf("precondition: expected worktree_error on unborn repo, got %v", body)
	}

	// Heal the repo (first commit), then Retry via POST (D-25 Retry path).
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitIn(t, repo, "add", "f.txt")
	gitIn(t, repo, "commit", "-m", "first")

	status, body := doJSON(t, "POST", wtURL(srv, id), nil)
	if status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200; body=%v", status, body)
	}
	if body["worktree_error"] != nil {
		t.Errorf("worktree_error = %v, want null after successful retry", body["worktree_error"])
	}
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("worktree_path = %v, want set after retry", body["worktree_path"])
	}
	if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
		t.Errorf("worktree dir missing after retry: %s (%v)", wtPath, err)
	}
}

func TestWorktreeCreateLazyForLegacyTask(t *testing.T) {
	srv, env := newWorktreeServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)
	// Phase-1-era task: inserted directly, no provisioning ran (D-26).
	res, err := env.db.Exec(
		`INSERT INTO tasks (project_id, title, position) VALUES (?, 'Old Task', 1.0)`, pid)
	if err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}
	id, _ := res.LastInsertId()

	status, body := doJSON(t, "POST", wtURL(srv, id), nil)
	if status != http.StatusOK {
		t.Fatalf("lazy create status = %d, want 200; body=%v", status, body)
	}
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" || body["worktree_error"] != nil {
		t.Fatalf("lazy create body = %v, want worktree_path set and no error", body)
	}
	if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
		t.Errorf("worktree dir missing: %s (%v)", wtPath, err)
	}
}

func TestWorktreeCreateIdempotent(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	id, _, wtPath, branch := provisionedTask(t, srv, "Already There")

	status, body := doJSON(t, "POST", wtURL(srv, id), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["worktree_path"] != wtPath || body["branch"] != branch {
		t.Errorf("idempotent POST changed worktree: got %v/%v, want %v/%v",
			body["worktree_path"], body["branch"], wtPath, branch)
	}
}

func TestWorktreeCreateUnknownTask(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	status, body := doJSON(t, "POST", wtURL(srv, 424242), nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%v", status, body)
	}
}

func TestWorktreeRecreateReusesKeptBranch(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	id, repo, _, branch := provisionedTask(t, srv, "Round Trip")

	// Clean removal — branch deliberately survives (D-34).
	status, body := doJSON(t, "DELETE", wtURL(srv, id), map[string]any{})
	if status != http.StatusNoContent {
		t.Fatalf("cleanup status = %d, want 204; body=%v", status, body)
	}
	if got := branchList(t, repo, branch); got == "" {
		t.Fatalf("branch %s gone after cleanup — D-34 violated", branch)
	}

	// Re-create must reuse the kept branch, not fail "branch already exists".
	status, body = doJSON(t, "POST", wtURL(srv, id), nil)
	if status != http.StatusOK {
		t.Fatalf("re-create status = %d, want 200; body=%v", status, body)
	}
	if body["worktree_error"] != nil {
		t.Errorf("re-create worktree_error = %v, want null (branch reuse path)", body["worktree_error"])
	}
	if body["branch"] != branch {
		t.Errorf("re-create branch = %v, want reused %q", body["branch"], branch)
	}
}

func TestWorktreeGet(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, _, wtPath, branch := provisionedTask(t, srv, "Inspect Me")

	status, body := doJSON(t, "GET", wtURL(srv, id), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["branch"] != branch || body["path"] != wtPath {
		t.Errorf("branch/path = %v/%v, want %v/%v", body["branch"], body["path"], branch, wtPath)
	}
	if body["dirty_files"] != float64(0) {
		t.Errorf("dirty_files = %v, want 0 on clean tree", body["dirty_files"])
	}
	if body["running_sessions"] != float64(0) {
		t.Errorf("running_sessions = %v, want 0", body["running_sessions"])
	}

	// Two untracked files → dirty_files 2 (untracked counts; Pitfall 2).
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(wtPath, name), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	_, body = doJSON(t, "GET", wtURL(srv, id), nil)
	if body["dirty_files"] != float64(2) {
		t.Errorf("dirty_files = %v, want 2 after two untracked files", body["dirty_files"])
	}

	// A running task session → running_sessions 1.
	sess, err := env.mgr.Spawn(session.SpawnOpts{Cwd: wtPath, TaskID: id})
	if err != nil {
		t.Fatalf("spawn task session: %v", err)
	}
	defer sess.Stop()
	_, body = doJSON(t, "GET", wtURL(srv, id), nil)
	if body["running_sessions"] != float64(1) {
		t.Errorf("running_sessions = %v, want 1", body["running_sessions"])
	}
}

func TestWorktreeGetNoWorktree(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	pid := createProject(t, srv, gitRepo(t)) // unborn → no worktree
	id := taskID(t, createTask(t, srv, pid, "No Tree"))

	status, body := doJSON(t, "GET", wtURL(srv, id), nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "task has no worktree" {
		t.Errorf("error = %q, want %q", body["error"], "task has no worktree")
	}
}

func TestWorktreeDeleteSessionsGate(t *testing.T) {
	srv, env := newWorktreeServer(t)
	id, repo, wtPath, branch := provisionedTask(t, srv, "Guard Sessions")

	sess, err := env.mgr.Spawn(session.SpawnOpts{Cwd: wtPath, TaskID: id})
	if err != nil {
		t.Fatalf("spawn task session: %v", err)
	}

	// Running session + no flags → 409, worktree untouched (D-32).
	status, body := doJSON(t, "DELETE", wtURL(srv, id), map[string]any{})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "sessions running" {
		t.Errorf("error = %q, want %q", body["error"], "sessions running")
	}
	if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
		t.Fatalf("worktree dir must be intact after refused cleanup: %v", err)
	}

	// Explicit stop gate → 204; session exited; dir gone; BRANCH KEPT (GIT-02/D-34).
	status, body = doJSON(t, "DELETE", wtURL(srv, id), map[string]any{"stop_sessions": true})
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%v", status, body)
	}
	if got := sess.Info().Status; got != session.StatusExited {
		t.Errorf("session status = %q, want exited after stop_sessions cleanup", got)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still present after cleanup: %v", err)
	}
	if got := branchList(t, repo, branch); got == "" {
		t.Errorf("branch %s deleted by cleanup — must ALWAYS be kept (D-34)", branch)
	}

	// Task lands in the D-26 absent state: all three columns NULL.
	status, task := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("task get status = %d; body=%v", status, task)
	}
	for _, key := range []string{"branch", "worktree_path", "worktree_error"} {
		if task[key] != nil {
			t.Errorf("task.%s = %v, want null after cleanup", key, task[key])
		}
	}
}

func TestWorktreeDeleteDirtyGate(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	id, repo, wtPath, branch := provisionedTask(t, srv, "Guard Dirt")

	if err := os.WriteFile(filepath.Join(wtPath, "wip.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Dirty + no force → 409 (D-33), worktree intact.
	status, body := doJSON(t, "DELETE", wtURL(srv, id), map[string]any{})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "worktree has uncommitted changes" {
		t.Errorf("error = %q, want %q", body["error"], "worktree has uncommitted changes")
	}
	if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
		t.Fatalf("worktree dir must be intact after refused cleanup: %v", err)
	}

	// Force → 204, dir gone, branch kept (GIT-03).
	status, body = doJSON(t, "DELETE", wtURL(srv, id), map[string]any{"force": true})
	if status != http.StatusNoContent {
		t.Fatalf("force status = %d, want 204; body=%v", status, body)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still present after forced cleanup")
	}
	if got := branchList(t, repo, branch); got == "" {
		t.Errorf("branch %s deleted by forced cleanup — must be kept (D-34)", branch)
	}
}

func TestWorktreeDeleteNoWorktree(t *testing.T) {
	srv, _ := newWorktreeServer(t)
	pid := createProject(t, srv, gitRepo(t)) // unborn → no worktree
	id := taskID(t, createTask(t, srv, pid, "Nothing To Clean"))

	status, body := doJSON(t, "DELETE", wtURL(srv, id), nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%v", status, body)
	}
}

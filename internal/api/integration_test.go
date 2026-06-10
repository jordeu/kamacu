package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kangent/internal/session"
	"kangent/internal/store"
	"kangent/internal/worktree"
)

// newIntegrationServer wires ALL route families over one DB, one
// worktree.Service, and one session.Manager — the exact production shape of
// cmd/kangent/main.go (Routes, then SessionRoutes, then WorktreeRoutes). If
// the production wiring drifts, this harness catches it.
func newIntegrationServer(t *testing.T) (*httptest.Server, *session.Manager) {
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
	SessionRoutes(mux, mgr, db)
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
	return srv, mgr
}

// TestIntegrationWorktreeLifecycle proves Phase 3 end to end over the real
// REST surface: task creation provisions a worktree+branch (GIT-01), a
// task-scoped bash session runs inside it (TERM-04), cleanup is gated on
// running sessions and dirty trees (GIT-03), removal always keeps the branch
// (GIT-02/D-34), and re-creation reuses the kept branch.
func TestIntegrationWorktreeLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	srv, mgr := newIntegrationServer(t)

	// The repo must be created on the OUTER t: a subtest's t.TempDir is
	// removed when the subtest ends, which would delete the repo (and its
	// .git/worktrees bookkeeping) out from under the later stages.
	repo := gitRepoWithCommit(t)

	var (
		taskID64  int64
		wtPath    string
		branch    string
		sessionID string
	)

	// stage runs the named subtest and aborts the lifecycle on failure —
	// later stages depend on earlier state.
	stage := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	stage("1_task_creation_provisions_worktree", func(t *testing.T) {
		pid := createProject(t, srv, repo)
		body := createTask(t, srv, pid, "Fix Login")
		taskID64 = taskID(t, body)

		wtPath, _ = body["worktree_path"].(string)
		if wtPath == "" {
			t.Fatalf("worktree_path = %v, want non-empty (GIT-01)", body["worktree_path"])
		}
		branch, _ = body["branch"].(string)
		wantBranch := fmt.Sprintf("task/fix-login-%d", taskID64)
		if branch != wantBranch {
			t.Fatalf("branch = %q, want collision-safe slug+id %q (GIT-01)", branch, wantBranch)
		}
		if body["worktree_error"] != nil {
			t.Fatalf("worktree_error = %v, want null", body["worktree_error"])
		}
		if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
			t.Fatalf("worktree dir missing on disk: %s (%v)", wtPath, err)
		}
		// The worktree lives OUTSIDE the repo tree (D-23): the relative path
		// from the repo to the worktree must escape upward.
		rel, err := filepath.Rel(repo, wtPath)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			t.Fatalf("worktree %s is inside the repo tree %s (rel %q, err %v)", wtPath, repo, rel, err)
		}
		if got := branchList(t, repo, "task/fix-login-*"); got == "" {
			t.Fatalf("git branch --list task/fix-login-* empty — branch not created (GIT-01)")
		}
	})

	stage("2_task_scoped_session_runs_in_worktree", func(t *testing.T) {
		status, body := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"task_id": taskID64})
		if status != http.StatusCreated {
			t.Fatalf("spawn status = %d, want 201; body=%v", status, body)
		}
		if body["label"] != "Bash 1" {
			t.Fatalf("label = %q, want %q (TERM-04 per-task counter)", body["label"], "Bash 1")
		}
		sessionID, _ = body["id"].(string)
		if sessionID == "" {
			t.Fatalf("session id missing: %v", body)
		}

		// Behavioral cwd proof: the shell expands $PWD; the echoed input only
		// ever contains the literal "$PWD", so a marker match cannot be
		// satisfied by command echo.
		sess, ok := mgr.Get(sessionID)
		if !ok {
			t.Fatalf("session %q not in manager", sessionID)
		}
		if err := sess.WriteInput([]byte("echo \"mark:$PWD\"\n")); err != nil {
			t.Fatalf("write input: %v", err)
		}
		want := []byte("mark:" + wtPath)
		deadline := time.Now().Add(10 * time.Second)
		for !bytes.Contains(sess.Snapshot(), want) {
			if time.Now().After(deadline) {
				t.Fatalf("shell cwd is not the worktree: %q never appeared in output:\n%s", want, sess.Snapshot())
			}
			time.Sleep(50 * time.Millisecond)
		}

		// The filtered list shows it running (TERM-04).
		status, list := doJSONList(t, fmt.Sprintf("%s/api/sessions?task_id=%d", srv.URL, taskID64))
		if status != http.StatusOK {
			t.Fatalf("filtered list status = %d, want 200", status)
		}
		if len(list) != 1 || list[0]["id"] != sessionID {
			t.Fatalf("filtered list = %v, want exactly the spawned session", list)
		}
		if list[0]["status"] != string(session.StatusRunning) {
			t.Fatalf("session status = %v, want running", list[0]["status"])
		}
	})

	stage("3_worktree_state_reports_dirt_and_sessions", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(wtPath, "wip.txt"), []byte("uncommitted\n"), 0o644); err != nil {
			t.Fatalf("write untracked file: %v", err)
		}
		status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, taskID64), nil)
		if status != http.StatusOK {
			t.Fatalf("GET worktree status = %d, want 200; body=%v", status, body)
		}
		dirty, _ := body["dirty_files"].(float64)
		if dirty < 1 {
			t.Errorf("dirty_files = %v, want >= 1 after untracked file", body["dirty_files"])
		}
		if body["running_sessions"] != float64(1) {
			t.Errorf("running_sessions = %v, want 1", body["running_sessions"])
		}
	})

	stage("4_cleanup_gates_compose", func(t *testing.T) {
		wtURL := fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, taskID64)

		// Running session + no flags → refused (D-32).
		status, body := doJSON(t, "DELETE", wtURL, map[string]any{})
		if status != http.StatusConflict {
			t.Fatalf("bare DELETE status = %d, want 409; body=%v", status, body)
		}
		if body["error"] != "sessions running" {
			t.Errorf("error = %q, want %q", body["error"], "sessions running")
		}

		// stop_sessions alone is not enough on a dirty tree (D-33, GIT-03):
		// the dirty gate is re-checked server-side and refuses BEFORE any
		// session is stopped or anything is removed.
		status, body = doJSON(t, "DELETE", wtURL, map[string]any{"stop_sessions": true})
		if status != http.StatusConflict {
			t.Fatalf("stop_sessions DELETE on dirty tree status = %d, want 409; body=%v", status, body)
		}
		if body["error"] != "worktree has uncommitted changes" {
			t.Errorf("error = %q, want %q", body["error"], "worktree has uncommitted changes")
		}

		// Both refusals left the worktree intact.
		if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
			t.Fatalf("worktree dir must be intact after refused cleanups: %v", err)
		}
	})

	stage("5_forced_cleanup_keeps_branch", func(t *testing.T) {
		wtURL := fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, taskID64)
		status, body := doJSON(t, "DELETE", wtURL, map[string]any{"stop_sessions": true, "force": true})
		if status != http.StatusNoContent {
			t.Fatalf("forced DELETE status = %d, want 204; body=%v", status, body)
		}

		// The task session was stopped before removal (Pitfall 4 ordering).
		sess, ok := mgr.Get(sessionID)
		if !ok {
			t.Fatalf("session %q vanished from manager", sessionID)
		}
		if got := sess.Info().Status; got != session.StatusExited {
			t.Errorf("session status = %q, want exited after stop_sessions cleanup", got)
		}

		// Dir gone, branch STILL there (GIT-02/D-34).
		if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
			t.Errorf("worktree dir still present after cleanup: %v", err)
		}
		if got := branchList(t, repo, branch); got == "" {
			t.Fatalf("branch %s deleted by cleanup — must ALWAYS be kept (GIT-02/D-34)", branch)
		}

		// Task lands in the D-26 absent state: all three fields null.
		status, task := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, taskID64), nil)
		if status != http.StatusOK {
			t.Fatalf("task get status = %d; body=%v", status, task)
		}
		for _, key := range []string{"branch", "worktree_path", "worktree_error"} {
			if task[key] != nil {
				t.Errorf("task.%s = %v, want null after cleanup", key, task[key])
			}
		}
	})

	stage("6_recreate_reuses_kept_branch", func(t *testing.T) {
		status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, taskID64), nil)
		if status != http.StatusOK {
			t.Fatalf("re-create status = %d, want 200; body=%v", status, body)
		}
		if body["worktree_error"] != nil {
			t.Fatalf("worktree_error = %v, want null (kept-branch reuse path)", body["worktree_error"])
		}
		if body["branch"] != branch {
			t.Errorf("re-create branch = %v, want reused %q", body["branch"], branch)
		}
		newPath, _ := body["worktree_path"].(string)
		if newPath == "" {
			t.Fatalf("worktree_path = %v, want set after re-create", body["worktree_path"])
		}
		if fi, err := os.Stat(newPath); err != nil || !fi.IsDir() {
			t.Errorf("re-created worktree dir missing: %s (%v)", newPath, err)
		}
	})
}

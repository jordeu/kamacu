package api

// GET /api/tasks/{id}/diff over httptest, on Phase-3 fixture repos (outer-t git
// repo with isolated config, real SQLite in t.TempDir(), worktree provisioned
// through the real REST create flow). The structured JSON contract and the
// honest error relays (Pitfall 7) are asserted end to end.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/store"
	"kangent/internal/worktree"
)

// newDiffServer wires the diff route alongside the worktree + task routes over
// one DB / worktree.Service / Manager, mirroring production registration.
func newDiffServer(t *testing.T) (*httptest.Server, *worktree.Service, *session.Manager) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	wtDir := t.TempDir()
	// Phase 6: provisioning reads worktree_base at use — seed the temp dir.
	if err := settings.Set(db, settings.KeyWorktreeBase, wtDir); err != nil {
		t.Fatalf("seed worktree_base: %v", err)
	}
	wt := worktree.NewService(wtDir)
	mgr := session.NewManager()
	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr)
	WorktreeRoutes(mux, db, wt, mgr)
	DiffRoutes(mux, db, wt)
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
	return srv, wt, mgr
}

// provisionTaskWithWorktree creates a project + task and provisions the
// worktree through the real POST flow, returning the task id and worktree path.
func provisionTaskWithWorktree(t *testing.T, srv *httptest.Server, repo string) (int64, string) {
	t.Helper()
	pid := createProject(t, srv, repo)
	body := createTask(t, srv, pid, "Review Me")
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("task %d has no worktree_path: %v", id, body)
	}
	return id, wtPath
}

func TestDiffModifiedAndUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	repo := gitRepoWithCommit(t) // main with file.txt committed
	id, wtPath := provisionTaskWithWorktree(t, srv, repo)

	// One modified tracked file, one untracked file.
	if err := os.WriteFile(filepath.Join(wtPath, "file.txt"), []byte("hello\nWORLD\n"), 0o644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "newfile.txt"), []byte("fresh\n"), 0o644); err != nil {
		t.Fatalf("write newfile.txt: %v", err)
	}

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET diff status = %d, want 200; body=%v", status, body)
	}
	if base, _ := body["base"].(string); base == "" {
		t.Errorf("base = %v, want non-empty", body["base"])
	}
	totals, _ := body["totals"].(map[string]any)
	if totals == nil {
		t.Fatalf("totals missing: %v", body)
	}
	if tf, _ := totals["files"].(float64); tf != 2 {
		t.Errorf("totals.files = %v, want 2", totals["files"])
	}
	files, _ := body["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files = %d entries, want 2: %v", len(files), files)
	}
	// Files sorted by path: file.txt before newfile.txt.
	first, _ := files[0].(map[string]any)
	second, _ := files[1].(map[string]any)
	if first["path"] != "file.txt" || second["path"] != "newfile.txt" {
		t.Errorf("paths = %v,%v want file.txt,newfile.txt (sorted)", first["path"], second["path"])
	}
	if second["status"] != "new" {
		t.Errorf("newfile.txt status = %v, want new", second["status"])
	}
}

func TestDiffPristine(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	repo := gitRepoWithCommit(t)
	id, _ := provisionTaskWithWorktree(t, srv, repo)

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET diff status = %d, want 200; body=%v", status, body)
	}
	// files must be present and an empty array (non-null) per D-63.
	files, ok := body["files"].([]any)
	if !ok {
		t.Fatalf("files not an array (must be [], not null): %v", body["files"])
	}
	if len(files) != 0 {
		t.Errorf("files = %v, want empty", files)
	}
	totals, _ := body["totals"].(map[string]any)
	if totals["files"] != float64(0) || totals["additions"] != float64(0) || totals["deletions"] != float64(0) {
		t.Errorf("totals = %v, want all zero", totals)
	}
}

func TestDiffNoWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)
	body := createTask(t, srv, pid, "Has Worktree")
	id := taskID(t, body)

	// Remove the worktree so the task has worktree_path = NULL.
	wtURL := fmt.Sprintf("%s/api/tasks/%d/worktree", srv.URL, id)
	status, _ := doJSON(t, "DELETE", wtURL, map[string]any{"force": true})
	if status != http.StatusNoContent {
		t.Fatalf("DELETE worktree status = %d, want 204", status)
	}

	status, dbody := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusConflict {
		t.Fatalf("GET diff (no worktree) status = %d, want 409; body=%v", status, dbody)
	}
	if dbody["error"] != "task has no worktree" {
		t.Errorf("error = %q, want %q (shared gate copy)", dbody["error"], "task has no worktree")
	}
}

func TestDiffUnknownTask(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, 999999), nil)
	if status != http.StatusNotFound {
		t.Fatalf("GET diff (unknown task) status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "task not found" {
		t.Errorf("error = %q, want %q", body["error"], "task not found")
	}
}

func TestDiffMissingWorktreeDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	repo := gitRepoWithCommit(t)
	id, wtPath := provisionTaskWithWorktree(t, srv, repo)

	// Manually delete the worktree directory (Pitfall 7): the task still
	// records the path, but the dir is gone.
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("remove worktree dir: %v", err)
	}

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("GET diff (missing dir) status = %d, want 500; body=%v", status, body)
	}
	if msg, _ := body["error"].(string); msg == "" {
		t.Errorf("error message empty, want a relayed message for the UI error card")
	}
}

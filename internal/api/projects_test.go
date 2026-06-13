package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/store"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
)

// newTestServer opens an isolated SQLite database in a temp dir, migrates it,
// and returns an httptest server with all API routes registered, plus the DB
// handle (for direct assertions) and the DB file path (for reopen tests).
//
// The worktree_base setting is seeded with the service's temp dir: since
// Phase 6 the creation path reads the setting (absent row = the real
// ~/.kangent default), so an unseeded harness would write into the
// developer's home. Every provisioning harness in this package does the same.
func newTestServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
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
	if err := settings.Set(db, settings.KeyWorktreeBase, wtDir); err != nil {
		t.Fatalf("seed worktree_base: %v", err)
	}
	mux := http.NewServeMux()
	Routes(mux, db, worktree.NewService(wtDir), session.NewManager(), tmux.Client{})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return srv, db, dbPath
}

// gitRepo creates a temp dir and runs `git init` in it, returning the path.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
	return dir
}

// doJSON issues an HTTP request with a JSON body and decodes the JSON
// response into a map (nil body / non-JSON responses yield an empty map).
func doJSON(t *testing.T, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := map[string]any{}
	if len(raw) > 0 {
		// Some endpoints return arrays; callers that need arrays use doJSONList.
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

// doJSONList issues a GET and decodes the response as a JSON array.
func doJSONList(t *testing.T, url string) (int, []map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode array from %s: %v\n%s", url, err, raw)
	}
	return resp.StatusCode, out
}

// createProject POSTs a project for the given repo path and returns its id.
func createProject(t *testing.T, srv *httptest.Server, repoPath string) int64 {
	t.Helper()
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": repoPath})
	if status != http.StatusCreated {
		t.Fatalf("create project: status=%d body=%v", status, body)
	}
	id, ok := body["id"].(float64)
	if !ok {
		t.Fatalf("create project: no numeric id in %v", body)
	}
	return int64(id)
}

func TestProjectCreateValidRepo(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": repo})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%v", status, body)
	}
	wantName := filepath.Base(repo)
	if body["name"] != wantName {
		t.Errorf("name = %q, want default %q", body["name"], wantName)
	}
	if body["repo_path"] != repo {
		t.Errorf("repo_path = %q, want %q", body["repo_path"], repo)
	}
	if body["created_at"] == "" || body["created_at"] == nil {
		t.Errorf("created_at missing in response: %v", body)
	}
}

func TestProjectCreateRelativePath(t *testing.T) {
	srv, _, _ := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": "relative/path"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "path must be absolute" {
		t.Errorf("error = %q, want %q", body["error"], "path must be absolute")
	}
}

func TestProjectCreateNotGitRepo(t *testing.T) {
	srv, _, _ := newTestServer(t)
	dir := t.TempDir() // exists, is a directory, but no git repo

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": dir})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%v", status, body)
	}
	msg, _ := body["error"].(string)
	if !bytes.Contains([]byte(msg), []byte("not a git repository")) {
		t.Errorf("error = %q, want it to contain %q", msg, "not a git repository")
	}
}

func TestProjectCreateDuplicate(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)

	createProject(t, srv, repo)
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": repo})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "this repository is already added" {
		t.Errorf("error = %q, want %q", body["error"], "this repository is already added")
	}
}

func TestProjectRename(t *testing.T) {
	srv, _, _ := newTestServer(t)
	id := createProject(t, srv, gitRepo(t))

	status, body := doJSON(t, "PATCH", fmt.Sprintf("%s/api/projects/%d", srv.URL, id), map[string]any{"name": "renamed"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["name"] != "renamed" {
		t.Errorf("name = %q, want %q", body["name"], "renamed")
	}

	status, body = doJSON(t, "PATCH", fmt.Sprintf("%s/api/projects/%d", srv.URL, id), map[string]any{"name": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty name: status = %d, want 400; body=%v", status, body)
	}
}

// TestUpdateProjectPartial exercises the grown PATCH: partial updates of
// description and github_repo, canonicalization, unlink, and the single
// hard-error case (a syntactically invalid ref). github_repo cases use refs
// that ParseRepoRef accepts syntactically so the test is deterministic whether
// or not gh is installed (ValidateRepo soft-saves the syntactic form).
func TestUpdateProjectPartial(t *testing.T) {
	srv, db, _ := newTestServer(t)
	id := createProject(t, srv, gitRepo(t))
	url := fmt.Sprintf("%s/api/projects/%d", srv.URL, id)

	// A fresh project serializes both new keys: description "" and github_repo null.
	status, body := doJSONList(t, srv.URL+"/api/projects")
	if status != http.StatusOK {
		t.Fatalf("list status = %d, want 200", status)
	}
	if len(body) != 1 {
		t.Fatalf("len = %d, want 1", len(body))
	}
	if _, ok := body[0]["description"]; !ok {
		t.Errorf("list project missing 'description' key: %v", body[0])
	}
	if v, ok := body[0]["github_repo"]; !ok || v != nil {
		t.Errorf("fresh github_repo = %v (ok=%v), want present and null", v, ok)
	}

	// name-only PATCH still renames and leaves description/github_repo untouched.
	status, pb := doJSON(t, "PATCH", url, map[string]any{"name": "renamed"})
	if status != http.StatusOK {
		t.Fatalf("rename status = %d, want 200; body=%v", status, pb)
	}
	if pb["name"] != "renamed" {
		t.Errorf("name = %q, want renamed", pb["name"])
	}
	if pb["description"] != "" {
		t.Errorf("description after name-only PATCH = %v, want \"\"", pb["description"])
	}

	// description set.
	status, pb = doJSON(t, "PATCH", url, map[string]any{"description": "hello"})
	if status != http.StatusOK {
		t.Fatalf("set description status = %d; body=%v", status, pb)
	}
	if pb["description"] != "hello" {
		t.Errorf("description = %v, want hello", pb["description"])
	}
	assertDBProject(t, db, id, "hello", nil)

	// description clear.
	status, pb = doJSON(t, "PATCH", url, map[string]any{"description": ""})
	if status != http.StatusOK {
		t.Fatalf("clear description status = %d; body=%v", status, pb)
	}
	if pb["description"] != "" {
		t.Errorf("description after clear = %v, want \"\"", pb["description"])
	}

	// github_repo canonicalized from a URL to owner/name.
	status, pb = doJSON(t, "PATCH", url, map[string]any{"github_repo": "https://github.com/cli/cli.git"})
	if status != http.StatusOK {
		t.Fatalf("link status = %d; body=%v", status, pb)
	}
	if pb["github_repo"] != "cli/cli" {
		t.Errorf("github_repo = %v, want cli/cli", pb["github_repo"])
	}
	repo := "cli/cli"
	assertDBProject(t, db, id, "", &repo)

	// github_repo "" → unlink (NULL / JSON null).
	status, pb = doJSON(t, "PATCH", url, map[string]any{"github_repo": ""})
	if status != http.StatusOK {
		t.Fatalf("unlink status = %d; body=%v", status, pb)
	}
	if v, ok := pb["github_repo"]; !ok || v != nil {
		t.Errorf("github_repo after unlink = %v (ok=%v), want null", v, ok)
	}
	assertDBProject(t, db, id, "", nil)

	// invalid ref → 400, canonical error, row unchanged. Link first so we can
	// prove the bad PATCH does not clobber the stored value.
	status, _ = doJSON(t, "PATCH", url, map[string]any{"github_repo": "owner/name"})
	if status != http.StatusOK {
		t.Fatalf("seed link status = %d, want 200", status)
	}
	status, pb = doJSON(t, "PATCH", url, map[string]any{"github_repo": "not-a-repo"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid ref status = %d, want 400; body=%v", status, pb)
	}
	if pb["error"] != "Not a valid repository — use owner/name or a GitHub URL." {
		t.Errorf("error = %q, want canonical invalid-ref copy", pb["error"])
	}
	seeded := "owner/name"
	assertDBProject(t, db, id, "", &seeded) // unchanged by the rejected PATCH

	// empty body → 400 nothing to update.
	status, pb = doJSON(t, "PATCH", url, map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("empty body status = %d, want 400; body=%v", status, pb)
	}

	// 404 for a bogus id.
	status, _ = doJSON(t, "PATCH", fmt.Sprintf("%s/api/projects/999999", srv.URL), map[string]any{"name": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("bogus id status = %d, want 404", status)
	}
}

// assertDBProject asserts the persisted description and github_repo for a project.
// wantRepo nil means the column must be NULL.
func assertDBProject(t *testing.T, db *sql.DB, id int64, wantDesc string, wantRepo *string) {
	t.Helper()
	var desc string
	var repo sql.NullString
	if err := db.QueryRow(`SELECT description, github_repo FROM projects WHERE id = ?`, id).Scan(&desc, &repo); err != nil {
		t.Fatalf("read back project %d: %v", id, err)
	}
	if desc != wantDesc {
		t.Errorf("DB description = %q, want %q", desc, wantDesc)
	}
	if wantRepo == nil {
		if repo.Valid {
			t.Errorf("DB github_repo = %q, want NULL", repo.String)
		}
	} else {
		if !repo.Valid || repo.String != *wantRepo {
			t.Errorf("DB github_repo = %v (valid=%v), want %q", repo.String, repo.Valid, *wantRepo)
		}
	}
}

func TestProjectDeleteCascadesAndKeepsRepo(t *testing.T) {
	srv, db, _ := newTestServer(t)
	repo := gitRepo(t)
	id := createProject(t, srv, repo)

	// Insert a task directly so we can prove the cascade.
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, position) VALUES (?, 'doomed', 1.0)`, id); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/projects/%d", srv.URL, id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if n != 0 {
		t.Errorf("tasks remaining after project delete = %d, want 0 (cascade)", n)
	}

	if _, err := os.Stat(repo); err != nil {
		t.Errorf("repo dir gone after project delete: %v (must be untouched)", err)
	}
}

func TestProjectList(t *testing.T) {
	srv, _, _ := newTestServer(t)
	createProject(t, srv, gitRepo(t))
	createProject(t, srv, gitRepo(t))

	status, list := doJSONList(t, srv.URL+"/api/projects")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2: %v", len(list), list)
	}
}

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

	"context"

	"kamacu/internal/github"
	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// newRepoTestServer is like newTestServer but also points the managed-clone base
// (~/.kangent/repos/) at an isolated temp dir via the HOME env so the repo-first
// create path (which hardcodes ExpandHome("~/.kangent/repos/")) never writes into
// the developer's real home. It returns the server, DB, and the repos base dir.
func newRepoTestServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home) // ExpandHome resolves ~ via os.UserHomeDir → HOME
	srv, db, _ := newTestServer(t)
	return srv, db, filepath.Join(home, ".kangent", "repos")
}

// countProjects returns the number of rows in projects (for atomicity asserts).
func countProjects(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	return n
}

// failCloneRunner installs a clone seam that records whether it was invoked and
// always fails; the test fails immediately if it is called when it must not be.
func failCloneNeverCalled(t *testing.T) func() {
	t.Helper()
	return github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		t.Errorf("clone runner invoked for %q → %q but it must NOT be (validate-before-clone / reattach)", ref, dest)
		return "", fmt.Errorf("clone must not run")
	})
}

// TestCreateRepoValidateBeforeClone (RPROJ-05): a syntactically invalid ref and
// a gh-unverified ref are both rejected 400 with NO clone attempted and NO row
// created. The clone seam fails the test if invoked.
func TestCreateRepoValidateBeforeClone(t *testing.T) {
	srv, db, _ := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	// gh present so the verified leg runs, but the validator says "not verified".
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, false, nil // never verified
	})()

	// Syntactically invalid ref → 400, no clone, no row (ParseRepoRef hard-reject).
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "-bad/owner"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid ref: status = %d, want 400; body=%v", status, body)
	}
	if n := countProjects(t, db); n != 0 {
		t.Fatalf("rows after invalid ref = %d, want 0", n)
	}

	// gh present but repo not verified → 400 msgRepoNotFound, no clone, no row.
	status, body = doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "octocat/nope"})
	if status != http.StatusBadRequest {
		t.Fatalf("unverified repo: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != msgRepoNotFound {
		t.Errorf("error = %q, want %q", body["error"], msgRepoNotFound)
	}
	if n := countProjects(t, db); n != 0 {
		t.Fatalf("rows after unverified ref = %d, want 0", n)
	}
}

// TestCreateRepoGHUnavailable: with gh absent, the repo-first path is rejected
// with msgGHUnavailable (no clone, no row) while the folder path on the SAME
// endpoint stays fully usable (RPROJ-04 safety).
func TestCreateRepoGHUnavailable(t *testing.T) {
	srv, db, _ := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(false)()

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status != http.StatusBadRequest {
		t.Fatalf("gh-absent repo create: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != msgGHUnavailable {
		t.Errorf("error = %q, want %q", body["error"], msgGHUnavailable)
	}
	if n := countProjects(t, db); n != 0 {
		t.Fatalf("rows after gh-absent repo create = %d, want 0", n)
	}

	// Folder path still works on the same endpoint.
	repo := gitRepo(t)
	status, body = doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": repo})
	if status != http.StatusCreated {
		t.Fatalf("folder create after gh-absent repo create: status = %d, want 201; body=%v", status, body)
	}
}

// TestCreateRepoSuccess (CKOUT-01): a verified repo clones, then a row is
// created with managed=1, github_repo=canonical, repo_path under the managed
// base. The clone seam runs a real git init at dest (so reattach/diff machinery
// sees a real repo) and returns success.
func TestCreateRepoSuccess(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "Octocat/Hello-World", true, nil // gh-canonical casing wins
	})()
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		// Simulate a real clone: create dest as a git repo.
		if out, err := exec.Command("git", "init", dest).CombinedOutput(); err != nil {
			return string(out), err
		}
		return "", nil
	})()

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "octocat/hello-world"})
	if status != http.StatusCreated {
		t.Fatalf("repo create: status = %d, want 201; body=%v", status, body)
	}
	if body["managed"] != true {
		t.Errorf("managed = %v, want true", body["managed"])
	}
	if body["github_repo"] != "Octocat/Hello-World" {
		t.Errorf("github_repo = %v, want canonical Octocat/Hello-World", body["github_repo"])
	}
	wantPath := filepath.Join(base, "Octocat", "Hello-World")
	if body["repo_path"] != wantPath {
		t.Errorf("repo_path = %v, want %v", body["repo_path"], wantPath)
	}
	if body["name"] != "Hello-World" {
		t.Errorf("name = %v, want repo name Hello-World", body["name"])
	}
	// DB row persisted managed=1 + github_repo.
	var managed int
	var ghRepo sql.NullString
	if err := db.QueryRow(`SELECT managed, github_repo FROM projects WHERE repo_path = ?`, wantPath).Scan(&managed, &ghRepo); err != nil {
		t.Fatalf("read back managed row: %v", err)
	}
	if managed != 1 {
		t.Errorf("DB managed = %d, want 1", managed)
	}
	if !ghRepo.Valid || ghRepo.String != "Octocat/Hello-World" {
		t.Errorf("DB github_repo = %v, want Octocat/Hello-World", ghRepo)
	}
}

// repoSuccessSeams installs the verified-clone seams shared by the
// description-capture tests: gh present, validate returns the canonical casing,
// and clone does a real `git init` at dest. Returns a single restore func that
// tears all three down (defer it once).
func repoSuccessSeams(t *testing.T) func() {
	t.Helper()
	restoreAvail := github.SetAvailableForTest(true)
	restoreValidate := github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "Octocat/Hello-World", true, nil
	})
	restoreClone := github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		if out, err := exec.Command("git", "init", dest).CombinedOutput(); err != nil {
			return string(out), err
		}
		return "", nil
	})
	return func() {
		restoreClone()
		restoreValidate()
		restoreAvail()
	}
}

// TestCreateRepoCapturesDescription (RPROJ-02 / D-05): a repo-first create
// captures the GitHub repo's description (best-effort, server-side — never a
// dialog field) and persists it into projects.description, surfaced in the
// create response AND read back from the DB row.
func TestCreateRepoCapturesDescription(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer repoSuccessSeams(t)()
	defer github.SetDescriptionRunnerForTest(func(_ context.Context, canonical string) string {
		return "hello world"
	})()

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "octocat/hello-world"})
	if status != http.StatusCreated {
		t.Fatalf("repo create: status = %d, want 201; body=%v", status, body)
	}
	if body["description"] != "hello world" {
		t.Errorf("response description = %v, want %q", body["description"], "hello world")
	}
	// DB row persisted the captured description.
	wantPath := filepath.Join(base, "Octocat", "Hello-World")
	var desc string
	if err := db.QueryRow(`SELECT description FROM projects WHERE repo_path = ?`, wantPath).Scan(&desc); err != nil {
		t.Fatalf("read back description: %v", err)
	}
	if desc != "hello world" {
		t.Errorf("DB description = %q, want %q", desc, "hello world")
	}
}

// TestCreateRepoEmptyDescriptionStill201 (D-05 degrade-don't-break): an empty
// description read (or gh absent) must NEVER change the create outcome — still
// 201, row still managed=1 + github_repo=canonical, description "".
func TestCreateRepoEmptyDescriptionStill201(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer repoSuccessSeams(t)()
	defer github.SetDescriptionRunnerForTest(func(_ context.Context, canonical string) string {
		return "" // empty / degraded read
	})()

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "octocat/hello-world"})
	if status != http.StatusCreated {
		t.Fatalf("repo create with empty description: status = %d, want 201; body=%v", status, body)
	}
	if body["description"] != "" {
		t.Errorf("response description = %v, want empty", body["description"])
	}
	if body["managed"] != true {
		t.Errorf("managed = %v, want true (create unaffected by empty description)", body["managed"])
	}
	if body["github_repo"] != "Octocat/Hello-World" {
		t.Errorf("github_repo = %v, want canonical Octocat/Hello-World", body["github_repo"])
	}
	// DB row: empty description, still managed=1.
	wantPath := filepath.Join(base, "Octocat", "Hello-World")
	var desc string
	var managed int
	if err := db.QueryRow(`SELECT description, managed FROM projects WHERE repo_path = ?`, wantPath).Scan(&desc, &managed); err != nil {
		t.Fatalf("read back row: %v", err)
	}
	if desc != "" {
		t.Errorf("DB description = %q, want empty", desc)
	}
	if managed != 1 {
		t.Errorf("DB managed = %d, want 1", managed)
	}
}

// TestCreateRepoCloneAtomicity (CKOUT-04 / D-01): a clone failure leaves zero
// new rows and no directory at dest — never a half-created project.
func TestCreateRepoCloneAtomicity(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return parsed, true, nil
	})()
	defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
		// Simulate git creating a partial dir, then failing.
		_ = os.MkdirAll(dest, 0o755)
		return "network is unreachable", fmt.Errorf("boom")
	})()

	before := countProjects(t, db)
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status < 400 {
		t.Fatalf("clone-fail create: status = %d, want >=400; body=%v", status, body)
	}
	if after := countProjects(t, db); after != before {
		t.Fatalf("rows after clone failure = %d, want unchanged %d (atomicity)", after, before)
	}
	dest := filepath.Join(base, "owner", "name")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest still exists after clone failure (stat err=%v); must be removed", err)
	}
}

// preCreateManagedDir creates dest (the managed clone location) as a real git
// repo with the given origin remote (empty origin = no remote added). It mirrors
// the on-disk shape reattach inspects.
func preCreateManagedDir(t *testing.T, dest, origin string) {
	t.Helper()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if out, err := exec.Command("git", "init", dest).CombinedOutput(); err != nil {
		t.Fatalf("git init dest: %v\n%s", err, out)
	}
	if origin != "" {
		if out, err := exec.Command("git", "-C", dest, "remote", "add", "origin", origin).CombinedOutput(); err != nil {
			t.Fatalf("git remote add origin: %v\n%s", err, out)
		}
	}
}

// TestReattachOriginMatch (CKOUT-05/D-10): dest already exists with an origin
// canonicalizing to the requested owner/name → reuse it (no re-clone) and INSERT
// managed=1. The clone seam fails the test if invoked.
func TestReattachOriginMatch(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "owner/name", true, nil
	})()

	dest := filepath.Join(base, "owner", "name")
	preCreateManagedDir(t, dest, "git@github.com:owner/name.git") // ssh origin, canonicalizes to owner/name

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status != http.StatusCreated {
		t.Fatalf("reattach-match: status = %d, want 201; body=%v", status, body)
	}
	if body["managed"] != true {
		t.Errorf("managed = %v, want true", body["managed"])
	}
	if body["repo_path"] != dest {
		t.Errorf("repo_path = %v, want reused dest %v", body["repo_path"], dest)
	}
	// Row persisted.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE repo_path = ? AND managed = 1`, dest).Scan(&n); err != nil {
		t.Fatalf("count managed row: %v", err)
	}
	if n != 1 {
		t.Errorf("managed rows at dest = %d, want 1", n)
	}
}

// TestReattachOriginMismatch (D-10): dest exists with a DIFFERENT origin → 409,
// directory NOT removed, no new row.
func TestReattachOriginMismatch(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "owner/name", true, nil
	})()

	dest := filepath.Join(base, "owner", "name")
	preCreateManagedDir(t, dest, "git@github.com:someone/else.git") // different repo

	before := countProjects(t, db)
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status != http.StatusConflict {
		t.Fatalf("reattach-mismatch: status = %d, want 409; body=%v", status, body)
	}
	if after := countProjects(t, db); after != before {
		t.Fatalf("rows after mismatch = %d, want unchanged %d", after, before)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("dest removed on mismatch (stat err=%v); must be untouched", err)
	}
}

// TestReattachNonGitDir (D-10): dest exists but is NOT a git repo → 409, dir
// untouched, no row. Never clobber a stray/user dir.
func TestReattachNonGitDir(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "owner/name", true, nil
	})()

	dest := filepath.Join(base, "owner", "name")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	// a stray file so the dir is non-empty and clearly user data
	if err := os.WriteFile(filepath.Join(dest, "keepme.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	before := countProjects(t, db)
	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status != http.StatusConflict {
		t.Fatalf("reattach-nongit: status = %d, want 409; body=%v", status, body)
	}
	if after := countProjects(t, db); after != before {
		t.Fatalf("rows after non-git dir = %d, want unchanged %d", after, before)
	}
	if _, err := os.Stat(filepath.Join(dest, "keepme.txt")); err != nil {
		t.Fatalf("stray file removed (stat err=%v); dir must be untouched", err)
	}
}

// TestReattachAlreadyAdded (D-10 dedup): dest exists with matching origin AND a
// project row already references it → 409 "this repository is already added".
func TestReattachAlreadyAdded(t *testing.T) {
	srv, db, base := newRepoTestServer(t)
	defer failCloneNeverCalled(t)()
	defer github.SetAvailableForTest(true)()
	defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
		return "owner/name", true, nil
	})()

	dest := filepath.Join(base, "owner", "name")
	preCreateManagedDir(t, dest, "https://github.com/owner/name.git")
	// Seed a row already referencing dest.
	if _, err := db.Exec(`INSERT INTO projects (name, repo_path, github_repo, managed) VALUES ('name', ?, 'owner/name', 1)`, dest); err != nil {
		t.Fatalf("seed managed row: %v", err)
	}

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo": "owner/name"})
	if status != http.StatusConflict {
		t.Fatalf("reattach-already-added: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "this repository is already added" {
		t.Errorf("error = %q, want %q", body["error"], "this repository is already added")
	}
}

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

// TestProjectManagedDefaultsZero proves the migration-00008 `managed` column
// flows create→scan→JSON: a folder-created project (the existing POST path,
// which omits `managed`) serializes "managed": false, i.e. the DEFAULT 0
// backfill (D-06/D-09 — folder dirs are never Kangent-owned).
func TestProjectManagedDefaultsZero(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepo(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/projects", map[string]any{"repo_path": repo})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%v", status, body)
	}
	managed, ok := body["managed"]
	if !ok {
		t.Fatalf("response missing \"managed\" key: %v", body)
	}
	if managed != false {
		t.Errorf("managed = %v, want false (folder default)", managed)
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

// TestUpdateProjectPartial exercises the grown PATCH: partial updates of name
// and description, the explicit-"" github_repo unlink, and the single
// host-independent hard-error case (a syntactically invalid ref, rejected by
// ParseRepoRef BEFORE gh is consulted). It stays gh-agnostic: existing links
// are seeded directly in the DB rather than via a now-mandatory-verified PATCH,
// so the test passes whether or not gh is installed/authenticated/online. The
// gh-verified link + canonicalization path is covered host-independently by
// package github's ParseRepoRef/ValidateRepo unit tests and exercised live on a
// gh-authenticated host; it is not asserted at the API layer here.
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

	// github_repo "" → unlink (NULL / JSON null). Unlink never invokes gh, so
	// seed the existing link directly in the DB (a gh-verified PATCH can't be
	// asserted host-independently now that linking is mandatory-verified).
	if _, err := db.Exec(`UPDATE projects SET github_repo = ? WHERE id = ?`, "cli/cli", id); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	status, pb = doJSON(t, "PATCH", url, map[string]any{"github_repo": ""})
	if status != http.StatusOK {
		t.Fatalf("unlink status = %d; body=%v", status, pb)
	}
	if v, ok := pb["github_repo"]; !ok || v != nil {
		t.Errorf("github_repo after unlink = %v (ok=%v), want null", v, ok)
	}
	assertDBProject(t, db, id, "", nil)

	// invalid ref → 400, canonical error, row unchanged. ParseRepoRef rejects
	// "not-a-repo" BEFORE gh is consulted, so the 400 holds on any host. Seed
	// the existing link directly in the DB (no gh-gated PATCH) so we can prove
	// the rejected PATCH does not clobber the stored value.
	if _, err := db.Exec(`UPDATE projects SET github_repo = ? WHERE id = ?`, "cli/cli", id); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	status, pb = doJSON(t, "PATCH", url, map[string]any{"github_repo": "not-a-repo"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid ref status = %d, want 400; body=%v", status, pb)
	}
	if pb["error"] != "Not a valid repository — use owner/name or a GitHub URL." {
		t.Errorf("error = %q, want canonical invalid-ref copy", pb["error"])
	}
	seeded := "cli/cli"
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

// TestUpdateProjectMandatoryValidation pins the MANDATORY hard-block contract
// (GHPRJ-03), superseding the soft verify_state advisory: a non-empty
// github_repo that gh cannot verify is rejected (400) and is NOT persisted,
// while description-only edits and the explicit-"" unlink never invoke gh and
// always succeed. Written host-independently — a bogus repo is never verified
// on any host (gh present → `gh repo view <bogus>` exits nonzero; gh absent →
// not verified) so the 400 holds either way. The exact error string is NOT
// asserted because it branches on github.Available(). The valid-repo → 200 path
// needs authenticated gh + network and is exercised live on this host, mirroring
// how 10-02 kept tests gh-agnostic.
func TestUpdateProjectMandatoryValidation(t *testing.T) {
	srv, db, _ := newTestServer(t)
	id := createProject(t, srv, gitRepo(t))
	url := fmt.Sprintf("%s/api/projects/%d", srv.URL, id)

	// Seed a link with a syntactically valid ref so we can prove a later
	// unverifiable PATCH does not clobber it. "owner/name" is syntactically
	// valid; on this gh host it also won't verify, so seed via the test that
	// already proves saving works — here we use the canonical owner/name which
	// the handler will attempt to verify. To keep the seed host-independent we
	// instead assert on whichever ref the seed leaves persisted.
	const seedRef = "owner/name"
	status, _ := doJSON(t, "PATCH", url, map[string]any{"github_repo": seedRef})
	// On a gh-authenticated host seedRef ("owner/name") may itself be
	// unverifiable, so this seed PATCH could 400. Only proceed with the
	// "bad PATCH does not clobber" assertion when the seed actually persisted.
	seedPersisted := status == http.StatusOK

	// A syntactically valid but unverifiable ref → 400, NOT persisted.
	const bogus = "octocat/this-repo-does-not-exist-kangent-test"
	status, body := doJSON(t, "PATCH", url, map[string]any{"github_repo": bogus})
	if status != http.StatusBadRequest {
		t.Fatalf("unverifiable repo status = %d, want 400; body=%v", status, body)
	}
	if seedPersisted {
		// The rejected PATCH must leave the previously-linked ref untouched.
		want := seedRef
		assertDBProject(t, db, id, "", &want)
	} else {
		// The seed itself was rejected, so the column is still NULL; the bogus
		// PATCH must not have created a link either.
		assertDBProject(t, db, id, "", nil)
	}

	// description-only PATCH → 200, saved, never invokes gh.
	status, body = doJSON(t, "PATCH", url, map[string]any{"description": "hello"})
	if status != http.StatusOK {
		t.Fatalf("description-only status = %d, want 200; body=%v", status, body)
	}
	if body["description"] != "hello" {
		t.Errorf("description = %v, want \"hello\"", body["description"])
	}

	// explicit unlink (github_repo "") → 200, github_repo NULL, no validation.
	status, body = doJSON(t, "PATCH", url, map[string]any{"github_repo": ""})
	if status != http.StatusOK {
		t.Fatalf("unlink status = %d, want 200; body=%v", status, body)
	}
	if v, ok := body["github_repo"]; !ok || v != nil {
		t.Errorf("github_repo after unlink = %v (ok=%v), want null", v, ok)
	}
	assertDBProject(t, db, id, "hello", nil)
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

// TestGithubOriginSuggestion exercises the dialog's on-open origin prefill
// endpoint (D-08): it canonicalizes the repo's `origin` remote to owner/name,
// and returns an empty suggestion (never an error) when there is no GitHub
// origin. A bogus project id 404s.
func TestGithubOriginSuggestion(t *testing.T) {
	srv, _, _ := newTestServer(t)

	// Repo with a GitHub ssh origin → canonicalized owner/name.
	repoWithOrigin := gitRepo(t)
	if out, err := exec.Command("git", "-C", repoWithOrigin, "remote", "add", "origin", "git@github.com:owner/name.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	id := createProject(t, srv, repoWithOrigin)
	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/projects/%d/github-origin", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("origin status = %d, want 200; body=%v", status, body)
	}
	if body["suggestion"] != "owner/name" {
		t.Errorf("suggestion = %v, want owner/name", body["suggestion"])
	}

	// Repo with an https origin → also canonicalized.
	repoHTTPS := gitRepo(t)
	if out, err := exec.Command("git", "-C", repoHTTPS, "remote", "add", "origin", "https://github.com/foo/bar.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add https: %v\n%s", err, out)
	}
	idHTTPS := createProject(t, srv, repoHTTPS)
	status, body = doJSON(t, "GET", fmt.Sprintf("%s/api/projects/%d/github-origin", srv.URL, idHTTPS), nil)
	if status != http.StatusOK {
		t.Fatalf("https origin status = %d, want 200; body=%v", status, body)
	}
	if body["suggestion"] != "foo/bar" {
		t.Errorf("https suggestion = %v, want foo/bar", body["suggestion"])
	}

	// Repo with NO origin remote → suggestion "" (NOT an error).
	repoNoOrigin := gitRepo(t)
	idNoOrigin := createProject(t, srv, repoNoOrigin)
	status, body = doJSON(t, "GET", fmt.Sprintf("%s/api/projects/%d/github-origin", srv.URL, idNoOrigin), nil)
	if status != http.StatusOK {
		t.Fatalf("no-origin status = %d, want 200; body=%v", status, body)
	}
	if body["suggestion"] != "" {
		t.Errorf("no-origin suggestion = %v, want \"\"", body["suggestion"])
	}

	// Non-GitHub origin → suggestion "" (ParseRepoRef rejects → empty, never error).
	repoOther := gitRepo(t)
	if out, err := exec.Command("git", "-C", repoOther, "remote", "add", "origin", "https://gitlab.com/foo/bar.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add gitlab: %v\n%s", err, out)
	}
	idOther := createProject(t, srv, repoOther)
	status, body = doJSON(t, "GET", fmt.Sprintf("%s/api/projects/%d/github-origin", srv.URL, idOther), nil)
	if status != http.StatusOK {
		t.Fatalf("non-github origin status = %d, want 200; body=%v", status, body)
	}
	if body["suggestion"] != "" {
		t.Errorf("non-github suggestion = %v, want \"\"", body["suggestion"])
	}

	// Bogus project id → 404.
	status, _ = doJSON(t, "GET", fmt.Sprintf("%s/api/projects/999999/github-origin", srv.URL), nil)
	if status != http.StatusNotFound {
		t.Fatalf("bogus id status = %d, want 404", status)
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

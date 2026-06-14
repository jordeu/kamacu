package api

// GET /api/tasks/{id}/diff over httptest, on Phase-3 fixture repos (outer-t git
// repo with isolated config, real SQLite in t.TempDir(), worktree provisioned
// through the real REST create flow). The structured JSON contract and the
// honest error relays (Pitfall 7) are asserted end to end.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/store"
	"kangent/internal/tmux"
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
	Routes(mux, db, wt, mgr, tmux.Client{})
	WorktreeRoutes(mux, db, wt, mgr, tmux.Client{})
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

// gitRepoForPRBase builds a three-commit, three-branch repo to discriminate the
// PR-base diff from the project-default diff (GHREV-03 / D-12):
//
//	main:         base.txt="m1"                                  (the project default)
//	feature-base: + basechange.txt="b1"  (off main)             (the PR's OWN base)
//	pr-head:      + prchange.txt="p1"     (off feature-base)    (the PR head)
//
// Diffed against feature-base (three-dot, merge-base = feature-base tip), the PR
// shows ONLY prchange.txt. Diffed against main (the WRONG, project-default base),
// it would ALSO show basechange.txt. So basechange.txt is the discriminator:
// present in a main-based diff, ABSENT in the correct feature-base diff.
// Returns the repo path and the pr-head commit OID.
func gitRepoForPRBase(t *testing.T) (repo, prHeadOID string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	run("init", "-b", "main")
	write("base.txt", "m1\n")
	run("add", "base.txt")
	run("commit", "-m", "main: base")
	// feature-base off main.
	run("checkout", "-b", "feature-base")
	write("basechange.txt", "b1\n")
	run("add", "basechange.txt")
	run("commit", "-m", "feature-base: basechange")
	// pr-head off feature-base.
	run("checkout", "-b", "pr-head")
	write("prchange.txt", "p1\n")
	run("add", "prchange.txt")
	run("commit", "-m", "pr-head: prchange")
	prHeadOID = run("rev-parse", "HEAD")
	// Leave the repo on main so a worktree can occupy pr-head/feature-base refs.
	run("checkout", "main")
	return dir, prHeadOID
}

// newDiffServerDB mirrors newDiffServer but also returns the *sql.DB so a test
// can hand-mint a github_pr review row directly (no open endpoint exists yet).
func newDiffServerDB(t *testing.T) (*httptest.Server, *sql.DB, *worktree.Service) {
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
	wt := worktree.NewService(wtDir)
	mgr := session.NewManager()
	mux := http.NewServeMux()
	Routes(mux, db, wt, mgr, tmux.Client{})
	WorktreeRoutes(mux, db, wt, mgr, tmux.Client{})
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
	return srv, db, wt
}

// gitC runs `git -C repo args...` with isolated config, failing the test on a
// nonzero exit. Used to provision a detached PR-head worktree by hand.
func gitC(t *testing.T, repo string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, repo, err, out)
	}
	return strings.TrimSpace(string(out))
}

// insertPRReviewRow mints a source='github_pr' task already pointing at a
// provisioned worktree (worktree_path set), with pr_base_ref — the shape the
// open endpoint (12-04) will produce, hand-built here so 12-03 is testable in
// isolation. Returns the task id.
func insertPRReviewRow(t *testing.T, db *sql.DB, projectID, prNumber int64, baseRef, worktreePath string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO tasks (project_id, title, description, status, position, source, pr_number, pr_base_ref, worktree_path)
		 VALUES (?, ?, '', 'todo', 1.0, 'github_pr', ?, ?, ?)`,
		projectID, fmt.Sprintf("PR #%d", prNumber), prNumber, baseRef, worktreePath)
	if err != nil {
		t.Fatalf("insert github_pr review row: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId for github_pr review row: %v", err)
	}
	return id
}

// TestDiffPRReviewUsesPRBase (GHREV-03 / D-12): a source='github_pr' task with
// pr_base_ref='feature-base' computes its diff against the PR's OWN base
// (feature-base), matching GitHub Files-changed — NOT the project default
// (main). The discriminator is basechange.txt: it belongs to the feature-base
// branch, so a correct PR-base diff EXCLUDES it (it is part of the base, not the
// PR), while the wrong project-default (main) diff would INCLUDE it.
func TestDiffPRReviewUsesPRBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, db, _ := newDiffServerDB(t)
	repo, prHeadOID := gitRepoForPRBase(t)
	pid := createProject(t, srv, repo)

	// Provision a DETACHED worktree on the PR head, OUTSIDE the project's
	// worktree root (the open endpoint owns placement; here we just need a tree).
	wtPath := filepath.Join(t.TempDir(), "pr-7")
	gitC(t, repo, "worktree", "add", "--detach", wtPath, prHeadOID)

	// Hand-mint the github_pr row pointing at that worktree (no open endpoint yet).
	prID := insertPRReviewRow(t, db, pid, 7, "feature-base", wtPath)

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, prID), nil)
	if status != http.StatusOK {
		t.Fatalf("GET pr diff status = %d, want 200; body=%v", status, body)
	}
	// Base must be the PR base, resolved to the local branch (feature-base
	// exists as refs/heads/feature-base) or origin/feature-base — never main/master.
	base, _ := body["base"].(string)
	if base != "feature-base" && base != "origin/feature-base" {
		t.Errorf("base = %q, want feature-base or origin/feature-base (the PR's OWN base)", base)
	}
	files, _ := body["files"].([]any)
	paths := map[string]bool{}
	for _, f := range files {
		fm, _ := f.(map[string]any)
		if p, ok := fm["path"].(string); ok {
			paths[p] = true
		}
	}
	if !paths["prchange.txt"] {
		t.Errorf("PR diff missing prchange.txt — the PR's own change must appear; files=%v", paths)
	}
	if paths["basechange.txt"] {
		t.Errorf("PR diff INCLUDES basechange.txt — that is part of the base (diffed against main, not feature-base); files=%v", paths)
	}
}

// TestDiffManualTaskUnchangedAlongsidePR (GHREV-03 negative): in the SAME repo,
// a manual task still resolves its base via ResolveBase (the project default,
// 'main'), proving the github_pr branch did not perturb the manual path.
func TestDiffManualTaskUnchangedAlongsidePR(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServer(t)
	repo, _ := gitRepoForPRBase(t)
	id, wtPath := provisionTaskWithWorktree(t, srv, repo)

	// Make one change in the manual worktree so the diff is non-trivial.
	if err := os.WriteFile(filepath.Join(wtPath, "manual.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatalf("write manual.txt: %v", err)
	}
	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET manual diff status = %d, want 200; body=%v", status, body)
	}
	// ResolveBase on this repo returns the project default branch 'main'.
	if base, _ := body["base"].(string); base != "main" {
		t.Errorf("manual task base = %q, want %q (ResolveBase, untouched by the PR branch)", base, "main")
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

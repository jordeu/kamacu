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

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
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

// TestDiffManagedTaskUsesOriginDefault (CKOUT-02 parity): a MANUAL task on a
// MANAGED project diffs against origin/main — fetched fresh at request time —
// so the tab shows exactly what a PR against the default branch will show.
// Origin advancing AFTER task creation must be chased by the diff-open fetch,
// and the stale local main must never win.
func TestDiffManagedTaskUsesOriginDefault(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	srv, db, _ := newDiffServerDB(t)
	origin, clone := makeOriginAndClone(t)
	pid := insertProjectRow(t, db, clone, true) // managed

	body := createTask(t, srv, pid, "Fresh Diff")
	id := taskID(t, body)
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("no worktree_path: %v", body)
	}
	// A task change, so the diff is non-empty.
	if err := os.WriteFile(filepath.Join(wtPath, "taskchange.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write taskchange.txt: %v", err)
	}

	// Origin advances AFTER task creation; the diff-open fetch must chase it.
	newTip := advanceOrigin(t, origin)

	status, dbody := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET managed diff status = %d, want 200; body=%v", status, dbody)
	}
	if base, _ := dbody["base"].(string); base != "origin/main" {
		t.Errorf("base = %q, want origin/main (managed: diff exactly what the PR will show)", base)
	}
	if got := cloneOriginMainSHA(t, clone); got != newTip {
		t.Errorf("clone origin/main = %s, want fetched tip %s (diff-open fetch did not run)", got, newTip)
	}
	// Sanity: the task's own change is in the diff (merge-base = the branch's
	// fork point, three-dot semantics — same as GitHub Files-changed).
	files, _ := dbody["files"].([]any)
	found := false
	for _, f := range files {
		fm, _ := f.(map[string]any)
		if p, _ := fm["path"].(string); p == "taskchange.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("taskchange.txt missing from managed diff; files=%v", dbody["files"])
	}
}

// TestDiffFolderTaskKeepsLocalBase: a FOLDER project's manual diff keeps the
// LOCAL default branch as its base even though origin advanced — the
// best-effort diff-open fetch refreshes origin/main for the merge-base, but
// folder projects never rebase their base resolution onto the remote ref
// (that is the managed path's behavior).
func TestDiffFolderTaskKeepsLocalBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	srv, db, _ := newDiffServerDB(t)
	origin, clone := makeOriginAndClone(t)
	pid := insertProjectRow(t, db, clone, false) // folder

	body := createTask(t, srv, pid, "Local Diff")
	id := taskID(t, body)
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("no worktree_path: %v", body)
	}

	newTip := advanceOrigin(t, origin)

	status, dbody := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET folder diff status = %d, want 200; body=%v", status, dbody)
	}
	if base, _ := dbody["base"].(string); base != "main" {
		t.Errorf("base = %q, want main (folder: local default branch stays the base)", base)
	}
	// The diff-open fetch is best-effort for folder projects too — it only
	// refreshes the remote-tracking ref for the merge-base.
	if got := cloneOriginMainSHA(t, clone); got != newTip {
		t.Errorf("clone origin/main = %s, want fetched tip %s (diff-open fetch did not run)", got, newTip)
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

// getDiffFile fetches GET /api/tasks/{id}/diff and returns the rendered hash and
// Viewed flag for the named path, plus whether that file appeared at all.
func getDiffFile(t *testing.T, srv *httptest.Server, id int64, path string) (hash string, viewed, found bool) {
	t.Helper()
	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d/diff", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("GET diff status = %d, want 200; body=%v", status, body)
	}
	files, _ := body["files"].([]any)
	for _, f := range files {
		fm, _ := f.(map[string]any)
		if p, _ := fm["path"].(string); p == path {
			h, _ := fm["hash"].(string)
			v, _ := fm["viewed"].(bool)
			return h, v, true
		}
	}
	return "", false, false
}

// putViewed issues the PUT .../diff/viewed toggle and returns the HTTP status.
func putViewed(t *testing.T, srv *httptest.Server, id int64, path, hash string, viewed bool) int {
	t.Helper()
	status, _ := doJSON(t, "PUT", fmt.Sprintf("%s/api/tasks/%d/diff/viewed", srv.URL, id),
		map[string]any{"path": path, "hash": hash, "viewed": viewed})
	return status
}

// TestViewedPersistToggleAndKeepHistory drives the read-merge + write endpoint
// end to end (DIFF-03/04): (a) PUT true then GET marks viewed; (b) PUT false
// clears it; (c) a changed rendered diff mints a new hash that reads un-viewed
// while the old-hash row is KEPT (D-02); (d) reverting to the byte-identical old
// diff restores the checkmark with NO new write.
func TestViewedPersistToggleAndKeepHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, db, _ := newDiffServerDB(t)
	repo := gitRepoWithCommit(t) // main with file.txt="hello\n" committed
	id, wtPath := provisionTaskWithWorktree(t, srv, repo)

	writeFile := func(content string) {
		if err := os.WriteFile(filepath.Join(wtPath, "file.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("write file.txt: %v", err)
		}
	}

	// Content C1 -> rendered-diff hash H1.
	writeFile("hello\nAAA\n")
	h1, viewed, found := getDiffFile(t, srv, id, "file.txt")
	if !found {
		t.Fatalf("file.txt missing from diff response")
	}
	if !isDiffHash(h1) {
		t.Fatalf("hash %q is not 64 lowercase hex", h1)
	}
	if viewed {
		t.Fatalf("file.txt viewed=true before any PUT; want false")
	}

	// (a) PUT true, GET marks viewed.
	if st := putViewed(t, srv, id, "file.txt", h1, true); st != http.StatusNoContent {
		t.Fatalf("(a) PUT viewed=true status = %d, want 204", st)
	}
	if _, viewed, _ = getDiffFile(t, srv, id, "file.txt"); !viewed {
		t.Fatalf("(a) file.txt viewed=false after PUT true; want true")
	}

	// (b) PUT false, GET clears it.
	if st := putViewed(t, srv, id, "file.txt", h1, false); st != http.StatusNoContent {
		t.Fatalf("(b) PUT viewed=false status = %d, want 204", st)
	}
	if _, viewed, _ = getDiffFile(t, srv, id, "file.txt"); viewed {
		t.Fatalf("(b) file.txt viewed=true after PUT false; want false")
	}

	// Re-mark viewed at H1 for the keep-history walk.
	if st := putViewed(t, srv, id, "file.txt", h1, true); st != http.StatusNoContent {
		t.Fatalf("re-mark PUT viewed=true status = %d, want 204", st)
	}

	// (c) Change the rendered diff -> new hash H2 reads un-viewed; the (path,H1)
	// row is deliberately KEPT (D-02, keep-history).
	writeFile("hello\nBBB\n")
	h2, viewed, found := getDiffFile(t, srv, id, "file.txt")
	if !found {
		t.Fatalf("file.txt missing from diff after change")
	}
	if h2 == h1 {
		t.Fatalf("(c) hash unchanged after content change: %q", h2)
	}
	if viewed {
		t.Fatalf("(c) file.txt viewed=true at new hash H2; want false (auto-reset)")
	}
	var keepCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM diff_viewed WHERE task_id=? AND file_path=? AND diff_hash=?`,
		id, "file.txt", h1).Scan(&keepCount); err != nil {
		t.Fatalf("query keep-history row: %v", err)
	}
	if keepCount != 1 {
		t.Fatalf("(c) old-hash (H1) row count = %d, want 1 (keep-history preserved)", keepCount)
	}

	// (d) Revert to the byte-identical C1 -> hash returns to H1 -> viewed again,
	// and no row was ever written at H2 (proving no implicit write on revert).
	writeFile("hello\nAAA\n")
	hBack, viewed, _ := getDiffFile(t, srv, id, "file.txt")
	if hBack != h1 {
		t.Fatalf("(d) reverted hash = %q, want H1 %q (byte-identical diff)", hBack, h1)
	}
	if !viewed {
		t.Fatalf("(d) file.txt viewed=false after revert to H1; want true (checkmark restored)")
	}
	var h2Count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM diff_viewed WHERE task_id=? AND file_path=? AND diff_hash=?`,
		id, "file.txt", h2).Scan(&h2Count); err != nil {
		t.Fatalf("query H2 row: %v", err)
	}
	if h2Count != 0 {
		t.Fatalf("(d) H2 row count = %d, want 0 (no write happened at H2)", h2Count)
	}
}

// TestViewedFKCascadeOnTaskDelete (e): deleting the owning task prunes every
// diff_viewed row via ON DELETE CASCADE. The DB is opened through store.Open
// (foreign_keys pragma ON), so the constraint actually fires; the assertion
// queries diff_viewed directly.
func TestViewedFKCascadeOnTaskDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, db, _ := newDiffServerDB(t)
	repo := gitRepoWithCommit(t)
	id, wtPath := provisionTaskWithWorktree(t, srv, repo)

	if err := os.WriteFile(filepath.Join(wtPath, "file.txt"), []byte("hello\nZ\n"), 0o644); err != nil {
		t.Fatalf("modify file.txt: %v", err)
	}
	h, _, found := getDiffFile(t, srv, id, "file.txt")
	if !found {
		t.Fatalf("file.txt missing from diff response")
	}
	if st := putViewed(t, srv, id, "file.txt", h, true); st != http.StatusNoContent {
		t.Fatalf("PUT viewed=true status = %d, want 204", st)
	}

	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM diff_viewed WHERE task_id = ?`, id).Scan(&before); err != nil {
		t.Fatalf("count rows before delete: %v", err)
	}
	if before == 0 {
		t.Fatalf("no diff_viewed rows persisted before delete")
	}

	// Delete the task row directly -> ON DELETE CASCADE prunes the rows.
	if _, err := db.Exec(`DELETE FROM tasks WHERE id = ?`, id); err != nil {
		t.Fatalf("delete task row: %v", err)
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM diff_viewed WHERE task_id = ?`, id).Scan(&after); err != nil {
		t.Fatalf("count rows after delete: %v", err)
	}
	if after != 0 {
		t.Fatalf("(e) diff_viewed rows after task delete = %d, want 0 (FK cascade)", after)
	}
}

// TestViewedRejectsBadInput (f): the write endpoint returns 400 for an empty
// path, a hash that is not exactly 64 lowercase hex chars, and an over-length
// path (T-22-03), never persisting garbage.
func TestViewedRejectsBadInput(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv, _, _ := newDiffServerDB(t)
	repo := gitRepoWithCommit(t)
	id, _ := provisionTaskWithWorktree(t, srv, repo)

	url := fmt.Sprintf("%s/api/tasks/%d/diff/viewed", srv.URL, id)
	goodHash := strings.Repeat("a", 64) // 64 lowercase hex

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty path", map[string]any{"path": "", "hash": goodHash, "viewed": true}},
		{"short hash", map[string]any{"path": "file.txt", "hash": "abc", "viewed": true}},
		{"uppercase hash", map[string]any{"path": "file.txt", "hash": strings.Repeat("A", 64), "viewed": true}},
		{"non-hex hash", map[string]any{"path": "file.txt", "hash": strings.Repeat("z", 64), "viewed": true}},
		{"over-length path", map[string]any{"path": strings.Repeat("a", 4097), "hash": goodHash, "viewed": true}},
	}
	for _, tc := range cases {
		status, body := doJSON(t, "PUT", url, tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("%s: PUT status = %d, want 400; body=%v", tc.name, status, body)
		}
	}
}

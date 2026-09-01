package api

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// createTask POSTs a task to a project and returns the decoded response body.
func createTask(t *testing.T, srv *httptest.Server, projectID int64, title string) map[string]any {
	t.Helper()
	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, projectID),
		map[string]any{"title": title})
	if status != http.StatusCreated {
		t.Fatalf("create task %q: status=%d body=%v", title, status, body)
	}
	return body
}

func taskID(t *testing.T, body map[string]any) int64 {
	t.Helper()
	id, ok := body["id"].(float64)
	if !ok {
		t.Fatalf("no numeric id in task body: %v", body)
	}
	return int64(id)
}

// gitRepoWithCommit creates a temp repo on branch main with one commit, so
// worktree provisioning has a valid base (gitRepo's unborn HEAD does not).
// Helper git commands run with isolated config + throwaway identity, matching
// internal/worktree's test hygiene.
func gitRepoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	run("add", "file.txt")
	run("commit", "-m", "initial")
	return dir
}

// branchList returns `git -C repo branch --list pattern` output.
func branchList(t *testing.T, repo, pattern string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "branch", "--list", pattern)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --list %s: %v\n%s", pattern, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitOut runs git in dir like gitIn (throwaway identity + host-config
// isolation) but returns trimmed combined output — for rev-parse reads the
// file:// fetch tests assert on. (gitIn, in worktrees_test.go, returns nothing.)
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// makeOriginAndClone builds a source repo on `main` with one commit and a
// file:// clone of it (origin/HEAD set, like a real gh repo clone). It returns
// the origin and clone dirs. The clone becomes a managed project's repo_path.
func makeOriginAndClone(t *testing.T) (origin, clone string) {
	t.Helper()
	origin = t.TempDir()
	gitIn(t, origin, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(origin, "file.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write origin file: %v", err)
	}
	gitIn(t, origin, "add", "file.txt")
	gitIn(t, origin, "commit", "-m", "c1")

	parent := t.TempDir()
	clone = filepath.Join(parent, "clone")
	gitIn(t, parent, "clone", "file://"+origin, clone)
	return origin, clone
}

// advanceOrigin pushes a new commit onto origin's main and returns the new tip
// SHA. The clone does NOT see it until something fetches.
func advanceOrigin(t *testing.T, origin string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(origin, "file.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write origin file v2: %v", err)
	}
	gitIn(t, origin, "add", "file.txt")
	gitIn(t, origin, "commit", "-m", "c2")
	return gitOut(t, origin, "rev-parse", "HEAD")
}

// cloneOriginMainSHA returns the clone's remote-tracking origin/main SHA — the
// ref the managed pre-task fetch advances and ResolveBase reads.
func cloneOriginMainSHA(t *testing.T, clone string) string {
	t.Helper()
	return gitOut(t, clone, "rev-parse", "origin/main")
}

// insertProjectRow inserts a project directly (bypassing the create endpoint —
// the repo-first managed create path is plan 02's surface; this keeps the
// pre-task-fetch tests independent of it) with the given managed marker and
// returns its id.
func insertProjectRow(t *testing.T, db *sql.DB, repoPath string, managed bool) int64 {
	t.Helper()
	m := 0
	if managed {
		m = 1
	}
	res, err := db.Exec(
		`INSERT INTO projects (name, repo_path, managed) VALUES (?, ?, ?)`,
		filepath.Base(repoPath), repoPath, m)
	if err != nil {
		t.Fatalf("insert project row (managed=%v): %v", managed, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId for project: %v", err)
	}
	return id
}

// TestManagedTaskWorktreeFetchesLatest (CKOUT-02/D-04): creating a task on a
// MANAGED project fetches the latest default branch before resolving the base,
// so the clone's origin/main advances to the origin tip pushed AFTER the clone.
func TestManagedTaskWorktreeFetchesLatest(t *testing.T) {
	// The API server's git fetch inherits the test process env; isolate host
	// git config so the file:// fetch is deterministic.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	srv, db, _ := newTestServer(t)
	origin, clone := makeOriginAndClone(t)
	pid := insertProjectRow(t, db, clone, true)

	// Push a NEW commit to origin AFTER the clone; the clone is now stale.
	newTip := advanceOrigin(t, origin)
	if cloneOriginMainSHA(t, clone) == newTip {
		t.Fatalf("clone already at origin tip before any fetch — fixture bug")
	}

	body := createTask(t, srv, pid, "Fresh Work")
	if body["worktree_error"] != nil {
		t.Fatalf("worktree_error = %v, want null", body["worktree_error"])
	}
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("worktree_path empty — provisioning failed: %v", body)
	}

	// The managed pre-task fetch advanced the clone's origin/main to the tip
	// pushed after the clone — proving the fetch ran before base resolution.
	if got := cloneOriginMainSHA(t, clone); got != newTip {
		t.Errorf("clone origin/main = %s, want fetched tip %s (managed pre-task fetch did not run)", got, newTip)
	}

	// The new branch must be BASED on the fetched tip: with no commits of its
	// own yet, its SHA IS its base. Pre-fix, it landed on the clone's stale
	// local main even though the fetch ran (ResolveBase's first leg preferred
	// refs/heads/main) — the exact outdated-master bug CKOUT-02 targets.
	branch, _ := body["branch"].(string)
	if branch == "" {
		t.Fatalf("no branch in create response: %v", body)
	}
	if sha := gitOut(t, clone, "rev-parse", branch); sha != newTip {
		t.Errorf("task branch %s = %s, want fetched tip %s — branched from a stale base", branch, sha, newTip)
	}
}

// TestFolderTaskWorktreeNoFetch (D-24): creating a task on a FOLDER project
// (managed=0) performs NO fetch — the clone's origin/main does NOT move even
// though origin advanced. Folder local-only behavior is preserved byte-for-byte.
func TestFolderTaskWorktreeNoFetch(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	srv, db, _ := newTestServer(t)
	origin, clone := makeOriginAndClone(t)
	pid := insertProjectRow(t, db, clone, false) // folder project

	before := cloneOriginMainSHA(t, clone)
	advanceOrigin(t, origin) // origin moves; a folder project must NOT chase it

	body := createTask(t, srv, pid, "Local Work")
	if body["worktree_error"] != nil {
		t.Fatalf("worktree_error = %v, want null", body["worktree_error"])
	}
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("worktree_path empty — provisioning failed: %v", body)
	}

	if after := cloneOriginMainSHA(t, clone); after != before {
		t.Errorf("folder project origin/main moved %s -> %s — a fetch happened (D-24 violated)", before, after)
	}
}

// TestManagedFetchBestEffortDoesNotBlock (D-05): a managed project whose origin
// is unreachable still provisions a worktree — the failed best-effort fetch is
// discarded and ResolveBase proceeds from the local base. Task creation never
// blocks on a failed fetch.
func TestManagedFetchBestEffortDoesNotBlock(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	srv, db, _ := newTestServer(t)
	origin, clone := makeOriginAndClone(t)
	pid := insertProjectRow(t, db, clone, true) // managed → fetch attempted

	// Make origin unreachable: remove it. The clone's `fetch origin main` now
	// fails, but provisioning must still succeed from the local base.
	if err := os.RemoveAll(origin); err != nil {
		t.Fatalf("remove origin: %v", err)
	}

	body := createTask(t, srv, pid, "Offline Work")
	// A failed fetch must NOT land in worktree_error (it is not a provisioning
	// gate, D-05) and must NOT block the worktree.
	if body["worktree_error"] != nil {
		t.Errorf("worktree_error = %v, want null — a failed best-effort fetch must not block (D-05)", body["worktree_error"])
	}
	if wtPath, _ := body["worktree_path"].(string); wtPath == "" {
		t.Fatalf("worktree_path empty — a failed fetch blocked provisioning (D-05 violated): %v", body)
	}
}

func TestTaskCreateProvisionsWorktree(t *testing.T) {
	srv, _, _ := newTestServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)

	body := createTask(t, srv, pid, "Fix Login")
	id := taskID(t, body)

	wantBranch := fmt.Sprintf("task/fix-login-%d", id)
	if body["branch"] != wantBranch {
		t.Errorf("branch = %v, want %q", body["branch"], wantBranch)
	}
	if body["worktree_error"] != nil {
		t.Errorf("worktree_error = %v, want null", body["worktree_error"])
	}
	wtPath, _ := body["worktree_path"].(string)
	if wtPath == "" {
		t.Fatalf("worktree_path = %v, want non-empty string", body["worktree_path"])
	}
	if !strings.HasSuffix(wtPath, fmt.Sprintf("/fix-login-%d", id)) {
		t.Errorf("worktree_path = %q, want suffix /fix-login-%d", wtPath, id)
	}
	if fi, err := os.Stat(wtPath); err != nil || !fi.IsDir() {
		t.Errorf("worktree dir missing on disk: %s (err %v)", wtPath, err)
	}
	if got := branchList(t, repo, wantBranch); got == "" {
		t.Errorf("git branch --list %s is empty — branch not created (GIT-01)", wantBranch)
	}
}

// TestTaskCreateBranchTemplateChange: a task created AFTER changing
// branch_template gets the template-driven branch (BRANCH-01) — settings are
// read at use inside provisionWorktree, no restart (SET-03).
func TestTaskCreateBranchTemplateChange(t *testing.T) {
	srv, db, _ := newTestServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)

	if err := settings.Set(db, settings.KeyBranchTemplate, "wip/{id}"); err != nil {
		t.Fatalf("set branch_template: %v", err)
	}

	body := createTask(t, srv, pid, "Templated Work")
	id := taskID(t, body)
	want := fmt.Sprintf("wip/%d", id)
	if body["branch"] != want {
		t.Errorf("branch = %v, want %q (current template must drive new branches)", body["branch"], want)
	}
	if body["worktree_error"] != nil {
		t.Errorf("worktree_error = %v, want null", body["worktree_error"])
	}
	if got := branchList(t, repo, want); got == "" {
		t.Errorf("git branch --list %s is empty — templated branch not created", want)
	}
}

// TestTaskCreateWorktreeBaseChange (WT-01/02): changing worktree_base affects
// only NEW worktrees; the previous task's stored absolute path is untouched
// and its tree still exists on disk.
func TestTaskCreateWorktreeBaseChange(t *testing.T) {
	srv, db, _ := newTestServer(t)
	repo := gitRepoWithCommit(t)
	pid := createProject(t, srv, repo)

	first := createTask(t, srv, pid, "Old Base")
	firstID := taskID(t, first)
	firstPath, _ := first["worktree_path"].(string)
	if firstPath == "" {
		t.Fatalf("first task provisioning failed: %v", first)
	}

	newBase := t.TempDir()
	if err := settings.Set(db, settings.KeyWorktreeBase, newBase); err != nil {
		t.Fatalf("set worktree_base: %v", err)
	}

	second := createTask(t, srv, pid, "New Base")
	secondPath, _ := second["worktree_path"].(string)
	if secondPath == "" {
		t.Fatalf("second task provisioning failed: %v", second)
	}
	wantPrefix := filepath.Join(newBase, filepath.Base(repo)) + string(os.PathSeparator)
	if !strings.HasPrefix(secondPath, wantPrefix) {
		t.Errorf("new worktree path = %q, want under %q (new base + repo-basename subdir)", secondPath, wantPrefix)
	}
	if fi, err := os.Stat(secondPath); err != nil || !fi.IsDir() {
		t.Errorf("new worktree missing on disk: %s (err %v)", secondPath, err)
	}

	// WT-02: the old tree is untouched on disk and its stored path unchanged.
	if fi, err := os.Stat(firstPath); err != nil || !fi.IsDir() {
		t.Errorf("old worktree disturbed by base change: %s (err %v)", firstPath, err)
	}
	status, got := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, firstID), nil)
	if status != http.StatusOK {
		t.Fatalf("GET first task: status = %d, want 200", status)
	}
	if got["worktree_path"] != firstPath {
		t.Errorf("stored worktree_path = %v, want unchanged %q", got["worktree_path"], firstPath)
	}
}

// TestTaskCreateCorruptTemplateLandsInWorktreeError: a hand-corrupted template
// (raw INSERT bypassing Set's validation) never crashes create — the
// create-time CheckRefFormat defense routes it into worktree_error (D-25) and
// the request still 201s.
func TestTaskCreateCorruptTemplateLandsInWorktreeError(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))

	if _, err := db.Exec(`INSERT INTO settings(key, value) VALUES('branch_template', 'bad..name-{id}')`); err != nil {
		t.Fatalf("raw settings insert: %v", err)
	}

	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid),
		map[string]any{"title": "Corrupt Template"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 — settings problems must never block idea capture; body=%v", status, body)
	}
	if body["branch"] != nil {
		t.Errorf("branch = %v, want null on failed provisioning", body["branch"])
	}
	if body["worktree_path"] != nil {
		t.Errorf("worktree_path = %v, want null on failed provisioning", body["worktree_path"])
	}
	werr, _ := body["worktree_error"].(string)
	if !strings.Contains(werr, "Not a valid git branch name.") {
		t.Errorf("worktree_error = %q, want it to contain %q", werr, "Not a valid git branch name.")
	}
}

func TestTaskCreateBrokenRepoStill201(t *testing.T) {
	srv, _, _ := newTestServer(t)
	// gitRepo = bare `git init`, no commit: unborn HEAD → provisioning fails.
	pid := createProject(t, srv, gitRepo(t))

	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid),
		map[string]any{"title": "No Base Yet"})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 — git problems must never block idea capture (D-25); body=%v", status, body)
	}
	if body["branch"] != nil {
		t.Errorf("branch = %v, want null on failed provisioning", body["branch"])
	}
	if body["worktree_path"] != nil {
		t.Errorf("worktree_path = %v, want null on failed provisioning", body["worktree_path"])
	}
	werr, _ := body["worktree_error"].(string)
	if werr == "" {
		t.Errorf("worktree_error = %v, want non-empty error string (D-25)", body["worktree_error"])
	}
}

func TestTaskJSONIncludesWorktreeFields(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))
	id := taskID(t, createTask(t, srv, pid, "Carry Fields"))

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("get status = %d, want 200; body=%v", status, body)
	}
	for _, key := range []string{"branch", "worktree_path", "worktree_error"} {
		if _, ok := body[key]; !ok {
			t.Errorf("GET /api/tasks/{id} missing %q", key)
		}
	}

	status, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if status != http.StatusOK {
		t.Fatalf("list status = %d, want 200", status)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	for _, key := range []string{"branch", "worktree_path", "worktree_error"} {
		if _, ok := list[0][key]; !ok {
			t.Errorf("GET /api/projects/{id}/tasks items missing %q", key)
		}
	}
}

// insertPRRow inserts a source='github_pr' task row directly via the DB
// (bypassing the create endpoint, which only makes manual tasks) and returns
// its id. This is how a PR review row is born until the open endpoint lands
// (Plan 12-04); this plan only needs the row to exist to prove the board-leak
// guards and the wire shape.
func insertPRRow(t *testing.T, db *sql.DB, projectID int64, prNumber int64, baseRef, status string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO tasks (project_id, title, description, status, position, source, pr_number, pr_base_ref)
		 VALUES (?, ?, '', ?, 1.0, 'github_pr', ?, ?)`,
		projectID, fmt.Sprintf("PR #%d", prNumber), status, prNumber, baseRef)
	if err != nil {
		t.Fatalf("insert github_pr row: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId for github_pr row: %v", err)
	}
	return id
}

// TestTaskJSONIncludesSourceFields (GHREV-04, 12-02 Task 1): the Task wire
// shape carries source/pr_number/pr_base_ref. A manual task reports
// source="manual" with null pr fields; a directly-inserted github_pr row
// round-trips its source/pr_number/pr_base_ref through GET /api/tasks/{id}.
func TestTaskJSONIncludesSourceFields(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))

	// Manual task: source="manual", pr fields null.
	manualID := taskID(t, createTask(t, srv, pid, "Manual Task"))
	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, manualID), nil)
	if status != http.StatusOK {
		t.Fatalf("GET manual task: status = %d, want 200; body=%v", status, body)
	}
	if body["source"] != "manual" {
		t.Errorf("manual task source = %v, want %q", body["source"], "manual")
	}
	if v, ok := body["pr_number"]; !ok || v != nil {
		t.Errorf("manual task pr_number = %v (present=%v), want null", v, ok)
	}
	if v, ok := body["pr_base_ref"]; !ok || v != nil {
		t.Errorf("manual task pr_base_ref = %v (present=%v), want null", v, ok)
	}

	// github_pr row: round-trips source/pr_number/pr_base_ref.
	prID := insertPRRow(t, db, pid, 42, "main", "todo")
	status, body = doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, prID), nil)
	if status != http.StatusOK {
		t.Fatalf("GET pr task: status = %d, want 200; body=%v", status, body)
	}
	if body["source"] != "github_pr" {
		t.Errorf("pr task source = %v, want %q", body["source"], "github_pr")
	}
	if got, _ := body["pr_number"].(float64); int64(got) != 42 {
		t.Errorf("pr task pr_number = %v, want 42", body["pr_number"])
	}
	if body["pr_base_ref"] != "main" {
		t.Errorf("pr task pr_base_ref = %v, want %q", body["pr_base_ref"], "main")
	}
}

func TestTaskCreateDefaults(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	body := createTask(t, srv, pid, "a")
	if body["status"] != "todo" {
		t.Errorf("status = %q, want %q", body["status"], "todo")
	}
	if body["description"] != "" {
		t.Errorf("description = %q, want empty", body["description"])
	}
	if body["title"] != "a" {
		t.Errorf("title = %q, want %q", body["title"], "a")
	}
	if _, ok := body["position"].(float64); !ok {
		t.Errorf("position missing or not a number: %v", body["position"])
	}
}

func TestTaskCreateLandsAtTop(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	createTask(t, srv, pid, "first")
	createTask(t, srv, pid, "second")
	last := createTask(t, srv, pid, "third")

	status, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if status != http.StatusOK {
		t.Fatalf("list status = %d, want 200", status)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	lowest := list[0]
	if lowest["id"] != last["id"] {
		t.Errorf("first listed task id = %v, want last-created %v (new tasks at top)", lowest["id"], last["id"])
	}
	for i := 1; i < len(list); i++ {
		if list[i-1]["position"].(float64) >= list[i]["position"].(float64) {
			t.Errorf("positions not strictly ascending at index %d: %v", i, list)
		}
	}
}

func TestTaskCreateValidation(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))

	status, body := doJSON(t, "POST", fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid),
		map[string]any{"title": "  "})
	if status != http.StatusBadRequest {
		t.Fatalf("whitespace title: status = %d, want 400; body=%v", status, body)
	}
	if body["error"] != "title is required" {
		t.Errorf("error = %q, want %q", body["error"], "title is required")
	}

	status, body = doJSON(t, "POST", srv.URL+"/api/projects/999/tasks", map[string]any{"title": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("nonexistent parent: status = %d, want 404; body=%v", status, body)
	}
}

func TestTaskGet(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "deep link me"))

	status, body := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["title"] != "deep link me" {
		t.Errorf("title = %q, want %q", body["title"], "deep link me")
	}
	if body["project_id"] != float64(pid) {
		t.Errorf("project_id = %v, want %v", body["project_id"], pid)
	}

	status, _ = doJSON(t, "GET", srv.URL+"/api/tasks/424242", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown id: status = %d, want 404", status)
	}
}

func TestTaskUpdate(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "keep title"))

	status, body := doJSON(t, "PATCH", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id),
		map[string]any{"description": "# md"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	if body["description"] != "# md" {
		t.Errorf("description = %q, want %q", body["description"], "# md")
	}
	if body["title"] != "keep title" {
		t.Errorf("title = %q, want unchanged %q", body["title"], "keep title")
	}

	status, body = doJSON(t, "PATCH", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id),
		map[string]any{"title": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty title: status = %d, want 400; body=%v", status, body)
	}
}

func TestTaskDelete(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	id := taskID(t, createTask(t, srv, pid, "doomed"))

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	status, _ := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusNotFound {
		t.Fatalf("after hard delete: GET status = %d, want 404", status)
	}
}

// TestTaskDeleteStopsSessions closes research gap #1: DELETE /api/tasks/{id}
// must stop ALL of the task's running sessions BEFORE deleting the row —
// otherwise a deleted task's agent keeps running headless, holding the
// worktree busy.
func TestTaskDeleteStopsSessions(t *testing.T) {
	srv, mgr, _ := newAgentServer(t)
	id, _ := worktreeTask(t, srv, "Doomed With Agent")
	spawnAgentFor(t, srv, id)

	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	// StopAllForTask blocks through the grace BEFORE the DB delete, so by the
	// time 204 arrives no session of the task may still be running.
	for _, info := range mgr.ListByTask(id) {
		if info.Status == session.StatusRunning {
			t.Errorf("session %s (%s) still running after task delete — leaked headless", info.ID, info.Label)
		}
	}

	// The 404 path is unchanged: deleting the already-deleted task.
	status, body := doJSON(t, "DELETE", fmt.Sprintf("%s/api/tasks/%d", srv.URL, id), nil)
	if status != http.StatusNotFound {
		t.Fatalf("repeat delete: status = %d, want 404; body=%v", status, body)
	}
}

// moveTask POSTs to /api/tasks/{id}/move and returns status + body.
func moveTask(t *testing.T, srv *httptest.Server, id int64, status string, afterID *int64) (int, map[string]any) {
	t.Helper()
	return doJSON(t, "POST", fmt.Sprintf("%s/api/tasks/%d/move", srv.URL, id),
		map[string]any{"status": status, "after_id": afterID})
}

// columnTasks returns the tasks of one column, in listed (position) order.
func columnTasks(t *testing.T, srv *httptest.Server, pid int64, status string) []map[string]any {
	t.Helper()
	code, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", code)
	}
	var col []map[string]any
	for _, task := range list {
		if task["status"] == status {
			col = append(col, task)
		}
	}
	return col
}

func TestMoveToTop(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	// existing occupants of in_review
	o1 := taskID(t, createTask(t, srv, pid, "occupant1"))
	o2 := taskID(t, createTask(t, srv, pid, "occupant2"))
	moveTask(t, srv, o1, "in_review", nil)
	moveTask(t, srv, o2, "in_review", nil)

	x := taskID(t, createTask(t, srv, pid, "x"))
	code, body := moveTask(t, srv, x, "in_review", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if body["status"] != "in_review" {
		t.Errorf("status = %q, want in_review", body["status"])
	}
	xPos := body["position"].(float64)
	for _, task := range columnTasks(t, srv, pid, "in_review") {
		if task["id"] != body["id"] && task["position"].(float64) <= xPos {
			t.Errorf("task %v position %v <= moved task %v (must be top)", task["id"], task["position"], xPos)
		}
	}
}

func TestMoveBetween(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	// creates land at top, so creation order c, b, a gives column order a, b, c
	c := taskID(t, createTask(t, srv, pid, "c"))
	b := taskID(t, createTask(t, srv, pid, "b"))
	taskID(t, createTask(t, srv, pid, "a"))
	_ = c

	x := taskID(t, createTask(t, srv, pid, "x")) // now at very top
	// move x after b: expected order a(?), wait — x currently above a; after move: a? Let's just assert between b and its follower.
	code, body := moveTask(t, srv, x, "todo", &b)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	col := columnTasks(t, srv, pid, "todo")
	// find b and x; x must be directly after b and strictly between b and the next one
	var bPos, xPos float64
	var xIdx = -1
	for i, task := range col {
		switch int64(task["id"].(float64)) {
		case b:
			bPos = task["position"].(float64)
		case x:
			xPos = task["position"].(float64)
			xIdx = i
		}
	}
	if xIdx < 1 || int64(col[xIdx-1]["id"].(float64)) != b {
		t.Fatalf("x is not directly after b in column: %v", col)
	}
	if xPos <= bPos {
		t.Errorf("x.position %v not > b.position %v", xPos, bPos)
	}
	if xIdx+1 < len(col) {
		next := col[xIdx+1]["position"].(float64)
		if xPos >= next {
			t.Errorf("x.position %v not < next %v", xPos, next)
		}
	}
}

func TestMoveToBottom(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	taskID(t, createTask(t, srv, pid, "later-top"))
	bottom := taskID(t, createTask(t, srv, pid, "will-be-bottom-anchor"))
	// bottom of todo is the FIRST created task; recompute: last in column order
	col := columnTasks(t, srv, pid, "todo")
	last := int64(col[len(col)-1]["id"].(float64))
	_ = bottom

	x := taskID(t, createTask(t, srv, pid, "x"))
	code, body := moveTask(t, srv, x, "todo", &last)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	xPos := body["position"].(float64)
	for _, task := range columnTasks(t, srv, pid, "todo") {
		if task["id"] != body["id"] && task["position"].(float64) >= xPos {
			t.Errorf("task %v position %v >= moved task %v (must be bottom)", task["id"], task["position"], xPos)
		}
	}
}

func TestMoveValidation(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	a := taskID(t, createTask(t, srv, pid, "a"))
	b := taskID(t, createTask(t, srv, pid, "b"))
	// put b in a different column
	moveTask(t, srv, b, "in_review", nil)

	// after_id in a different column than the target status
	code, body := moveTask(t, srv, a, "todo", &b)
	if code != http.StatusBadRequest {
		t.Fatalf("cross-column after_id: status = %d, want 400; body=%v", code, body)
	}

	// after_id in a different project
	pid2 := createProject(t, srv, gitRepo(t))
	d := taskID(t, createTask(t, srv, pid2, "d"))
	code, body = moveTask(t, srv, a, "todo", &d)
	if code != http.StatusBadRequest {
		t.Fatalf("cross-project after_id: status = %d, want 400; body=%v", code, body)
	}

	// bogus status
	code, body = moveTask(t, srv, a, "bogus", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("bogus status: status = %d, want 400; body=%v", code, body)
	}
	if body["error"] != "invalid status" {
		t.Errorf("error = %q, want %q", body["error"], "invalid status")
	}
}

// TestPRReviewNeverLeaksToBoard (GHREV-04, 12-02 Task 2): the milestone's
// single highest-severity regression guard. A source='github_pr' row must NEVER
// appear on the board, must NEVER receive a board position via /move (409), and
// must NEVER perturb a manual task's positioning math.
func TestPRReviewNeverLeaksToBoard(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))

	// A manual task and a PR-review row co-exist in the same project + status.
	manualID := taskID(t, createTask(t, srv, pid, "Manual Card"))
	prID := insertPRRow(t, db, pid, 7, "main", "todo")

	// 1. The PR row is ABSENT from the board; the manual task IS present.
	code, list := doJSONList(t, fmt.Sprintf("%s/api/projects/%d/tasks", srv.URL, pid))
	if code != http.StatusOK {
		t.Fatalf("board list status = %d, want 200", code)
	}
	var sawManual, sawPR bool
	for _, task := range list {
		switch int64(task["id"].(float64)) {
		case manualID:
			sawManual = true
		case prID:
			sawPR = true
		}
	}
	if !sawManual {
		t.Errorf("manual task %d missing from board list", manualID)
	}
	if sawPR {
		t.Errorf("PR review %d leaked onto the board (GHREV-04)", prID)
	}

	// 2. POST /api/tasks/{prId}/move → 409 (a PR review can never get a board
	// position, even via a hand-crafted request).
	code, body := moveTask(t, srv, prID, "in_progress", nil)
	if code != http.StatusConflict {
		t.Fatalf("move PR review: status = %d, want 409; body=%v", code, body)
	}
	if body["error"] != "PR reviews are not board tasks" {
		t.Errorf("move PR review error = %q, want %q", body["error"], "PR reviews are not board tasks")
	}
	// The PR row's status must be unchanged (the move was rejected before any write).
	status, prBody := doJSON(t, "GET", fmt.Sprintf("%s/api/tasks/%d", srv.URL, prID), nil)
	if status != http.StatusOK {
		t.Fatalf("GET PR review after rejected move: status = %d", status)
	}
	if prBody["status"] != "todo" {
		t.Errorf("PR review status = %q after rejected move, want unchanged %q", prBody["status"], "todo")
	}

	// 3. Positioning purity: a manual task moved to the top of To Do lands at a
	// position strictly below every OTHER manual task, with the PR row's
	// position (1.0) NOT perturbing the MIN-based top-of-column math.
	other := taskID(t, createTask(t, srv, pid, "Other Manual"))
	_ = other
	code, body = moveTask(t, srv, manualID, "todo", nil)
	if code != http.StatusOK {
		t.Fatalf("move manual to top: status = %d, want 200; body=%v", code, body)
	}
	movedPos := body["position"].(float64)
	for _, task := range columnTasks(t, srv, pid, "todo") {
		if int64(task["id"].(float64)) == manualID {
			continue
		}
		if task["position"].(float64) <= movedPos {
			t.Errorf("manual task %v position %v <= moved task position %v — must be strict top of column",
				task["id"], task["position"], movedPos)
		}
	}
}

// TestPRReviewExcludedFromCreatePositioning (GHREV-04): a manual task created in
// a project whose ONLY existing To Do row is a PR review still lands at a sane
// top-of-column position (the PR row's position must not seed the MIN). With no
// manual To Do rows, COALESCE(MIN,2.0)-1.0 = 1.0; a PR row at any position must
// not change that.
func TestPRReviewExcludedFromCreatePositioning(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))

	// Seed a PR row at a low position to try to poison the create MIN.
	if _, err := db.Exec(
		`INSERT INTO tasks (project_id, title, description, status, position, source, pr_number, pr_base_ref)
		 VALUES (?, 'PR #99', '', 'todo', -5.0, 'github_pr', 99, 'main')`, pid); err != nil {
		t.Fatalf("seed low-position PR row: %v", err)
	}

	body := createTask(t, srv, pid, "First Manual")
	pos := body["position"].(float64)
	// Baseline (no PR row) yields 1.0; the PR row's -5.0 must be excluded, so the
	// manual task must NOT inherit a position derived from -5.0 (which would be
	// -6.0). Assert it matches the clean top-of-empty-column value.
	if pos != 1.0 {
		t.Errorf("first manual task position = %v, want 1.0 (PR row must not seed create MIN)", pos)
	}
}

func TestMoveStressRenormalize(t *testing.T) {
	srv, _, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	// two anchors; creation order means anchorB sits ABOVE anchorA in todo
	taskID(t, createTask(t, srv, pid, "anchorA"))
	anchorB := taskID(t, createTask(t, srv, pid, "anchorB"))

	// 200 tasks each squeezed between anchorB and whatever currently follows it
	for i := 0; i < 200; i++ {
		id := taskID(t, createTask(t, srv, pid, fmt.Sprintf("squeeze-%d", i)))
		code, body := moveTask(t, srv, id, "todo", &anchorB)
		if code != http.StatusOK {
			t.Fatalf("squeeze %d: status = %d, body=%v", i, code, body)
		}
	}

	col := columnTasks(t, srv, pid, "todo")
	if len(col) != 202 {
		t.Fatalf("column size = %d, want 202", len(col))
	}
	seen := map[int64]bool{}
	for i, task := range col {
		id := int64(task["id"].(float64))
		if seen[id] {
			t.Fatalf("duplicate task id %d in column", id)
		}
		seen[id] = true
		if i > 0 {
			prev := col[i-1]["position"].(float64)
			cur := task["position"].(float64)
			if prev >= cur {
				t.Fatalf("positions not strictly increasing at index %d: %v >= %v", i, prev, cur)
			}
		}
	}
}

func TestMovePersistsAcrossReopen(t *testing.T) {
	srv, db, dbPath := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	a := taskID(t, createTask(t, srv, pid, "a"))
	b := taskID(t, createTask(t, srv, pid, "b"))
	c := taskID(t, createTask(t, srv, pid, "c"))
	moveTask(t, srv, a, "in_progress", nil)
	moveTask(t, srv, c, "in_progress", &a)
	moveTask(t, srv, b, "done", nil)

	var before []int64
	for _, task := range columnTasks(t, srv, pid, "in_progress") {
		before = append(before, int64(task["id"].(float64)))
	}

	srv.Close()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	rows, err := db2.Query(`SELECT id FROM tasks WHERE project_id = ? AND status = 'in_progress' ORDER BY position ASC`, pid)
	if err != nil {
		t.Fatalf("query reopened db: %v", err)
	}
	defer rows.Close()
	var after []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		after = append(after, id)
	}
	if len(before) != len(after) {
		t.Fatalf("in_progress sizes differ: before=%v after=%v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("order diverged after reopen: before=%v after=%v", before, after)
		}
	}
	// also verify b really persisted to done
	var st string
	if err := db2.QueryRow(`SELECT status FROM tasks WHERE id = ?`, b).Scan(&st); err != nil || st != "done" {
		t.Fatalf("task b status after reopen = %q (err %v), want done", st, err)
	}
}

// statusAt reads the *_at column for a task directly from the DB. Returns the
// raw stored ISO string and whether it was non-NULL.
func statusAt(t *testing.T, db *sql.DB, id int64, col string) (string, bool) {
	t.Helper()
	var v sql.NullString
	// col is a fixed test literal, never user input.
	if err := db.QueryRow(`SELECT `+col+` FROM tasks WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatalf("read %s for task %d: %v", col, id, err)
	}
	return v.String, v.Valid
}

// TestMoveStampsDoneAt (D-90): entering Done stamps done_at with a non-NULL ISO
// timestamp — the reaper's clock (REAP-01).
func TestMoveStampsDoneAt(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	x := taskID(t, createTask(t, srv, pid, "x"))

	if _, ok := statusAt(t, db, x, "done_at"); ok {
		t.Fatalf("done_at non-NULL before any move to done")
	}
	if code, body := moveTask(t, srv, x, "done", nil); code != http.StatusOK {
		t.Fatalf("move to done: status=%d body=%v", code, body)
	}
	got, ok := statusAt(t, db, x, "done_at")
	if !ok || got == "" {
		t.Fatalf("done_at = %q ok=%v, want non-NULL ISO timestamp after move to done", got, ok)
	}
}

// TestMoveStampsEnteredStatusOnly (D-90): moving to in_progress stamps
// in_progress_at and leaves the (here unset) done_at untouched.
func TestMoveStampsEnteredStatusOnly(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	x := taskID(t, createTask(t, srv, pid, "x"))

	if code, _ := moveTask(t, srv, x, "in_progress", nil); code != http.StatusOK {
		t.Fatalf("move to in_progress: status=%d", code)
	}
	if got, ok := statusAt(t, db, x, "in_progress_at"); !ok || got == "" {
		t.Fatalf("in_progress_at = %q ok=%v, want stamped", got, ok)
	}
	if _, ok := statusAt(t, db, x, "done_at"); ok {
		t.Fatalf("done_at stamped by a move to in_progress — only the entered status's column must change")
	}
}

// TestMoveDoneAtLastEntryWins (D-90): re-entering Done (done -> in_review ->
// done) OVERWRITES done_at with the later time; leaving Done does NOT clear it.
func TestMoveDoneAtLastEntryWins(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepo(t))
	x := taskID(t, createTask(t, srv, pid, "x"))

	moveTask(t, srv, x, "done", nil)
	first, ok := statusAt(t, db, x, "done_at")
	if !ok || first == "" {
		t.Fatalf("done_at not stamped on first entry to done")
	}

	// Leave Done: done_at must survive (the reaper's status gate, not done_at,
	// is what cancels reaping).
	moveTask(t, srv, x, "in_review", nil)
	afterLeave, ok := statusAt(t, db, x, "done_at")
	if !ok || afterLeave != first {
		t.Fatalf("leaving Done changed done_at: was %q now %q (ok=%v) — leaving must not clear it", first, afterLeave, ok)
	}

	// Re-enter Done after a beat: last-entry-wins overwrites with a later time.
	// strftime('%f') gives millisecond resolution; sleep past it to guarantee a
	// strictly greater lexical timestamp.
	time.Sleep(5 * time.Millisecond)
	moveTask(t, srv, x, "done", nil)
	second, ok := statusAt(t, db, x, "done_at")
	if !ok || second == "" {
		t.Fatalf("done_at missing after re-entry to done")
	}
	if second <= first {
		t.Fatalf("re-entering Done did not overwrite done_at (last-entry-wins): first=%q second=%q", first, second)
	}
}

// TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition (D-01 /
// MCPTASK-01): the unscoped GET /api/tasks endpoint returns ONLY
// source='manual' tasks (PR-review rows stay off the board — GHREV-04) and
// orders them by status, position ASC. The status sort is a plain string
// comparison, so done < in_progress < in_review < todo alphabetically. Seeds
// one manual + one github_pr row in the same project and asserts only the
// manual row appears.
func TestTaskList_All_ReturnsOnlyManualTasksOrderedByStatusPosition(t *testing.T) {
	srv, db, _ := newTestServer(t)
	pid := createProject(t, srv, gitRepoWithCommit(t))

	// Insert a "done" manual task first by creating it (defaults to todo) then
	// moving it to done. Then create a second manual task (stays todo). The
	// unscoped list sorts by status ASC (string compare), so done < todo. Then
	// insert a github_pr row that must NEVER appear in the unscoped list.
	doneID := taskID(t, createTask(t, srv, pid, "Done First"))
	moveTask(t, srv, doneID, "done", nil)
	todoID := taskID(t, createTask(t, srv, pid, "Todo Second"))
	_ = todoID
	insertPRRow(t, db, pid, 99, "main", "todo")

	status, list := doJSONList(t, srv.URL+"/api/tasks")
	if status != http.StatusOK {
		t.Fatalf("GET /api/tasks: status=%d, want 200", status)
	}
	if len(list) != 2 {
		t.Fatalf("GET /api/tasks returned %d rows, want 2 (manual only — PR row excluded by GHREV-04)", len(list))
	}
	// Ordering: status ASC (string compare) → "done" sorts before "todo".
	if list[0]["status"] != "done" {
		t.Errorf("row 0 status = %v, want done (status ASC string-compare ordering)", list[0]["status"])
	}
	if list[1]["status"] != "todo" {
		t.Errorf("row 1 status = %v, want todo (status ASC string-compare ordering)", list[1]["status"])
	}
	// No row should be a PR-review source.
	for i, row := range list {
		if row["source"] != "manual" {
			t.Errorf("row %d source = %v, want manual (PR rows must not leak)", i, row["source"])
		}
	}
}

// TestTaskList_EmptyReturnsEmptyArray (D-01): on an empty DB the unscoped
// GET /api/tasks returns [] (not null) — matches listByProject's
// tasks := []Task{} initialization.
func TestTaskList_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _, _ := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/tasks")
	if err != nil {
		t.Fatalf("GET /api/tasks: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	// Empty array, NOT null.
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Errorf("GET /api/tasks empty body = %q, want %q", strings.TrimSpace(string(raw)), "[]")
	}
}

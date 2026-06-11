package api

import (
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

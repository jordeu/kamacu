package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kamacu/internal/session"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// stubPRState is a spy PRStateGetter for the panel tests: it returns a mapped
// state per (repo, number) and records every call so a test can assert the
// panel called gh only for github_pr rows and never for a manual/orphan row.
// A missing key returns an error, exercising the degrade-don't-break path.
type stubPRState struct {
	states map[int]string // pr_number -> "OPEN"|"CLOSED"|"MERGED"
	calls  int
	err    error // when set, every call returns this error (gh absent)
}

func (s *stubPRState) PRState(ctx context.Context, repo string, n int) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	if st, ok := s.states[n]; ok {
		return st, nil
	}
	return "", fmt.Errorf("no state for PR #%d", n)
}

// newCleanupPanelServer wires the worktree-cleanup panel routes over a migrated
// temp DB, a worktree.Service, an empty session.Manager, a zero-value tmux
// client (no live tmux), and the supplied PRStateGetter spy.
func newCleanupPanelServer(t *testing.T, pr PRStateGetter) (*httptest.Server, *testWorktreeEnv) {
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
	WorktreeCleanupRoutes(mux, db, wt, mgr, tmux.Client{}, pr)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		db.Close()
	})
	return srv, &testWorktreeEnv{db: db, wt: wt, mgr: mgr}
}

// seedProjectFull inserts a project with icon fields and returns its id.
func seedProjectFull(t *testing.T, db *sql.DB, name, repo string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO projects (name, repo_path, icon_letters, icon_color) VALUES (?, ?, ?, ?)`,
		name, repo, "PR", "#123456")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedTaskFull inserts a task with status/source/pr fields and a worktree path.
func seedTaskFull(t *testing.T, db *sql.DB, projectID int64, title, status, branch, wtPath, source string, prNumber int, prBase string) int64 {
	t.Helper()
	var pn any
	if prNumber > 0 {
		pn = prNumber
	}
	var pb any
	if prBase != "" {
		pb = prBase
	}
	res, err := db.Exec(
		`INSERT INTO tasks (project_id, title, status, position, branch, worktree_path, source, pr_number, pr_base_ref)
		 VALUES (?, ?, ?, 1.0, ?, ?, ?, ?, ?)`,
		projectID, title, status, branch, wtPath, source, pn, pb)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// addLinkedWorktree adds a linked worktree on a new branch to a repo, returning
// its path. Uses the package's isolated-config helper hygiene.
func addLinkedWorktree(t *testing.T, repo, branch string) string {
	t.Helper()
	wtPath := filepath.Join(t.TempDir(), branch)
	full := append([]string{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@test"},
		"worktree", "add", wtPath, "-b", branch)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add %s: %v\n%s", branch, err, out)
	}
	return wtPath
}

// projectGroupsFromList parses the GET /api/worktrees response into typed
// helpers a test can assert on without re-deriving the JSON shape each time.
type listResp struct {
	Projects []struct {
		ProjectID   int64  `json:"project_id"`
		ProjectName string `json:"project_name"`
		IconLetters string `json:"icon_letters"`
		IconColor   string `json:"icon_color"`
		Worktrees   []struct {
			Repo           string  `json:"repo"`
			Path           string  `json:"path"`
			TaskID         int64   `json:"task_id"`
			Classification string  `json:"classification"`
			Association    *string `json:"association"`
			PRState        *string `json:"pr_state"`
			Branch         string  `json:"branch"`
			Dirty          int     `json:"dirty"`
			Unpushed       *int    `json:"unpushed"`
			Stash          int     `json:"stash"`
			Blocked        bool    `json:"blocked"`
			BlockedPath    *string `json:"blocked_path"`
			Sessions       int     `json:"sessions"`
		} `json:"worktrees"`
	} `json:"projects"`
	Counts struct {
		Total    int `json:"total"`
		Orphaned int `json:"orphaned"`
	} `json:"counts"`
}

func getList(t *testing.T, srv *httptest.Server) listResp {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/worktrees")
	if err != nil {
		t.Fatalf("GET /api/worktrees: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/worktrees status = %d, want 200", resp.StatusCode)
	}
	var out listResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return out
}

// TestWorktreeCleanupListClassifies is the core union+classify test: a repo with
// one referenced task worktree + one orphan (git-listed, no task) yields one
// project group with 2 rows (referenced task_id>0, orphan task_id==0); the main
// worktree is absent; counts reflect the non-main rows and orphan subset.
func TestWorktreeCleanupListClassifies(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Alpha", repo)

	// Referenced worktree: a linked worktree with a matching task row. status
	// "done" makes it a cleanup candidate so it is shown (WTREE-01 refinement:
	// the panel lists only candidates; an in-progress referenced row is hidden).
	refPath := addLinkedWorktree(t, repo, "feature")
	seedTaskFull(t, env.db, pid, "Do the thing", "done", "feature", refPath, "manual", 0, "")

	// Orphan worktree: a linked worktree with NO task row.
	_ = addLinkedWorktree(t, repo, "orphan-branch")

	resp := getList(t, srv)
	if len(resp.Projects) != 1 {
		t.Fatalf("projects = %d, want 1; %+v", len(resp.Projects), resp)
	}
	g := resp.Projects[0]
	if g.ProjectName != "Alpha" || g.IconLetters != "PR" {
		t.Errorf("group meta = %q/%q, want Alpha/PR", g.ProjectName, g.IconLetters)
	}
	if len(g.Worktrees) != 2 {
		t.Fatalf("worktrees = %d, want 2 (referenced + orphan, main excluded); %+v", len(g.Worktrees), g.Worktrees)
	}
	var sawRef, sawOrphan bool
	for _, wt := range g.Worktrees {
		if filepath.Clean(wt.Path) == filepath.Clean(repo) {
			t.Errorf("main worktree leaked into rows: %s", wt.Path)
		}
		switch wt.Classification {
		case "referenced":
			sawRef = true
			if wt.TaskID == 0 {
				t.Errorf("referenced row has task_id 0")
			}
			if wt.Association == nil || *wt.Association != "Task: Do the thing" {
				t.Errorf("referenced association = %v, want %q", wt.Association, "Task: Do the thing")
			}
		case "orphan":
			sawOrphan = true
			if wt.TaskID != 0 {
				t.Errorf("orphan row has task_id %d, want 0", wt.TaskID)
			}
			if wt.Association != nil {
				t.Errorf("orphan association = %v, want null", *wt.Association)
			}
		default:
			t.Errorf("unexpected classification %q for %s", wt.Classification, wt.Path)
		}
	}
	if !sawRef || !sawOrphan {
		t.Errorf("classification coverage: referenced=%v orphan=%v", sawRef, sawOrphan)
	}
	if resp.Counts.Total != 2 {
		t.Errorf("counts.total = %d, want 2", resp.Counts.Total)
	}
	if resp.Counts.Orphaned != 1 {
		t.Errorf("counts.orphaned = %d, want 1", resp.Counts.Orphaned)
	}
}

// TestWorktreeCleanupListStalePointer: a task row whose worktree_path is not in
// its repo's git list is a STALE row (task_id>0), with no dirty/unpushed
// computed (dir gone).
func TestWorktreeCleanupListStalePointer(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Beta", repo)

	// A task pointing at a path that was never a git-registered worktree.
	gonePath := filepath.Join(t.TempDir(), "gone")
	staleID := seedTaskFull(t, env.db, pid, "Vanished", "done", "stale-branch", gonePath, "manual", 0, "")

	resp := getList(t, srv)
	if len(resp.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(resp.Projects))
	}
	rows := resp.Projects[0].Worktrees
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 stale row; %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Classification != "stale" {
		t.Errorf("classification = %q, want stale", r.Classification)
	}
	if r.TaskID != staleID {
		t.Errorf("stale task_id = %d, want %d", r.TaskID, staleID)
	}
	if r.Dirty != 0 || r.Unpushed != nil {
		t.Errorf("stale row computed flags: dirty=%d unpushed=%v, want 0/null (dir gone)", r.Dirty, r.Unpushed)
	}
	if resp.Counts.Total != 1 || resp.Counts.Orphaned != 0 {
		t.Errorf("counts = %d total / %d orphaned, want 1 / 0", resp.Counts.Total, resp.Counts.Orphaned)
	}
}

// TestWorktreeCleanupListDirtyFlag: a referenced DONE task with 2 uncommitted
// files reports dirty=2; the base resolves so unpushed is a number (0), not null.
// status "done" keeps the row a cleanup candidate (shown) even though it is
// dirty — a done-but-dirty referenced row stays shown as a manual force-remove
// candidate (WTREE-01 refinement).
func TestWorktreeCleanupListDirtyFlag(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Gamma", repo)
	wtPath := addLinkedWorktree(t, repo, "dirty-branch")
	seedTaskFull(t, env.db, pid, "Messy", "done", "dirty-branch", wtPath, "manual", 0, "")

	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(wtPath, name), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	resp := getList(t, srv)
	rows := resp.Projects[0].Worktrees
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Dirty != 2 {
		t.Errorf("dirty = %d, want 2", rows[0].Dirty)
	}
	if rows[0].Unpushed == nil {
		t.Errorf("unpushed = null, want a number (base resolvable in a real repo)")
	}
}

// TestWorktreeCleanupListUnpushedNullDegrade: an orphan worktree checked out on
// a DETACHED HEAD has no branch base to diff against, so its row lists with
// unpushed:null while the GET stays 200 (a resolution failure degrades one row's
// flag, never the whole GET). This exercises the null branch of unpushedBase.
func TestWorktreeCleanupListUnpushedNullDegrade(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	seedProjectFull(t, env.db, "Delta", repo)

	// A detached orphan worktree: `git worktree add --detach <path>` checks out
	// HEAD with no branch. It has no task row (orphan) and no branch (detached),
	// so unpushedBase returns ("", false) → unpushed:null.
	wtPath := filepath.Join(t.TempDir(), "detached-orphan")
	full := append([]string{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@test"},
		"worktree", "add", "--detach", wtPath)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add --detach: %v\n%s", err, out)
	}

	resp := getList(t, srv)
	rows := resp.Projects[0].Worktrees
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(rows), rows)
	}
	if rows[0].Classification != "orphan" {
		t.Errorf("classification = %q, want orphan", rows[0].Classification)
	}
	if rows[0].Unpushed != nil {
		t.Errorf("unpushed = %d, want null (detached orphan has no base)", *rows[0].Unpushed)
	}
	// The GET returned 200 with the row intact — the degrade path did not 500.
}

// TestWorktreeCleanupListPRStateLowercased: a github_pr row whose PRState returns
// "MERGED" emits pr_state "merged" (lowercase, matching the frontend type), and
// PRState errors leave pr_state null with no 500.
func TestWorktreeCleanupListPRStateLowercased(t *testing.T) {
	pr := &stubPRState{states: map[int]string{42: "MERGED"}}
	srv, env := newCleanupPanelServer(t, pr)
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Epsilon", repo)
	wtPath := addLinkedWorktree(t, repo, "pr-branch")
	seedTaskFull(t, env.db, pid, "Review", "in_review", "pr-branch", wtPath, "github_pr", 42, "main")

	resp := getList(t, srv)
	rows := resp.Projects[0].Worktrees
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].PRState == nil || *rows[0].PRState != "merged" {
		t.Errorf("pr_state = %v, want %q (lowercased from MERGED)", rows[0].PRState, "merged")
	}
	if pr.calls == 0 {
		t.Errorf("PRState was never called for a github_pr row")
	}
}

// TestWorktreeCleanupListPRStateAbsentDegrades: with a gh-absent spy, a github_pr
// row's PRState errors, so under the WTREE-01 refinement the row is HIDDEN
// (gh-unconfirmed ⇒ treated as still-active ⇒ hidden, degrade-don't-break). Its
// project has no other rows, so the group is omitted, counts.total is 0, and the
// GET stays 200 (a gh failure hides one row, never a broken chip / 500).
func TestWorktreeCleanupListPRStateAbsentDegrades(t *testing.T) {
	pr := &stubPRState{err: fmt.Errorf("gh not installed")}
	srv, env := newCleanupPanelServer(t, pr)
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Zeta", repo)
	wtPath := addLinkedWorktree(t, repo, "pr-branch")
	seedTaskFull(t, env.db, pid, "Review", "in_review", "pr-branch", wtPath, "github_pr", 99, "main")

	resp := getList(t, srv)
	if len(resp.Projects) != 0 {
		t.Fatalf("projects = %d, want 0 (gh-unconfirmed PR row hidden, empty group omitted); %+v", len(resp.Projects), resp.Projects)
	}
	if resp.Counts.Total != 0 {
		t.Errorf("counts.total = %d, want 0 (row hidden)", resp.Counts.Total)
	}
	// The GET returned 200 (getList asserts status 200) — degrade did not 500.
}

// TestWorktreeCleanupListShowsOnlyCandidates asserts the WTREE-01 refinement
// (user decision reversing "list every worktree" → "list only cleanup
// candidates"): GET /api/worktrees returns ONLY cleanup-candidate worktrees and
// hides active-work rows.
//
// SHOWN (candidates): orphan, stale pointer, referenced-done task, referenced
// github_pr whose PRState is MERGED or CLOSED.
// HIDDEN (active work): referenced manual task not done (in_progress),
// referenced github_pr whose PRState is OPEN, and a github_pr whose PRState
// lookup errors (gh-unconfirmed ⇒ treated as still-active ⇒ hidden).
// Also: a project whose worktrees are all hidden is omitted from projects, and
// counts.total equals the number of shown rows.
func TestWorktreeCleanupListShowsOnlyCandidates(t *testing.T) {
	pr := &stubPRState{states: map[int]string{
		10: "MERGED", // shown
		11: "CLOSED", // shown
		20: "OPEN",   // hidden (active)
		// 21 has no mapping → PRState errors → hidden (gh-unconfirmed)
	}}
	srv, env := newCleanupPanelServer(t, pr)

	// Alpha: a mix of shown + hidden rows.
	repoA := gitRepoWithCommit(t)
	pidA := seedProjectFull(t, env.db, "Alpha", repoA)

	// SHOWN: orphan (git-listed, no task).
	orphanPath := addLinkedWorktree(t, repoA, "orphan-wt")

	// SHOWN: stale pointer (task path not in git's list).
	stalePath := filepath.Join(t.TempDir(), "gone")
	seedTaskFull(t, env.db, pidA, "Vanished", "in_progress", "stale-branch", stalePath, "manual", 0, "")

	// SHOWN: referenced manual task, status done.
	donePath := addLinkedWorktree(t, repoA, "done-wt")
	seedTaskFull(t, env.db, pidA, "Finished", "done", "done-wt", donePath, "manual", 0, "")

	// SHOWN: referenced github_pr, PRState MERGED.
	mergedPath := addLinkedWorktree(t, repoA, "merged-wt")
	seedTaskFull(t, env.db, pidA, "Merged PR", "in_review", "merged-wt", mergedPath, "github_pr", 10, "main")

	// SHOWN: referenced github_pr, PRState CLOSED.
	closedPath := addLinkedWorktree(t, repoA, "closed-wt")
	seedTaskFull(t, env.db, pidA, "Closed PR", "in_review", "closed-wt", closedPath, "github_pr", 11, "main")

	// HIDDEN: referenced manual task, status in_progress (active work).
	wipPath := addLinkedWorktree(t, repoA, "wip-wt")
	seedTaskFull(t, env.db, pidA, "In progress", "in_progress", "wip-wt", wipPath, "manual", 0, "")

	// HIDDEN: referenced github_pr, PRState OPEN (active work).
	openPath := addLinkedWorktree(t, repoA, "open-wt")
	seedTaskFull(t, env.db, pidA, "Open PR", "in_review", "open-wt", openPath, "github_pr", 20, "main")

	// HIDDEN: referenced github_pr whose PRState lookup errors (gh-unconfirmed).
	errPath := addLinkedWorktree(t, repoA, "err-wt")
	seedTaskFull(t, env.db, pidA, "Unknown PR", "in_review", "err-wt", errPath, "github_pr", 21, "main")

	// Beta: only active-work rows → project group must be omitted entirely.
	repoB := gitRepoWithCommit(t)
	pidB := seedProjectFull(t, env.db, "Beta", repoB)
	betaWIP := addLinkedWorktree(t, repoB, "beta-wip")
	seedTaskFull(t, env.db, pidB, "Beta WIP", "in_progress", "beta-wip", betaWIP, "manual", 0, "")

	resp := getList(t, srv)

	// Beta has zero shown rows → omitted; only Alpha remains.
	if len(resp.Projects) != 1 {
		t.Fatalf("projects = %d, want 1 (Beta omitted — all rows hidden); %+v", len(resp.Projects), resp.Projects)
	}
	g := resp.Projects[0]
	if g.ProjectName != "Alpha" {
		t.Fatalf("shown project = %q, want Alpha (Beta must be omitted)", g.ProjectName)
	}

	shown := map[string]string{} // clean path -> classification
	for _, wt := range g.Worktrees {
		shown[filepath.Clean(wt.Path)] = wt.Classification
	}

	wantShown := map[string]string{
		filepath.Clean(orphanPath): "orphan",
		filepath.Clean(stalePath):  "stale",
		filepath.Clean(donePath):   "referenced",
		filepath.Clean(mergedPath): "referenced",
		filepath.Clean(closedPath): "referenced",
	}
	for p, cls := range wantShown {
		got, ok := shown[p]
		if !ok {
			t.Errorf("candidate row missing from list: %s (want classification %q)", p, cls)
			continue
		}
		if got != cls {
			t.Errorf("row %s classification = %q, want %q", p, got, cls)
		}
	}

	for _, p := range []string{wipPath, openPath, errPath, betaWIP} {
		if _, in := shown[filepath.Clean(p)]; in {
			t.Errorf("active-work row wrongly shown: %s", p)
		}
	}

	if len(g.Worktrees) != len(wantShown) {
		t.Errorf("shown rows = %d, want %d (only candidates); rows=%v", len(g.Worktrees), len(wantShown), shown)
	}

	// counts.total counts only shown rows; counts.orphaned only shown orphans.
	if resp.Counts.Total != len(wantShown) {
		t.Errorf("counts.total = %d, want %d (shown rows only)", resp.Counts.Total, len(wantShown))
	}
	if resp.Counts.Orphaned != 1 {
		t.Errorf("counts.orphaned = %d, want 1 (only the shown orphan)", resp.Counts.Orphaned)
	}
}

// --- Task 2: remove / clean-eligible / clear-pointer ---

// postJSON POSTs a JSON body and returns (status, decoded map). A non-JSON body
// (204/empty) yields an empty map.
func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	return doJSON(t, "POST", url, body)
}

// TestWorktreeCleanupRemove: a clean referenced worktree removed via the shared
// gated path returns 204 and the task's worktree columns are nulled.
func TestWorktreeCleanupRemove(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Alpha", repo)
	wtPath := addLinkedWorktree(t, repo, "feature")
	id := seedTaskFull(t, env.db, pid, "Do it", "done", "feature", wtPath, "manual", 0, "")

	status, body := postJSON(t, srv.URL+"/api/worktrees/remove", map[string]any{
		"repo": repo, "path": wtPath, "task_id": id, "force": true, "stop_sessions": true,
	})
	if status != http.StatusNoContent {
		t.Fatalf("remove status = %d, want 204; body=%v", status, body)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still present after remove: %v", err)
	}
	br, wp := taskColsFor(t, env.db, id)
	if br != "" || wp != "" {
		t.Errorf("task columns not nulled after remove: branch=%q worktree_path=%q", br, wp)
	}
}

// TestWorktreeCleanupBlocked: a permission-blocked shell (a mode-000 subdir git
// can't delete) returns HTTP 200 with {outcome:"blocked", path:...}, NOT a 500,
// and does NOT --force (the tree survives).
func TestWorktreeCleanupBlocked(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode-000 does not block removal")
	}
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Beta", repo)
	wtPath := addLinkedWorktree(t, repo, "blocked-branch")
	id := seedTaskFull(t, env.db, pid, "Blocked", "done", "blocked-branch", wtPath, "manual", 0, "")

	blocked := filepath.Join(wtPath, ".db")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir blocked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "x"), []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	status, body := postJSON(t, srv.URL+"/api/worktrees/remove", map[string]any{
		"repo": repo, "path": wtPath, "task_id": id, "force": true, "stop_sessions": true,
	})
	if status != http.StatusOK {
		t.Fatalf("blocked remove status = %d, want 200 (not 500); body=%v", status, body)
	}
	if body["outcome"] != "blocked" {
		t.Errorf("outcome = %v, want %q", body["outcome"], "blocked")
	}
	if body["path"] == nil || body["path"] == "" {
		t.Errorf("blocked response missing offending path: %v", body)
	}
	// Pitfall 1: no --force retry — the tree survives as a blocked shell.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Errorf("worktree dir vanished — a --force retry was attempted on the blocked case")
	}
}

// TestWorktreeCleanupCleanEligibleDryRun: preview with 1 orphan (clean) + 1
// done-clean-referenced + 1 dirty-done-referenced returns exactly the orphan
// (reason "orphaned") and the clean done row (reason "done"); the dirty one is
// excluded (a tripped gate makes it ineligible; bulk never forces).
func TestWorktreeCleanupCleanEligibleDryRun(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Gamma", repo)

	orphanPath := addLinkedWorktree(t, repo, "orphan-wt")

	doneCleanPath := addLinkedWorktree(t, repo, "done-clean")
	seedTaskFull(t, env.db, pid, "Done clean", "done", "done-clean", doneCleanPath, "manual", 0, "")

	dirtyDonePath := addLinkedWorktree(t, repo, "done-dirty")
	seedTaskFull(t, env.db, pid, "Done dirty", "done", "done-dirty", dirtyDonePath, "manual", 0, "")
	if err := os.WriteFile(filepath.Join(dirtyDonePath, "wip.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dirty: %v", err)
	}

	status, body := postJSON(t, srv.URL+"/api/worktrees/clean-eligible?dry_run=1", nil)
	if status != http.StatusOK {
		t.Fatalf("dry-run status = %d, want 200; body=%v", status, body)
	}
	itemsRaw, _ := body["items"].([]any)
	byPath := map[string]string{}
	for _, it := range itemsRaw {
		m, _ := it.(map[string]any)
		byPath[filepath.Clean(m["path"].(string))] = m["reason"].(string)
	}
	if len(byPath) != 2 {
		t.Fatalf("preview items = %d, want 2 (orphan + done-clean); got %v", len(byPath), byPath)
	}
	if r := byPath[filepath.Clean(orphanPath)]; r != "orphaned" {
		t.Errorf("orphan reason = %q, want %q", r, "orphaned")
	}
	if r := byPath[filepath.Clean(doneCleanPath)]; r != "done" {
		t.Errorf("done-clean reason = %q, want %q", r, "done")
	}
	if _, in := byPath[filepath.Clean(dirtyDonePath)]; in {
		t.Errorf("dirty-done worktree wrongly eligible: %v", byPath)
	}
}

// TestWorktreeCleanupCleanEligibleExecute: the applied bulk removes the eligible
// set best-effort and reports {removed, skipped}; it removes the orphan + clean
// done rows and leaves the dirty one on disk (never forced).
func TestWorktreeCleanupCleanEligibleExecute(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Delta", repo)

	orphanPath := addLinkedWorktree(t, repo, "orphan-wt")
	doneCleanPath := addLinkedWorktree(t, repo, "done-clean")
	seedTaskFull(t, env.db, pid, "Done clean", "done", "done-clean", doneCleanPath, "manual", 0, "")
	dirtyDonePath := addLinkedWorktree(t, repo, "done-dirty")
	seedTaskFull(t, env.db, pid, "Done dirty", "done", "done-dirty", dirtyDonePath, "manual", 0, "")
	if err := os.WriteFile(filepath.Join(dirtyDonePath, "wip.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dirty: %v", err)
	}

	status, body := postJSON(t, srv.URL+"/api/worktrees/clean-eligible", nil)
	if status != http.StatusOK {
		t.Fatalf("execute status = %d, want 200; body=%v", status, body)
	}
	if body["removed"] != float64(2) {
		t.Errorf("removed = %v, want 2", body["removed"])
	}
	for _, p := range []string{orphanPath, doneCleanPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("eligible worktree not removed: %s", p)
		}
	}
	// The dirty-done worktree was never forced → still on disk.
	if _, err := os.Stat(dirtyDonePath); err != nil {
		t.Errorf("dirty-done worktree removed (forced!) — bulk must never force: %v", err)
	}
}

// TestWorktreeCleanupClearPointer: clearing a stale pointer nulls the task's
// worktree columns, keeps the row, and returns 204.
func TestWorktreeCleanupClearPointer(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Epsilon", repo)
	gonePath := filepath.Join(t.TempDir(), "gone")
	id := seedTaskFull(t, env.db, pid, "Vanished", "done", "kept-branch", gonePath, "manual", 0, "")

	status, body := postJSON(t, srv.URL+"/api/worktrees/clear-pointer", map[string]any{"task_id": id})
	if status != http.StatusNoContent {
		t.Fatalf("clear-pointer status = %d, want 204; body=%v", status, body)
	}
	// The row still exists with worktree_path NULL (D-07 keeps the row + branch).
	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count task: %v", err)
	}
	if count != 1 {
		t.Fatalf("task row deleted by clear-pointer, want kept")
	}
	br, wp := taskColsFor(t, env.db, id)
	if wp != "" {
		t.Errorf("worktree_path = %q, want NULL after clear-pointer", wp)
	}
	if br != "" {
		t.Errorf("branch = %q, want NULL after clear-pointer (columns nulled, row kept)", br)
	}
}

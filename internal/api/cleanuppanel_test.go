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

	// Referenced worktree: a linked worktree with a matching task row.
	refPath := addLinkedWorktree(t, repo, "feature")
	seedTaskFull(t, env.db, pid, "Do the thing", "in_progress", "feature", refPath, "manual", 0, "")

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

// TestWorktreeCleanupListDirtyFlag: a referenced manual task with 2 uncommitted
// files reports dirty=2; the base resolves so unpushed is a number (0), not null.
func TestWorktreeCleanupListDirtyFlag(t *testing.T) {
	srv, env := newCleanupPanelServer(t, &stubPRState{})
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Gamma", repo)
	wtPath := addLinkedWorktree(t, repo, "dirty-branch")
	seedTaskFull(t, env.db, pid, "Messy", "in_progress", "dirty-branch", wtPath, "manual", 0, "")

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
// row lists with pr_state null and the GET is 200 (never a broken chip / 500).
func TestWorktreeCleanupListPRStateAbsentDegrades(t *testing.T) {
	pr := &stubPRState{err: fmt.Errorf("gh not installed")}
	srv, env := newCleanupPanelServer(t, pr)
	repo := gitRepoWithCommit(t)
	pid := seedProjectFull(t, env.db, "Zeta", repo)
	wtPath := addLinkedWorktree(t, repo, "pr-branch")
	seedTaskFull(t, env.db, pid, "Review", "in_review", "pr-branch", wtPath, "github_pr", 99, "main")

	resp := getList(t, srv)
	rows := resp.Projects[0].Worktrees
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].PRState != nil {
		t.Errorf("pr_state = %v, want null (gh absent degrade)", *rows[0].PRState)
	}
}

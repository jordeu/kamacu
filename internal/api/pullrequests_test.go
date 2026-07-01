package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/github"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/worktree"
)

// prFake is the call-counting fake runner injected into the Service so no real
// gh ever spawns. It records the repo/repoDir of the most recent call (to prove
// the handler's SELECT threads github_repo + repo_path through, Pitfall 6) and
// returns one PR with state "ok".
type prFake struct {
	calls    atomic.Int64
	lastRepo string
	lastDir  string
}

func (f *prFake) run(ctx context.Context, repo, repoDir string) (github.PRLists, string, error) {
	f.calls.Add(1)
	f.lastRepo, f.lastDir = repo, repoDir
	return github.PRLists{
		Awaiting: []github.PRSummary{{Number: 7, Title: "fake", Author: "octocat", Checks: "pass"}},
	}, "ok", nil
}

// newPRTestDB opens + migrates an isolated temp DB and returns the handle.
func newPRTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newPREnv wires a fresh mux with PullRequestRoutes backed by a Service whose
// runner is the supplied fake. now may be nil for the real clock; tests that
// need to advance past the attempt floor inject a controllable clock. A real
// worktree.Service (rooted at a temp dir) is wired so the POST review route can
// provision; tests that only exercise the GET list / the POST gates never reach
// it.
func newPREnv(t *testing.T, db *sql.DB, fake *prFake, now func() time.Time) *http.ServeMux {
	t.Helper()
	svc := github.New(github.Config{Runner: fake.run, Now: now})
	wtSvc := worktree.NewService(t.TempDir())
	mux := http.NewServeMux()
	PullRequestRoutes(mux, db, svc, wtSvc)
	return mux
}

// prPost drives a POST through the mux and returns the recorder.
func prPost(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

// gitInit runs git in dir with a throwaway identity, failing on nonzero exit.
func gitInit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// makePRRepo builds a real local repo carrying a refs/pull/<n>/head ref (the
// fork-PR shape: only the pull ref resolves the head) and returns the repo path
// + the head OID. This lets the review handler's CheckoutPR run against real git
// with no gh and no network — the gh half is stubbed via the viewPR seam.
func makePRRepo(t *testing.T, prNumber int) (repoPath, headOID string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repoPath = t.TempDir()
	gitInit(t, repoPath, "init", "-q", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	gitInit(t, repoPath, "add", "file.txt")
	gitInit(t, repoPath, "commit", "-q", "-m", "base commit")
	gitInit(t, repoPath, "checkout", "-q", "-b", "pr-source")
	if err := os.WriteFile(filepath.Join(repoPath, "pr.txt"), []byte("pr change\n"), 0o644); err != nil {
		t.Fatalf("write pr.txt: %v", err)
	}
	gitInit(t, repoPath, "add", "pr.txt")
	gitInit(t, repoPath, "commit", "-q", "-m", "pr head commit")
	headOID = strings.TrimSpace(gitInit(t, repoPath, "rev-parse", "HEAD"))
	gitInit(t, repoPath, "update-ref", "refs/pull/"+itoa(int64(prNumber))+"/head", headOID)
	gitInit(t, repoPath, "checkout", "-q", "trunk")
	gitInit(t, repoPath, "branch", "-D", "pr-source")
	// origin must resolve for CheckoutPR's `git fetch origin refs/pull/<n>/head`;
	// point it back at this same repo (self-remote — the pull ref lives here).
	gitInit(t, repoPath, "remote", "add", "origin", repoPath)
	gitInit(t, repoPath, "fetch", "-q", "origin")
	return repoPath, headOID
}

// stubViewPR swaps the handler's viewPR seam to return the given detail and
// restores it after the test. n-keying lets a test return per-PR details.
func stubViewPR(t *testing.T, fn func(ctx context.Context, repo string, n int) (github.PRDetail, error)) {
	t.Helper()
	prev := viewPR
	viewPR = fn
	t.Cleanup(func() { viewPR = prev })
}

// newPRReviewEnv wires a mux whose review route provisions through a real
// worktree.Service rooted at wtRoot, with worktree_base set to wtRoot so
// PathUnder lands inside it.
func newPRReviewEnv(t *testing.T, db *sql.DB, wtRoot string) *http.ServeMux {
	t.Helper()
	if err := settings.Set(db, settings.KeyWorktreeBase, wtRoot); err != nil {
		t.Fatalf("set worktree_base: %v", err)
	}
	svc := github.New(github.Config{})
	wtSvc := worktree.NewService(wtRoot)
	mux := http.NewServeMux()
	PullRequestRoutes(mux, db, svc, wtSvc)
	return mux
}

// insertProject inserts a project row. repo == "" leaves github_repo NULL
// (unlinked). repoPath must be unique per row (the column is UNIQUE).
func insertProject(t *testing.T, db *sql.DB, name, repoPath, repo string) int64 {
	t.Helper()
	var ghRepo any
	if repo != "" {
		ghRepo = repo
	}
	res, err := db.Exec(
		`INSERT INTO projects(name, repo_path, github_repo) VALUES(?, ?, ?)`,
		name, repoPath, ghRepo,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// prGet drives a GET through the mux and returns the recorder.
func prGet(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// decodePRResult unmarshals a body into github.Result, failing on error.
func decodePRResult(t *testing.T, rec *httptest.ResponseRecorder) github.Result {
	t.Helper()
	var res github.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return res
}

// TestPullRequestsDisabledWhenToggleOff: github_integration off -> 200 +
// state=disabled, and the runner is NEVER invoked (GHSET-02 backend gate).
func TestPullRequestsDisabledWhenToggleOff(t *testing.T) {
	db := newPRTestDB(t)
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatalf("set toggle off: %v", err)
	}
	id := insertProject(t, db, "p", "/tmp/repo-off", "owner/name")
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degrade, never error)", rec.Code)
	}
	if res := decodePRResult(t, rec); res.State != "disabled" {
		t.Errorf("state = %q, want disabled", res.State)
	}
	if fake.calls.Load() != 0 {
		t.Errorf("runner called %d times, want 0 (toggle gate must short-circuit before gh)", fake.calls.Load())
	}
}

// TestPullRequestsDisabledWhenUnlinked: integration on (default) but the
// project has NULL github_repo -> 200 + state=disabled, runner NOT invoked.
func TestPullRequestsDisabledWhenUnlinked(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-unlinked", "") // NULL github_repo
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if res := decodePRResult(t, rec); res.State != "disabled" {
		t.Errorf("state = %q, want disabled", res.State)
	}
	if fake.calls.Load() != 0 {
		t.Errorf("runner called %d times, want 0 (link gate must short-circuit before gh)", fake.calls.Load())
	}
}

// TestPullRequestsUnknownProjectDisabled: an unknown id still returns 200 +
// disabled (NOT 404) — the column never blocks on a project-lookup miss.
func TestPullRequestsUnknownProjectDisabled(t *testing.T) {
	db := newPRTestDB(t)
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)

	rec := prGet(t, mux, "/api/projects/9999/pull-requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for unknown project (degrade-don't-break)", rec.Code)
	}
	if res := decodePRResult(t, rec); res.State != "disabled" {
		t.Errorf("state = %q, want disabled for unknown project", res.State)
	}
	if fake.calls.Load() != 0 {
		t.Errorf("runner called %d times, want 0 for unknown project", fake.calls.Load())
	}
}

// TestPullRequestsOkWhenLinkedAndOn: integration on + project linked -> 200 +
// state=ok with the fake's PR list. The runner saw repo == github_repo and
// repoDir == repo_path (Pitfall 6).
func TestPullRequestsOkWhenLinkedAndOn(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-linked", "octo/widget")
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodePRResult(t, rec)
	if res.State != "ok" {
		t.Fatalf("state = %q, want ok", res.State)
	}
	if len(res.PRs) != 1 || res.PRs[0].Number != 7 {
		t.Errorf("prs = %+v, want the fake's single PR #7", res.PRs)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("runner called %d times, want 1", fake.calls.Load())
	}
	if fake.lastRepo != "octo/widget" {
		t.Errorf("runner repo = %q, want octo/widget (github_repo threaded through)", fake.lastRepo)
	}
	if fake.lastDir != "/tmp/repo-linked" {
		t.Errorf("runner repoDir = %q, want /tmp/repo-linked (repo_path threaded through, Pitfall 6)", fake.lastDir)
	}
}

// TestPullRequestsRefreshForcesFetch: a plain GET caches; a ?refresh=1 GET past
// the attempt floor forces a second fetch (force bypasses the 60s TTL).
func TestPullRequestsRefreshForcesFetch(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-refresh", "octo/widget")
	fake := &prFake{}
	cur := time.Now()
	mux := newPREnv(t, db, fake, func() time.Time { return cur })

	prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests")
	if fake.calls.Load() != 1 {
		t.Fatalf("calls after first GET = %d, want 1", fake.calls.Load())
	}

	cur = cur.Add(30 * time.Second) // inside 60s TTL, past the 10s floor

	// Plain GET inside the TTL serves the cache — no fetch.
	prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests")
	if fake.calls.Load() != 1 {
		t.Errorf("calls after plain GET within TTL = %d, want 1", fake.calls.Load())
	}
	// ?refresh=1 maps to force=true — bypasses the TTL (floor already satisfied).
	prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests?refresh=1")
	if fake.calls.Load() != 2 {
		t.Errorf("calls after ?refresh=1 past floor = %d, want 2", fake.calls.Load())
	}
}

// TestPullRequestsBadId: a non-numeric id is rejected with 400 (pathID). This
// is the ONLY non-200 the endpoint produces.
func TestPullRequestsBadId(t *testing.T) {
	db := newPRTestDB(t)
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)

	rec := prGet(t, mux, "/api/projects/abc/pull-requests")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for non-numeric id", rec.Code)
	}
}

// TestPullRequestsNeverNon200ForValidId: across every gate (off / unlinked /
// unknown / ok), a numeric id NEVER yields a non-200 — degradation rides the
// state field, never the HTTP status (GHCOL-05 / GHSET-03).
func TestPullRequestsNeverNon200ForValidId(t *testing.T) {
	db := newPRTestDB(t)
	off := insertProject(t, db, "off", "/tmp/repo-n1", "owner/name")
	unlinked := insertProject(t, db, "unl", "/tmp/repo-n2", "")
	linked := insertProject(t, db, "lnk", "/tmp/repo-n3", "owner/name")
	fake := &prFake{}

	// toggle on (default): linked & unlinked exercised.
	mux := newPREnv(t, db, fake, nil)
	for _, id := range []int64{unlinked, linked} {
		if rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests"); rec.Code != http.StatusOK {
			t.Errorf("id %d (toggle on): status = %d, want 200", id, rec.Code)
		}
	}

	// toggle off: the off-gate path also 200s.
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatalf("set toggle off: %v", err)
	}
	if rec := prGet(t, mux, "/api/projects/"+itoa(off)+"/pull-requests"); rec.Code != http.StatusOK {
		t.Errorf("id %d (toggle off): status = %d, want 200", off, rec.Code)
	}
}

// --- Review open-or-reattach endpoint (GHREV-01/02) ---

// reviewResp is the {task, pr} JSON the review endpoint returns.
type reviewResp struct {
	Task Task `json:"task"`
	PR   struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		Author      string `json:"author"`
		URL         string `json:"url"`
		BaseRefName string `json:"baseRefName"`
	} `json:"pr"`
}

func decodeReview(t *testing.T, rec *httptest.ResponseRecorder) reviewResp {
	t.Helper()
	var r reviewResp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode review body %q: %v", rec.Body.String(), err)
	}
	return r
}

func countPRRows(t *testing.T, db *sql.DB, projectID int64, prNumber int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE project_id = ? AND pr_number = ? AND source = 'github_pr'`,
		projectID, prNumber,
	).Scan(&n); err != nil {
		t.Fatalf("count pr rows: %v", err)
	}
	return n
}

// TestReviewToggleOff: integration off -> 409 BEFORE any gh/worktree work.
func TestReviewToggleOff(t *testing.T) {
	db := newPRTestDB(t)
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatalf("set toggle off: %v", err)
	}
	id := insertProject(t, db, "p", "/tmp/rev-off", "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())
	// Guard: viewPR must never be reached.
	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		t.Fatal("viewPR called despite toggle off")
		return github.PRDetail{}, nil
	})

	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for toggle off", rec.Code)
	}
	if countPRRows(t, db, id, 5) != 0 {
		t.Error("a PR row was created despite the toggle-off gate")
	}
}

// TestReviewUnlinkedProject: project with NULL github_repo -> 409 before gh.
func TestReviewUnlinkedProject(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/rev-unlinked", "") // NULL github_repo
	mux := newPRReviewEnv(t, db, t.TempDir())
	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		t.Fatal("viewPR called despite unlinked project")
		return github.PRDetail{}, nil
	})

	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for unlinked project", rec.Code)
	}
	if countPRRows(t, db, id, 5) != 0 {
		t.Error("a PR row was created despite the unlinked gate")
	}
}

// TestReviewUnknownProject: unknown project id -> 409 (can't open a review
// without a linked repo).
func TestReviewUnknownProject(t *testing.T) {
	db := newPRTestDB(t)
	mux := newPRReviewEnv(t, db, t.TempDir())
	rec := prPost(t, mux, "/api/projects/9999/pull-requests/5/review")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for unknown project", rec.Code)
	}
}

// TestReviewBadPRNumber: a non-numeric {n} -> 400.
func TestReviewBadPRNumber(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/rev-badn", "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())
	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/abc/review")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-numeric PR number", rec.Code)
	}
}

// TestReviewViewPRFailureDegrades: a gh ViewPR error surfaces as 502 and leaves
// NO half-created row.
func TestReviewViewPRFailureDegrades(t *testing.T) {
	db := newPRTestDB(t)
	repo, _ := makePRRepo(t, 5)
	id := insertProject(t, db, "p", repo, "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())
	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		return github.PRDetail{}, context.DeadlineExceeded
	})

	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when ViewPR fails", rec.Code)
	}
	if countPRRows(t, db, id, 5) != 0 {
		t.Error("a PR row was created despite the ViewPR failure (should be no half-created row)")
	}
}

// TestReviewCreateThenReattach is the GHREV-01/02 core: first open creates ONE
// github_pr row + a detached worktree; a second open returns the SAME task id
// with no duplicate row and no second worktree (the existing dir would make a
// re-provision fail, so same-id + dir-intact proves reattach).
func TestReviewCreateThenReattach(t *testing.T) {
	db := newPRTestDB(t)
	repo, headOID := makePRRepo(t, 5)
	id := insertProject(t, db, "p", repo, "owner/name")
	wtRoot := t.TempDir()
	mux := newPRReviewEnv(t, db, wtRoot)

	detail := github.PRDetail{
		Number: 5, Title: "Fix the thing", Body: "PR body here",
		AuthorLogin: "octocat", URL: "https://example.com/pr/5",
		HeadRefName: "feature", HeadRefOid: headOID,
		BaseRefName: "trunk", BaseRefOid: "deadbeef",
	}
	var viewCalls atomic.Int64
	stubViewPR(t, func(ctx context.Context, r string, n int) (github.PRDetail, error) {
		viewCalls.Add(1)
		if r != "owner/name" {
			t.Errorf("viewPR repo = %q, want owner/name", r)
		}
		if n != 5 {
			t.Errorf("viewPR n = %d, want 5", n)
		}
		return detail, nil
	})

	// First open: creates the row + worktree.
	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec.Code != http.StatusOK {
		t.Fatalf("first open status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	first := decodeReview(t, rec)
	if first.Task.Source != "github_pr" {
		t.Errorf("task.source = %q, want github_pr", first.Task.Source)
	}
	if first.Task.PRNumber == nil || *first.Task.PRNumber != 5 {
		t.Errorf("task.pr_number = %v, want 5", first.Task.PRNumber)
	}
	if first.Task.PRBaseRef == nil || *first.Task.PRBaseRef != "trunk" {
		t.Errorf("task.pr_base_ref = %v, want trunk", first.Task.PRBaseRef)
	}
	if first.Task.WorktreePath == nil || *first.Task.WorktreePath == "" {
		t.Fatalf("task.worktree_path is unset after a successful open")
	}
	if _, err := os.Stat(*first.Task.WorktreePath); err != nil {
		t.Errorf("worktree dir missing: %v", err)
	}
	// Live PR detail is returned (no drift).
	if first.PR.Title != "Fix the thing" || first.PR.Body != "PR body here" {
		t.Errorf("pr detail = %+v, want live title/body from ViewPR", first.PR)
	}
	if first.PR.Author != "octocat" || first.PR.BaseRefName != "trunk" {
		t.Errorf("pr detail author/base = %q/%q, want octocat/trunk", first.PR.Author, first.PR.BaseRefName)
	}
	if countPRRows(t, db, id, 5) != 1 {
		t.Fatalf("pr rows after first open = %d, want 1", countPRRows(t, db, id, 5))
	}
	firstWtPath := *first.Task.WorktreePath

	// Second open: reattach. SAME id, no duplicate row, worktree dir intact
	// (a re-provision would have failed on the existing path / made a new row).
	rec2 := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec2.Code != http.StatusOK {
		t.Fatalf("reattach status = %d (%s), want 200", rec2.Code, rec2.Body.String())
	}
	second := decodeReview(t, rec2)
	if second.Task.ID != first.Task.ID {
		t.Errorf("reattach task id = %d, want same id %d (GHREV-02)", second.Task.ID, first.Task.ID)
	}
	if countPRRows(t, db, id, 5) != 1 {
		t.Errorf("pr rows after reattach = %d, want 1 (no duplicate)", countPRRows(t, db, id, 5))
	}
	if second.Task.WorktreePath == nil || *second.Task.WorktreePath != firstWtPath {
		t.Errorf("reattach worktree_path = %v, want unchanged %q", second.Task.WorktreePath, firstWtPath)
	}
	if _, err := os.Stat(firstWtPath); err != nil {
		t.Errorf("worktree dir vanished on reattach: %v", err)
	}
}

// TestReviewReprovisionWhenWorktreeNull: a prior open that failed (row present,
// worktree_path NULL) re-provisions on reopen using the existing row id.
func TestReviewReprovisionWhenWorktreeNull(t *testing.T) {
	db := newPRTestDB(t)
	repo, headOID := makePRRepo(t, 5)
	id := insertProject(t, db, "p", repo, "owner/name")
	wtRoot := t.TempDir()
	mux := newPRReviewEnv(t, db, wtRoot)

	// Seed a failed-provision row: github_pr, pr_number 5, worktree_path NULL.
	res, err := db.Exec(
		`INSERT INTO tasks (project_id, title, status, source, pr_number, pr_base_ref, position, worktree_error)
		 VALUES (?, '#5 Old', 'todo', 'github_pr', 5, 'trunk', 0, 'earlier failure')`, id)
	if err != nil {
		t.Fatalf("seed failed row: %v", err)
	}
	existingID, _ := res.LastInsertId()

	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		return github.PRDetail{Number: 5, Title: "T", BaseRefName: "trunk", HeadRefOid: headOID}, nil
	})

	rec := prPost(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/5/review")
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	got := decodeReview(t, rec)
	if got.Task.ID != existingID {
		t.Errorf("reopen task id = %d, want existing %d (re-provision in place)", got.Task.ID, existingID)
	}
	if got.Task.WorktreePath == nil || *got.Task.WorktreePath == "" {
		t.Errorf("worktree_path still unset after re-provision")
	}
	if countPRRows(t, db, id, 5) != 1 {
		t.Errorf("pr rows after re-provision = %d, want 1 (no new row)", countPRRows(t, db, id, 5))
	}
}

// --- Live PR detail endpoint (12-07 re-hydration on reload) ---

// prDetailResp mirrors the bare prWire the GET .../{n} handler returns (NOT the
// {task, pr} envelope of POST /review).
type prDetailResp struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Author      string `json:"author"`
	URL         string `json:"url"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	Commits     int    `json:"commits"`
	State       string `json:"state"`
}

func decodePRDetail(t *testing.T, rec *httptest.ResponseRecorder) prDetailResp {
	t.Helper()
	var d prDetailResp
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode pr detail body %q: %v", rec.Body.String(), err)
	}
	return d
}

// TestPRDetailOkWhenLinked: GET .../{n} on a linked, integration-on project
// returns the live prWire (author/url/headRefName/commits), pure read — no
// worktree, no DB write. This is the F5 re-hydration path.
func TestPRDetailOkWhenLinked(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-detail-ok", "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())

	var viewCalls atomic.Int64
	stubViewPR(t, func(ctx context.Context, repo string, n int) (github.PRDetail, error) {
		viewCalls.Add(1)
		if repo != "owner/name" {
			t.Errorf("viewPR repo = %q, want owner/name", repo)
		}
		if n != 7 {
			t.Errorf("viewPR n = %d, want 7", n)
		}
		return github.PRDetail{
			Number: 7, Title: "Fix it", Body: "body",
			AuthorLogin: "octocat", URL: "https://example.com/pr/7",
			HeadRefName: "feature", BaseRefName: "trunk", Commits: 3,
			State: "MERGED",
		}, nil
	})

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	d := decodePRDetail(t, rec)
	if d.Number != 7 || d.Author != "octocat" || d.URL != "https://example.com/pr/7" {
		t.Errorf("detail number/author/url = %d/%q/%q, want 7/octocat/the url", d.Number, d.Author, d.URL)
	}
	if d.HeadRefName != "feature" || d.BaseRefName != "trunk" || d.Commits != 3 {
		t.Errorf("detail head/base/commits = %q/%q/%d, want feature/trunk/3", d.HeadRefName, d.BaseRefName, d.Commits)
	}
	if d.State != "MERGED" {
		t.Errorf("detail state = %q, want MERGED (drives the D-09 review banner)", d.State)
	}
	if viewCalls.Load() != 1 {
		t.Errorf("viewPR called %d times, want 1", viewCalls.Load())
	}
	// No PR row should have been created — this is a pure read.
	if countPRRows(t, db, id, 7) != 0 {
		t.Errorf("pr rows after a pure GET detail = %d, want 0 (no DB write)", countPRRows(t, db, id, 7))
	}
}

// TestPRDetailToggleOff: integration off -> 409 BEFORE any gh read.
func TestPRDetailToggleOff(t *testing.T) {
	db := newPRTestDB(t)
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatalf("set toggle off: %v", err)
	}
	id := insertProject(t, db, "p", "/tmp/repo-detail-off", "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())
	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		t.Fatal("viewPR called despite toggle off")
		return github.PRDetail{}, nil
	})

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/7")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for toggle off", rec.Code)
	}
}

// TestPRDetailUnlinkedProject: NULL github_repo -> 409 before gh.
func TestPRDetailUnlinkedProject(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-detail-unlinked", "") // NULL github_repo
	mux := newPRReviewEnv(t, db, t.TempDir())
	stubViewPR(t, func(context.Context, string, int) (github.PRDetail, error) {
		t.Fatal("viewPR called despite unlinked project")
		return github.PRDetail{}, nil
	})

	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/7")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for unlinked project", rec.Code)
	}
}

// TestPRDetailBadPRNumber: a non-numeric {n} -> 400.
func TestPRDetailBadPRNumber(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/repo-detail-badn", "owner/name")
	mux := newPRReviewEnv(t, db, t.TempDir())
	rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests/abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-numeric PR number", rec.Code)
	}
}

// itoa is a tiny local int64→string helper to keep the test imports lean.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

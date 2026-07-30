package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/github"
	"kamacu/internal/settings"
)

// noopReviewRunner is injected as the Service's review-column Runner so the
// activity tests can never accidentally spawn gh via that seam (the activity
// handler only calls GetMergedClosed, never Get).
func noopReviewRunner(context.Context, string, string) (github.PRLists, string, error) {
	return github.PRLists{}, "ok", nil
}

// safeCompletedNoGh is the default CompletedRunner when a test passes nil: it
// returns a no_gh degrade with zero gh spawn, so a test that accidentally leaves
// a linked repo in scope can never hang on a real gh spawn.
func safeCompletedNoGh(context.Context, string, string) ([]github.ReviewDoneSummary, string, error) {
	return nil, "no_gh", nil
}

// newActivityEnv wires a fresh mux with ActivityRoutes backed by a Service whose
// CompletedRunner is the supplied fake (review-column Runner is a no-op so no gh
// spawn is possible). now may be nil for the real clock; tests that advance past
// the attempt floor (refresh propagation) inject a controllable clock. A nil
// completed defaults to a no-spawn no_gh runner (never listCompletedReviews).
func newActivityEnv(t *testing.T, db *sql.DB, completed func(ctx context.Context, repo, repoDir string) ([]github.ReviewDoneSummary, string, error), now func() time.Time) *http.ServeMux {
	t.Helper()
	if completed == nil {
		completed = safeCompletedNoGh
	}
	cfg := github.Config{
		Runner:          noopReviewRunner,
		CompletedRunner: completed,
		Now:             now,
	}
	svc := github.New(cfg)
	mux := http.NewServeMux()
	ActivityRoutes(mux, db, svc)
	return mux
}

// seedWorkspace is defined in projects_test.go (same package) and reused here:
// it inserts a workspace row and returns its id (Personal=1 already exists from
// migration 00012, so new rows yield id>=2).

// seedActivityProject inserts a project row with an explicit workspace_id and an
// optional github_repo link. repoPath must be unique per row (UNIQUE column).
func seedActivityProject(t *testing.T, db *sql.DB, name, repoPath, repo string, workspaceID int64) int64 {
	t.Helper()
	var ghRepo any
	if repo != "" {
		ghRepo = repo
	}
	res, err := db.Exec(
		`INSERT INTO projects(name, repo_path, github_repo, workspace_id) VALUES(?, ?, ?, ?)`,
		name, repoPath, ghRepo, workspaceID,
	)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("project last id: %v", err)
	}
	return id
}

// strToNull maps an empty string to a SQL NULL argument; non-empty passes through.
func strToNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// seedDoneTask inserts a status='done' task with the *_at columns set (empty
// string → NULL). source discriminates manual vs github_pr so callers can prove
// the D-12 board-leak filter excludes github_pr rows.
func seedDoneTask(t *testing.T, db *sql.DB, projectID int64, title, source, doneAt, inProgAt, inRevAt string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO tasks(project_id, title, status, source, position, done_at, in_progress_at, in_review_at)
		 VALUES (?, ?, 'done', ?, 0, ?, ?, ?)`,
		projectID, title, source, strToNull(doneAt), strToNull(inProgAt), strToNull(inRevAt),
	)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("task last id: %v", err)
	}
	return id
}

// completedFixture is the canned (prs, state) a per-repo fake returns.
type completedFixture struct {
	prs   []github.ReviewDoneSummary
	state string
}

// completedRunnerByRepo builds a CompletedRunner keyed by repo name so repo A
// and repo B can return different states (the partial-degrade scenario). The
// shared call counter proves how many gh spawns happened.
func completedRunnerByRepo(table map[string]completedFixture, calls *atomic.Int64) func(ctx context.Context, repo, repoDir string) ([]github.ReviewDoneSummary, string, error) {
	return func(ctx context.Context, repo, repoDir string) ([]github.ReviewDoneSummary, string, error) {
		if calls != nil {
			calls.Add(1)
		}
		f, ok := table[repo]
		if !ok {
			return nil, "error", nil
		}
		return f.prs, f.state, nil
	}
}

// activityGet drives a GET through the mux and returns the recorder.
func activityGet(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// decodeActivity unmarshals a body into activityResponse (same package — full
// typed access to Tasks/Reviews/Stats including the ReviewDoneSummary PRs).
func decodeActivity(t *testing.T, rec *httptest.ResponseRecorder) activityResponse {
	t.Helper()
	var res activityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return res
}

// msISO formats a time as the reaper's millisecond-ISO done_at storage format
// (the shape the tasks-done SQL lexical-compares against).
func msISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// secISO formats a time as gh's second-precision closedAt shape (the bare-Z
// layout parseActivityTime's fallback handles — used for ReviewDoneSummary
// fixtures to prove the precision-mismatch comparison).
func secISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// TestActivityHandler_GlobalScopeAndWindow: global scope returns the in-window
// manual done task, excludes the out-of-window task, excludes source='github_pr'
// rows (D-12), and orders done_at DESC.
func TestActivityHandler_GlobalScopeAndWindow(t *testing.T) {
	db := newPRTestDB(t)
	pid := seedActivityProject(t, db, "P", "/tmp/act-global", "", 1)
	now := time.Now().UTC()

	// Two in-window manual tasks with distinct done_at (newer + older) so the
	// DESC ordering is genuinely asserted, plus one out-of-window manual task
	// and one in-window github_pr task (both excluded).
	newer := seedDoneTask(t, db, pid, "in-window newer", "manual", msISO(now.Add(-1*24*time.Hour)), "", "")
	older := seedDoneTask(t, db, pid, "in-window older", "manual", msISO(now.Add(-3*24*time.Hour)), "", "")
	outWin := seedDoneTask(t, db, pid, "out-of-window manual", "manual", msISO(now.Add(-20*24*time.Hour)), "", "")
	ghPR := seedDoneTask(t, db, pid, "done github_pr", "github_pr", msISO(now.Add(-1*time.Hour)), "", "")
	_ = outWin
	_ = ghPR

	mux := newActivityEnv(t, db, func(context.Context, string, string) ([]github.ReviewDoneSummary, string, error) {
		t.Error("CompletedRunner called under GATE 1 default-on with an unlinked project")
		return nil, "ok", nil
	}, nil)

	// No linked project -> reviews.state="ok" empty prs; the runner must NOT fire.
	rec := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)

	if len(res.Tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2 (the two in-window manual tasks); got %+v", len(res.Tasks), res.Tasks)
	}
	// done_at DESC: newer (1d ago) before older (3d ago).
	if res.Tasks[0].ID != newer {
		t.Errorf("tasks[0].id = %d, want the newer task %d (done_at DESC)", res.Tasks[0].ID, newer)
	}
	if res.Tasks[1].ID != older {
		t.Errorf("tasks[1].id = %d, want the older task %d (done_at DESC)", res.Tasks[1].ID, older)
	}
	for _, tk := range res.Tasks {
		if tk.Title == "done github_pr" {
			t.Error("github_pr row leaked into tasks-done (D-12 filter broken)")
		}
		if tk.Title == "out-of-window manual" {
			t.Error("out-of-window task leaked into tasks-done (window filter broken)")
		}
	}
	// Reviews: no linked repos -> ok with empty prs.
	if res.Reviews.State != "ok" {
		t.Errorf("reviews.state = %q, want ok (no linked repos)", res.Reviews.State)
	}
	if res.Stats.TaskCount != 2 {
		t.Errorf("stats.taskCount = %d, want 2", res.Stats.TaskCount)
	}
}

// TestActivityHandler_WorkspaceScope: ?scope=workspace:N narrows tasks to that
// workspace's projects.
func TestActivityHandler_WorkspaceScope(t *testing.T) {
	db := newPRTestDB(t)
	alpha := seedWorkspace(t, db, "Alpha")
	pPersonal := seedActivityProject(t, db, "Personal-proj", "/tmp/act-ws-pp", "", 1)
	pAlpha := seedActivityProject(t, db, "Alpha-proj", "/tmp/act-ws-ap", "", alpha)
	now := time.Now().UTC()
	seedDoneTask(t, db, pPersonal, "personal task", "manual", msISO(now.Add(-1*time.Hour)), "", "")
	alphaTask := seedDoneTask(t, db, pAlpha, "alpha task", "manual", msISO(now.Add(-2*time.Hour)), "", "")

	mux := newActivityEnv(t, db, nil, nil)
	rec := activityGet(t, mux, "/api/activity?scope=workspace:"+itoa(alpha)+"&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)
	if len(res.Tasks) != 1 || res.Tasks[0].ID != alphaTask {
		t.Errorf("tasks = %+v, want only the alpha-workspace task %d", res.Tasks, alphaTask)
	}
}

// TestActivityHandler_ProjectScope: ?scope=project:N narrows to that project.
func TestActivityHandler_ProjectScope(t *testing.T) {
	db := newPRTestDB(t)
	p1 := seedActivityProject(t, db, "P1", "/tmp/act-ps-1", "", 1)
	p2 := seedActivityProject(t, db, "P2", "/tmp/act-ps-2", "", 1)
	now := time.Now().UTC()
	seedDoneTask(t, db, p1, "p1 task", "manual", msISO(now.Add(-1*time.Hour)), "", "")
	p2Task := seedDoneTask(t, db, p2, "p2 task", "manual", msISO(now.Add(-2*time.Hour)), "", "")

	mux := newActivityEnv(t, db, nil, nil)
	rec := activityGet(t, mux, "/api/activity?scope=project:"+itoa(p2)+"&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)
	if len(res.Tasks) != 1 || res.Tasks[0].ID != p2Task {
		t.Errorf("tasks = %+v, want only project %d's task", res.Tasks, p2)
	}
}

// TestActivityHandler_WindowMonth: a 20-day-old task is excluded under week but
// included under month.
func TestActivityHandler_WindowMonth(t *testing.T) {
	db := newPRTestDB(t)
	pid := seedActivityProject(t, db, "P", "/tmp/act-wm", "", 1)
	now := time.Now().UTC()
	oldTask := seedDoneTask(t, db, pid, "20 days ago", "manual", msISO(now.Add(-20*24*time.Hour)), "", "")

	mux := newActivityEnv(t, db, nil, nil)

	recWeek := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if recWeek.Code != http.StatusOK {
		t.Fatalf("week status = %d", recWeek.Code)
	}
	if res := decodeActivity(t, recWeek); len(res.Tasks) != 0 {
		t.Errorf("week tasks len = %d, want 0 (20-day-old out of week window); got %+v", len(res.Tasks), res.Tasks)
	}

	recMonth := activityGet(t, mux, "/api/activity?scope=global&window=month")
	if recMonth.Code != http.StatusOK {
		t.Fatalf("month status = %d", recMonth.Code)
	}
	if res := decodeActivity(t, recMonth); len(res.Tasks) != 1 || res.Tasks[0].ID != oldTask {
		t.Errorf("month tasks = %+v, want the 20-day-old task %d (inside 30d window)", res.Tasks, oldTask)
	}
}

// TestActivityHandler_ReviewsWindowFilter: a PR whose completedAt (gh closedAt,
// second-precision) is 10 days ago is DROPPED under window=week but INCLUDED
// under window=month. Proves cutoff threads into aggregateReviews and the
// second-precision closedAt is parsed via parseActivityTime (not string-compared
// across the ms/s precision mismatch). REVIEWS-01 / STATS-01.
func TestActivityHandler_ReviewsWindowFilter(t *testing.T) {
	db := newPRTestDB(t)
	seedActivityProject(t, db, "P", "/tmp/act-rwf", "octo/window", 1)
	now := time.Now().UTC()
	prOld := github.ReviewDoneSummary{
		Number: 10, Title: "closed 10d ago",
		CompletedAt: secISO(now.Add(-10 * 24 * time.Hour)), // second-precision (gh closedAt shape)
		URL:         "https://example.com/pr/10",
	}
	prNew := github.ReviewDoneSummary{
		Number: 11, Title: "closed 1d ago",
		CompletedAt: secISO(now.Add(-1 * 24 * time.Hour)),
		URL:         "https://example.com/pr/11",
	}
	var calls atomic.Int64
	mux := newActivityEnv(t, db, completedRunnerByRepo(map[string]completedFixture{
		"octo/window": {prs: []github.ReviewDoneSummary{prOld, prNew}, state: "ok"},
	}, &calls), nil)

	// Week window: cutoff = now - 7d. The 10-day PR is before the cutoff -> dropped.
	recWeek := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if recWeek.Code != http.StatusOK {
		t.Fatalf("week status = %d", recWeek.Code)
	}
	resWeek := decodeActivity(t, recWeek)
	if len(resWeek.Reviews.PRs) != 1 {
		t.Fatalf("week reviews.prs len = %d, want 1 (10-day PR dropped, 1-day PR kept); got %+v", len(resWeek.Reviews.PRs), resWeek.Reviews.PRs)
	}
	if resWeek.Reviews.PRs[0].Number != 11 {
		t.Errorf("week surviving PR number = %d, want 11 (the 1-day PR)", resWeek.Reviews.PRs[0].Number)
	}

	// Month window: cutoff = now - 30d. Both survive.
	recMonth := activityGet(t, mux, "/api/activity?scope=global&window=month")
	if recMonth.Code != http.StatusOK {
		t.Fatalf("month status = %d", recMonth.Code)
	}
	resMonth := decodeActivity(t, recMonth)
	if len(resMonth.Reviews.PRs) != 2 {
		t.Fatalf("month reviews.prs len = %d, want 2 (both within 30d); got %+v", len(resMonth.Reviews.PRs), resMonth.Reviews.PRs)
	}
}

// TestActivityHandler_Gate1Disabled: github_integration off -> reviews.state
// "disabled", ZERO gh spawns, tasks + stats still populate. REVIEWS-04 / D-02.
func TestActivityHandler_Gate1Disabled(t *testing.T) {
	db := newPRTestDB(t)
	if err := settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatalf("set toggle off: %v", err)
	}
	pid := seedActivityProject(t, db, "P", "/tmp/act-gate", "octo/linked", 1)
	now := time.Now().UTC()
	seedDoneTask(t, db, pid, "done task", "manual", msISO(now.Add(-1*time.Hour)), "", "")

	var calls atomic.Int64
	mux := newActivityEnv(t, db, completedRunnerByRepo(map[string]completedFixture{
		"octo/linked": {prs: []github.ReviewDoneSummary{{Number: 1, CompletedAt: secISO(now)}}, state: "ok"},
	}, &calls), nil)

	rec := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)
	if res.Reviews.State != "disabled" {
		t.Errorf("reviews.state = %q, want disabled (GATE 1 off)", res.Reviews.State)
	}
	if calls.Load() != 0 {
		t.Errorf("CompletedRunner called %d times, want 0 (GATE 1 must short-circuit before any gh spawn)", calls.Load())
	}
	if len(res.Tasks) != 1 {
		t.Errorf("tasks len = %d, want 1 (tasks populate regardless of gh state)", len(res.Tasks))
	}
	if res.Stats.TaskCount != 1 {
		t.Errorf("stats.taskCount = %d, want 1", res.Stats.TaskCount)
	}
}

// TestActivityHandler_NoGh: every in-scope repo degrades no_gh -> reviews.state
// "no_gh", prs empty, tasks + stats still populate.
func TestActivityHandler_NoGh(t *testing.T) {
	db := newPRTestDB(t)
	seedActivityProject(t, db, "A", "/tmp/act-nogh-a", "octo/a", 1)
	seedActivityProject(t, db, "B", "/tmp/act-nogh-b", "octo/b", 1)
	pid := seedActivityProject(t, db, "Tasks", "/tmp/act-nogh-t", "", 1)
	now := time.Now().UTC()
	seedDoneTask(t, db, pid, "done", "manual", msISO(now.Add(-1*time.Hour)), "", "")

	var calls atomic.Int64
	mux := newActivityEnv(t, db, completedRunnerByRepo(map[string]completedFixture{
		"octo/a": {state: "no_gh"},
		"octo/b": {state: "no_gh"},
	}, &calls), nil)

	rec := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)
	if res.Reviews.State != "no_gh" {
		t.Errorf("reviews.state = %q, want no_gh (all repos degraded)", res.Reviews.State)
	}
	if len(res.Reviews.PRs) != 0 {
		t.Errorf("reviews.prs len = %d, want 0 (all degraded)", len(res.Reviews.PRs))
	}
	if calls.Load() != 2 {
		t.Errorf("runner calls = %d, want 2 (one per linked repo)", calls.Load())
	}
	if len(res.Tasks) != 1 {
		t.Errorf("tasks len = %d, want 1", len(res.Tasks))
	}
}

// TestActivityHandler_Partial: repo A ok + repo B no_gh -> reviews.state
// "partial", prs carries only A's PRs annotated with A's provenance. D-08.
func TestActivityHandler_Partial(t *testing.T) {
	db := newPRTestDB(t)
	pa := seedActivityProject(t, db, "ProjA", "/tmp/act-part-a", "octo/a", 1)
	seedActivityProject(t, db, "ProjB", "/tmp/act-part-b", "octo/b", 1)
	now := time.Now().UTC()

	prA := github.ReviewDoneSummary{
		Number: 42, Title: "A's PR",
		CompletedAt: secISO(now.Add(-1 * time.Hour)),
		URL:         "https://example.com/pr/42",
	}
	mux := newActivityEnv(t, db, completedRunnerByRepo(map[string]completedFixture{
		"octo/a": {prs: []github.ReviewDoneSummary{prA}, state: "ok"},
		"octo/b": {state: "no_gh"},
	}, nil), nil)

	rec := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)
	if res.Reviews.State != "partial" {
		t.Errorf("reviews.state = %q, want partial (ok + no_gh mix)", res.Reviews.State)
	}
	if len(res.Reviews.PRs) != 1 {
		t.Fatalf("reviews.prs len = %d, want 1 (only repo A's PR); got %+v", len(res.Reviews.PRs), res.Reviews.PRs)
	}
	p := res.Reviews.PRs[0]
	if p.Number != 42 {
		t.Errorf("prs[0].number = %d, want 42", p.Number)
	}
	if p.ProjectName != "ProjA" {
		t.Errorf("prs[0].projectName = %q, want ProjA (provenance annotated)", p.ProjectName)
	}
	if p.ProjectID != pa {
		t.Errorf("prs[0].projectId = %d, want %d", p.ProjectID, pa)
	}
	if p.Repo != "octo/a" {
		t.Errorf("prs[0].repo = %q, want octo/a", p.Repo)
	}
}

// TestActivityHandler_StatsOverSeededDB: hand-computed cycle/dwellInProgress/
// dwellInReview over known *_at timestamps, including the even-N median (Pitfall
// 6) and the dwellInProgress branch (in_review_at set vs NULL — Open Q3).
func TestActivityHandler_StatsOverSeededDB(t *testing.T) {
	db := newPRTestDB(t)
	pid := seedActivityProject(t, db, "P", "/tmp/act-stats", "", 1)
	now := time.Now().UTC()

	// Task 1: full InProgress -> InReview -> Done, 2 days ago.
	base1 := now.Add(-2 * 24 * time.Hour)
	seedDoneTask(t, db, pid, "full path", "manual",
		msISO(base1.Add(3*time.Hour)),            // done_at = base+3h
		msISO(base1),                             // in_progress_at = base
		msISO(base1.Add(1*time.Hour)))            // in_review_at = base+1h
	// cycle = 3h = 10800s; dwellInProgress = inRev-inProg = 1h = 3600s; dwellInReview = done-inRev = 2h = 7200s

	// Task 2: skipped In Review (in_review_at NULL), 1 day ago.
	base2 := now.Add(-1 * 24 * time.Hour)
	seedDoneTask(t, db, pid, "skipped review", "manual",
		msISO(base2.Add(2*time.Hour)), // done_at = base+2h
		msISO(base2),                  // in_progress_at = base
		"")                            // in_review_at NULL
	// cycle = 2h = 7200s; dwellInProgress = done-inProg = 2h = 7200s (Open Q3 fallback); dwellInReview excluded

	mux := newActivityEnv(t, db, nil, nil)
	rec := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	res := decodeActivity(t, rec)

	if res.Stats.TaskCount != 2 {
		t.Fatalf("taskCount = %d, want 2", res.Stats.TaskCount)
	}
	// cycle over [10800s, 7200s] -> sorted [7200,10800], n=2 even, median=9000.
	wantCycle := timeStat{N: 2, Min: 7200, Max: 10800, Median: 9000}
	if res.Stats.Cycle != wantCycle {
		t.Errorf("cycle = %+v, want %+v", res.Stats.Cycle, wantCycle)
	}
	// dwellInProgress over [3600s (task1), 7200s (task2 fallback)] -> median=5400.
	wantDIP := timeStat{N: 2, Min: 3600, Max: 7200, Median: 5400}
	if res.Stats.DwellInProgress != wantDIP {
		t.Errorf("dwellInProgress = %+v, want %+v (Open Q3 fallback path)", res.Stats.DwellInProgress, wantDIP)
	}
	// dwellInReview over [7200s (task1 only; task2 excluded)] -> n=1.
	wantDIR := timeStat{N: 1, Min: 7200, Max: 7200, Median: 7200}
	if res.Stats.DwellInReview != wantDIR {
		t.Errorf("dwellInReview = %+v, want %+v", res.Stats.DwellInReview, wantDIR)
	}
	// reviewCount is a plain number and reviews carries no cycle/dwell object (STATS-04).
	if res.Stats.ReviewCount != 0 {
		t.Errorf("reviewCount = %d, want 0 (no linked repos)", res.Stats.ReviewCount)
	}
}

// TestActivityHandler_RefreshPropagation: ?refresh=1 fans force=true out to every
// in-scope repo's GetMergedClosed, bypassing the 5min TTL (bound by the 10s
// attempt floor). Two linked repos; a controllable clock advances past the floor
// between the cached first call and the refresh call. Open Q4.
func TestActivityHandler_RefreshPropagation(t *testing.T) {
	db := newPRTestDB(t)
	seedActivityProject(t, db, "A", "/tmp/act-ref-a", "octo/a", 1)
	seedActivityProject(t, db, "B", "/tmp/act-ref-b", "octo/b", 1)
	now := time.Now().UTC()
	pr := github.ReviewDoneSummary{Number: 7, Title: "fresh", CompletedAt: secISO(now.Add(-1 * time.Hour))}

	var calls atomic.Int64
	cur := time.Now()
	mux := newActivityEnv(t, db, completedRunnerByRepo(map[string]completedFixture{
		"octo/a": {prs: []github.ReviewDoneSummary{pr}, state: "ok"},
		"octo/b": {prs: []github.ReviewDoneSummary{pr}, state: "ok"},
	}, &calls), func() time.Time { return cur })

	// First call (no refresh): caches both repos; runner fires once per repo.
	rec1 := activityGet(t, mux, "/api/activity?scope=global&window=week")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d", rec1.Code)
	}
	if calls.Load() != 2 {
		t.Fatalf("runner calls after first GET = %d, want 2 (one per repo)", calls.Load())
	}

	// Advance past the 10s attempt floor but stay inside the 5min TTL.
	cur = cur.Add(15 * time.Second)

	// ?refresh=1 -> force=true bypasses the TTL (floor already satisfied);
	// runner fires once per repo again.
	rec2 := activityGet(t, mux, "/api/activity?scope=global&window=week&refresh=1")
	if rec2.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", rec2.Code)
	}
	if calls.Load() != 4 {
		t.Errorf("runner calls after refresh = %d, want 4 (force bypassed TTL on both repos)", calls.Load())
	}
}

// TestActivityHandler_InvalidScope: an unknown scope prefix -> HTTP 400.
func TestActivityHandler_InvalidScope(t *testing.T) {
	db := newPRTestDB(t)
	mux := newActivityEnv(t, db, nil, nil)
	rec := activityGet(t, mux, "/api/activity?scope=bogus&window=week")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid scope", rec.Code)
	}
}

// TestActivityHandler_InvalidWindow: an unknown window value -> HTTP 400.
func TestActivityHandler_InvalidWindow(t *testing.T) {
	db := newPRTestDB(t)
	mux := newActivityEnv(t, db, nil, nil)
	rec := activityGet(t, mux, "/api/activity?scope=global&window=decade")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid window", rec.Code)
	}
}

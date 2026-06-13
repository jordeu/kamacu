package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"kangent/internal/github"
	"kangent/internal/settings"
	"kangent/internal/store"
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

func (f *prFake) run(ctx context.Context, repo, repoDir string) ([]github.PRSummary, string, error) {
	f.calls.Add(1)
	f.lastRepo, f.lastDir = repo, repoDir
	return []github.PRSummary{{Number: 7, Title: "fake", Author: "octocat", Checks: "pass"}}, "ok", nil
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
// need to advance past the attempt floor inject a controllable clock.
func newPREnv(t *testing.T, db *sql.DB, fake *prFake, now func() time.Time) *http.ServeMux {
	t.Helper()
	svc := github.New(github.Config{Runner: fake.run, Now: now})
	mux := http.NewServeMux()
	PullRequestRoutes(mux, db, svc)
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

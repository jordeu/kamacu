package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kamacu/internal/github"
	"kamacu/internal/worktree"
)

func setPRVisibility(t *testing.T, mux *http.ServeMux, path, body string, want int) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
	if rec.Code != want {
		t.Fatalf("PUT %s = %d: %s, want %d", path, rec.Code, rec.Body.String(), want)
	}
}

func TestPRVisibilityPersistsAndFiltersBothQueues(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/hidden-prs", "org/repo")
	other := insertProject(t, db, "other", "/tmp/other-hidden-prs", "org/repo")
	calls := 0
	makeMux := func() *http.ServeMux {
		svc := github.New(github.Config{Runner: func(context.Context, string, string) (github.PRLists, string, error) {
			calls++
			return github.PRLists{
				Awaiting: []github.PRSummary{{Number: 7}, {Number: 8}},
				Reviewed: []github.PRSummary{{Number: 9}},
			}, "ok", nil
		}})
		mux := http.NewServeMux()
		PullRequestRoutes(mux, db, svc, worktree.NewService(t.TempDir()))
		return mux
	}
	mux := makeMux()
	base := "/api/projects/" + itoa(id) + "/pull-requests"
	read := func(path string) github.Result {
		t.Helper()
		rec := prGet(t, mux, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		return decodePRResult(t, rec)
	}
	if got := read(base); len(got.PRs) != 2 || len(got.Reviewed) != 1 {
		t.Fatalf("initial queues: %+v", got)
	}
	for _, n := range []string{"7", "9", "7"} { // hiding is idempotent
		setPRVisibility(t, mux, base+"/"+n+"/visibility", `{"hidden":true}`, http.StatusOK)
	}
	if got := read(base); len(got.PRs) != 1 || got.PRs[0].Number != 8 || len(got.Reviewed) != 0 {
		t.Fatalf("hidden queues: %+v", got)
	}
	assertIncluded := func() {
		t.Helper()
		got := read(base + "?include_hidden=1&refresh=1")
		if len(got.PRs) != 2 || !got.PRs[0].Hidden || got.PRs[1].Hidden || len(got.Reviewed) != 1 || !got.Reviewed[0].Hidden {
			t.Fatalf("included queues: %+v", got)
		}
	}
	assertIncluded()
	if got := read("/api/projects/" + itoa(other) + "/pull-requests?include_hidden=1"); len(got.PRs) != 2 || got.PRs[0].Hidden || got.Reviewed[0].Hidden {
		t.Fatalf("preferences leaked to other project: %+v", got)
	}
	if calls != 1 {
		t.Fatalf("visibility should not force GitHub fetches, got %d", calls)
	}
	mux = makeMux() // new cache/server still uses persisted choices
	assertIncluded()
	if _, err := db.Exec(`UPDATE projects SET github_repo = 'org/different' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if got := read(base); len(got.PRs) != 2 || len(got.Reviewed) != 1 {
		t.Fatalf("preferences leaked after relinking: %+v", got)
	}
	if _, err := db.Exec(`UPDATE projects SET github_repo = 'ORG/REPO' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	assertIncluded() // repository identity is case insensitive
	for _, n := range []string{"7", "9", "7"} {
		setPRVisibility(t, mux, base+"/"+n+"/visibility", `{"hidden":false}`, http.StatusOK)
	}
	if got := read(base); len(got.PRs) != 2 || got.PRs[0].Hidden || len(got.Reviewed) != 1 {
		t.Fatalf("restored queues: %+v", got)
	}
	var tasks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("visibility created %d tasks", tasks)
	}
}

func TestPRVisibilityValidation(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "linked", "/tmp/visibility-validation", "org/repo")
	unlinked := insertProject(t, db, "unlinked", "/tmp/visibility-unlinked", "")
	fake := &prFake{}
	mux := newPREnv(t, db, fake, nil)
	base := "/api/projects/" + itoa(id) + "/pull-requests/"
	for _, n := range []string{"abc", "0", "-1"} {
		setPRVisibility(t, mux, base+n+"/visibility", `{"hidden":true}`, http.StatusBadRequest)
	}
	for _, body := range []string{`{`, `{}`, `{"hidden":null}`, `{"hidden":"yes"}`} {
		setPRVisibility(t, mux, base+"7/visibility", body, http.StatusBadRequest)
	}
	setPRVisibility(t, mux, "/api/projects/99999/pull-requests/7/visibility", `{"hidden":true}`, http.StatusNotFound)
	setPRVisibility(t, mux, "/api/projects/"+itoa(unlinked)+"/pull-requests/7/visibility", `{"hidden":true}`, http.StatusConflict)
	if fake.calls.Load() != 0 {
		t.Fatal("visibility invoked GitHub")
	}
}

func TestPRVisibilityReadFailureDegradesWithoutExposingHiddenPRs(t *testing.T) {
	db := newPRTestDB(t)
	id := insertProject(t, db, "p", "/tmp/visibility-read-failure", "org/repo")
	mux := newPREnv(t, db, &prFake{}, nil)
	if _, err := db.Exec(`DROP TABLE hidden_pr_reviews`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?include_hidden=1"} {
		rec := prGet(t, mux, "/api/projects/"+itoa(id)+"/pull-requests"+query)
		if rec.Code != http.StatusOK {
			t.Fatalf("list must degrade with HTTP 200, got %d: %s", rec.Code, rec.Body.String())
		}
		got := decodePRResult(t, rec)
		if got.State != "error" || len(got.PRs) != 0 || len(got.Reviewed) != 0 {
			t.Fatalf("unexpected failure result: %+v", got)
		}
	}
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kamacu/internal/github"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

func TestProjectPRStatusesScopeDedupAndDisabled(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "status.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO projects(id,name,repo_path) VALUES(1,'one','/one'),(2,'two','/two')`,
		`INSERT INTO tasks(id,project_id,title,source,position) VALUES(1,1,'one','manual',0),(2,1,'two','manual',1),(3,2,'other','manual',0),(4,1,'review','github_pr',2)`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct {
		task, n int64
		role    string
	}{{1, 7, "implementation"}, {2, 7, "implementation"}, {1, 8, "review"}, {1, 9, "incidental"}, {3, 10, "implementation"}, {4, 11, "review"}} {
		if err = store.PutPRLink(t.Context(), db, store.PRLink{TaskID: entry.task, Repository: "acme/repo", Number: entry.n, Role: entry.role, Source: "explicit", Evidence: "test", ObservedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	svc := github.NewPRStatusService(func(_ context.Context, repo string, n int64) (github.PRStatus, error) {
		calls.Add(1)
		out := github.UnknownPRStatus(repo, n, "")
		out.Humans.State = "approved"
		out.CI.State = "pass"
		now := time.Now()
		out.FetchedAt = &now
		return out, nil
	}, nil)
	h := prContextHandlers{db: db, statuses: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/task-pr-statuses", h.projectStatuses)
	mux.HandleFunc("POST /api/projects/{id}/task-pr-statuses/refresh", h.refreshStatus)
	get := func(path string) (int, []github.PRStatus) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			return w.Code, nil
		}
		var response struct{ Statuses []github.PRStatus }
		if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return w.Code, response.Statuses
	}
	for range 2 {
		code, statuses := get("/api/projects/1/task-pr-statuses")
		if code != 200 || len(statuses) != 2 || statuses[0].Number != 7 || statuses[1].Number != 8 {
			t.Fatal(code, statuses)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("duplicate links or polls caused duplicate GitHub reads", calls.Load())
	}
	if _, err = db.Exec(`UPDATE tasks SET status='in_progress' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if code, active := get("/api/projects/1/task-pr-statuses?active=1"); code != 200 || len(active) != 1 || active[0].Number != 7 {
		t.Fatal("active overview must exclude backlog, completed and review-workspace links", code, active)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='in_review' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if code, active := get("/api/projects/1/task-pr-statuses?active=1"); code != 200 || len(active) != 2 {
		t.Fatal("active overview must include review-stage links", code, active)
	}
	if _, err = db.Exec(`UPDATE tasks SET status='done' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if code, active := get("/api/projects/1/task-pr-statuses?active=1"); code != 200 || len(active) != 1 {
		t.Fatal("completed task links must leave the active overview", code, active)
	}

	if err = settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatal(err)
	}
	_, statuses := get("/api/projects/1/task-pr-statuses")
	if len(statuses) != 2 || statuses[0].Humans.State != "unknown" || statuses[0].Error != "GitHub integration disabled" || calls.Load() != 2 {
		t.Fatal(statuses, calls.Load())
	}
	if code, _ := get("/api/projects/404/task-pr-statuses"); code != 404 {
		t.Fatal(code)
	}
	if err = settings.Set(db, settings.KeyGithubIntegration, "on"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		project, repo string
		number        int
		code          int
	}{
		{"1", "ACME/REPO", 7, 200},
		{"1", "acme/repo", 7, 200},
		{"1", "acme/repo", 9, 404},  // Rejected association.
		{"1", "acme/repo", 10, 404}, // Other project's PR.
		{"404", "acme/repo", 7, 404},
		{"1", "invalid", 7, 400},
		{"1", "acme/repo", 0, 400},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/projects/"+tc.project+"/task-pr-statuses/refresh", strings.NewReader(fmt.Sprintf(`{"repository":%q,"number":%d}`, tc.repo, tc.number))))
		if w.Code != tc.code {
			t.Fatalf("refresh %+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
	if calls.Load() != 4 {
		t.Fatal("refresh must bypass cached status and scope invalid requests", calls.Load())
	}
	if _, err = db.Exec(`UPDATE projects SET github_repo='acme/preview' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/projects/1/task-pr-statuses/refresh", strings.NewReader(`{"repository":"acme/preview","number":123}`)))
	if w.Code != 200 || calls.Load() != 5 {
		t.Fatal("project-repository previews should refresh without a task", w.Code, calls.Load())
	}
	if err = settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/projects/1/task-pr-statuses/refresh", strings.NewReader(`{"repository":"acme/repo","number":7}`)))
	if w.Code != 200 || calls.Load() != 5 || !strings.Contains(w.Body.String(), "GitHub integration disabled") {
		t.Fatal("disabled integration must not fetch", w.Code, w.Body.String(), calls.Load())
	}
}

// Exercise the state left by an exhausted deadline without waiting thirty
// seconds: four workers start, then the request is canceled. Even when no
// lookup succeeds, the next poll must give the other four PRs the first slots.
func TestProjectPRStatusesRotateAfterEntirelyFailedPolls(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "fair-status.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects(id,name,repo_path) VALUES(1,'one','/one'); INSERT INTO tasks(id,project_id,title,source,position) VALUES(1,1,'one','manual',0)`); err != nil {
		t.Fatal(err)
	}
	for n := int64(1); n <= 8; n++ {
		if err = store.PutPRLink(t.Context(), db, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: n, Role: "implementation", Source: "explicit", Evidence: "test", ObservedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	clock := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	started := make(chan int64, 8)
	svc := github.NewPRStatusService(func(ctx context.Context, _ string, n int64) (github.PRStatus, error) {
		started <- n
		<-ctx.Done()
		return github.PRStatus{}, ctx.Err()
	}, func() time.Time { return clock })
	h := prContextHandlers{db: db, statuses: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/task-pr-statuses", h.projectStatuses)
	for round := 0; round < 3; round++ {
		ctx, cancel := context.WithCancel(t.Context())
		w := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			defer close(done)
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects/1/task-pr-statuses", nil).WithContext(ctx))
		}()
		seen := map[int64]bool{}
		for range 4 {
			select {
			case n := <-started:
				seen[n] = true
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("workers did not start", round, seen)
			}
		}
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("canceled poll did not finish")
		}
		offset := int64((round % 2) * 4)
		for n := int64(1); n <= 4; n++ {
			if !seen[offset+n] {
				t.Fatalf("round %d first workers=%v want PRs %d..%d", round, seen, offset+1, offset+4)
			}
		}
		if w.Code != http.StatusOK {
			t.Fatal(w.Code, w.Body.String())
		}
		var body struct{ Statuses []github.PRStatus }
		if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Statuses) != 8 {
			t.Fatal(body)
		}
		for i, s := range body.Statuses {
			if s.Number != int64(i+1) {
				t.Fatal("scheduling changed response order", body)
			}
		}
		select {
		case n := <-started:
			t.Fatal("canceled queued PR started a fetch", n)
		default:
		}
		clock = clock.Add(61 * time.Second)
	}
}

func TestSinglePRStatusBeforeTaskExists(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "single.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects(id,name,repo_path,github_repo) VALUES(1,'P','/p','acme/repo'),(2,'Unlinked','/u',NULL)`); err != nil {
		t.Fatal(err)
	}
	if err = settings.Set(db, settings.KeyGithubIntegration, "on"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	h := prContextHandlers{db: db, statuses: github.NewPRStatusService(func(_ context.Context, repo string, n int64) (github.PRStatus, error) {
		calls.Add(1)
		return github.UnknownPRStatus(repo, n, "test"), nil
	}, nil)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/pull-requests/{n}/status", h.pullRequestStatus)
	for _, test := range []struct {
		path string
		want int
	}{{"/api/projects/1/pull-requests/7/status", 200}, {"/api/projects/2/pull-requests/7/status", 409}, {"/api/projects/99/pull-requests/7/status", 404}, {"/api/projects/1/pull-requests/0/status", 400}, {"/api/projects/1/pull-requests/bad/status", 400}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
		if w.Code != test.want {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
	if err = settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects/1/pull-requests/8/status", nil))
	var status github.PRStatus
	if err = json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Error != "GitHub integration disabled" || status.Number != 8 {
		t.Fatal(status)
	}
	// No tasks or task-to-PR links were needed; disabling integration never reads GitHub.
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

func TestActiveProjectPRStatusesRefreshBypassesCache(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "active-refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO projects(id,name,repo_path) VALUES(1,'one','/one')`,
		`INSERT INTO tasks(id,project_id,title,source,status,position) VALUES(1,1,'active','manual','in_progress',0),(2,1,'done','manual','done',1)`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []struct{ task, number int64 }{{1, 7}, {2, 8}} {
		if err = store.PutPRLink(t.Context(), db, store.PRLink{TaskID: link.task, Repository: "acme/repo", Number: link.number, Role: "implementation", Source: "explicit", Evidence: "test", ObservedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	svc := github.NewPRStatusService(func(_ context.Context, repo string, n int64) (github.PRStatus, error) {
		calls.Add(1)
		result := github.UnknownPRStatus(repo, n, "")
		now := time.Now()
		result.FetchedAt = &now
		return result, nil
	}, nil)
	h := prContextHandlers{db: db, statuses: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/task-pr-statuses", h.projectStatuses)
	for i, path := range []string{"?active=1", "?active=1", "?active=1&refresh=1"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects/1/task-pr-statuses"+path, nil))
		var result struct{ Statuses []github.PRStatus }
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(result.Statuses) != 1 || result.Statuses[0].Number != 7 {
			t.Fatal("refresh must retain active-only scope", w.Code, result)
		}
		want := int32(1)
		if i == 2 {
			want = 2
		}
		if calls.Load() != want {
			t.Fatalf("%s: got %d GitHub calls, want %d", path, calls.Load(), want)
		}
	}
	if err = settings.Set(db, settings.KeyGithubIntegration, "off"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects/1/task-pr-statuses?active=1&refresh=1", nil))
	if calls.Load() != 2 || !strings.Contains(w.Body.String(), "GitHub integration disabled") {
		t.Fatal("forced overview refresh must respect integration settings", calls.Load(), w.Body.String())
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

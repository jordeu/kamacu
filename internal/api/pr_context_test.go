package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/notifications"
	"kamacu/internal/store"
)

func TestPRContextLookupAndCorrection(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "context.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Implement',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old := readPRFacts
	defer func() { readPRFacts = old }()
	readPRFacts = func(_ context.Context, repo string, n int64) notifications.PRFacts {
		yes := true
		return notifications.PRFacts{Repository: repo, Number: n, Available: true, State: "open", Viewer: "alice", ViewerEverApproved: &yes}
	}
	mux := http.NewServeMux()
	prContextRoutes(mux, db, nil)
	req := httptest.NewRequest("PUT", "/api/tasks/1/pr-links", strings.NewReader(`{"repository":"https://github.com/ACME/Repo/pull/7","role":"implementation","evidence":"Created PR"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"corrected":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/pr-context?repository=acme%2Frepo&number=7", nil))
	var out struct {
		GitHub  notifications.PRFacts `json:"github"`
		Related []store.RelatedTask   `json:"related_work"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(out.Related) != 1 || out.GitHub.ViewerEverApproved == nil || !*out.GitHub.ViewerEverApproved {
		t.Fatal(w.Body.String())
	}
	// An inferred update cannot undo a rejected association.
	req = httptest.NewRequest("PUT", "/api/tasks/1/pr-links", strings.NewReader(`{"repository":"acme/repo","number":7,"role":"incidental","evidence":"Wrong PR"}`))
	mux.ServeHTTP(httptest.NewRecorder(), req)
	if err = store.PutPRLink(context.Background(), db, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "review", Source: "terminal", Evidence: "Reviewing", ObservedAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/pr-context?repository=acme%2Frepo&number=7", nil))
	if !strings.Contains(w.Body.String(), `"related_work":[]`) {
		t.Fatal(w.Body.String())
	}
}

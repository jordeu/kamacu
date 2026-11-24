package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/store"
)

func TestTaskMoveExpectedStatus(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "moves.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects(id,name,repo_path) VALUES(1,'project','/tmp/project');
      INSERT INTO tasks(id,project_id,title,source,status,position) VALUES(1,1,'task','manual','in_progress',1)`); err != nil {
		t.Fatal(err)
	}
	h := &taskHandlers{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/move", h.move)
	for _, tc := range []struct {
		name, body, wantStatus string
		wantCode               int
	}{
		{"stale undo", `{"status":"todo","expected_status":"done"}`, "in_progress", 409},
		{"invalid expectation", `{"status":"done","expected_status":"invalid"}`, "in_progress", 400},
		{"matching move", `{"status":"in_review","expected_status":"in_progress"}`, "in_review", 200},
		{"concurrent status change", `{"status":"done","expected_status":"in_progress"}`, "in_review", 409},
		{"legacy move", `{"status":"done"}`, "done", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/tasks/1/move", strings.NewReader(tc.body)))
			if w.Code != tc.wantCode {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.wantCode, w.Body.String())
			}
			var status string
			if err = db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != tc.wantStatus {
				t.Fatalf("got status %s, want %s", status, tc.wantStatus)
			}
			if w.Code == 200 {
				var result Task
				if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Status != tc.wantStatus {
					t.Fatal(result, err)
				}
			}
		})
	}
}

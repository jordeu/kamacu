package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/store"
)

func TestInboxCatchUpRoutes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "catchup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO projects(id,name,repo_path) VALUES(1,'P','/p')`,
		`INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Task',0)`,
		`INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,summary,context,question,options,status,created_at,updated_at,heartbeat_at,queue_position) VALUES('old',1,'old','old','old','S','C','Q','[]','answered',1,1,1,1),('selected',1,'live','live','current','S','C','Q','[]','pending',2,2,2,2)`,
	} {
		if _, err = db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	svc := inbox.New(db, func(string) inbox.Session { return inbox.Session{TaskID: 1, Live: true} })
	defer svc.Close()
	mux := http.NewServeMux()
	InboxRoutes(mux, svc, "")
	get := func(path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatal(path, w.Code, w.Body.String())
		}
		return w
	}
	var queue []inbox.Question
	if err = json.Unmarshal(get("/api/inbox/catch-up", 200).Body.Bytes(), &queue); err != nil || len(queue) != 1 || queue[0].ID != "selected" {
		t.Fatal(queue, err)
	}
	var history inbox.Page
	if err = json.Unmarshal(get("/api/inbox/selected/history", 200).Body.Bytes(), &history); err != nil || len(history.Items) != 1 || history.Items[0].ID != "old" {
		t.Fatal(history, err)
	}
	get("/api/inbox/selected/history?offset=-1", 400)
	get("/api/inbox/selected/history?offset=no", 400)
	get("/api/inbox/missing/history", 404)
	get("/api/inbox/selected", 200)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/inbox/selected/dismiss", strings.NewReader(`{"revision":1}`)))
	var result inbox.Result
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Outcome != "dismissed" || result.Question.Status != "dismissed" {
		t.Fatal("dismissal contract", w.Code, result, err)
	}
}

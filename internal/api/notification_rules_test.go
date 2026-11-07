package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/notifications"
	"kamacu/internal/store"
)

func TestNotificationRuleLifecycleRoutes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{} })
	defer box.Close()
	s := notifications.New(db, box, nil, nil)
	mux := http.NewServeMux()
	NotificationRoutes(mux, s)
	SettingsRoutes(mux, db)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return w
	}
	for _, body := range []string{`null`, `{} {}`, `{"unknown":true}`} {
		if w := call("POST", "/api/notifications/rules/drafts", body); w.Code != 400 {
			t.Fatal("invalid draft accepted", w.Code, body)
		}
	}
	w := call("POST", "/api/notifications/rules/drafts", `{"base_revision":0,"rules":[]}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var draft notifications.RuleSet
	if err = json.Unmarshal(w.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if w = call("POST", "/api/notifications/rules/"+draft.ID+"/activate", ""); w.Code != 409 {
		t.Fatal("draft activated without compilation")
	}
	if w = call("POST", "/api/notifications/rules/"+draft.ID+"/compile", ""); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w = call("GET", "/api/notifications/rules", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var sets []notifications.RuleSet
	json.Unmarshal(w.Body.Bytes(), &sets)
	if len(sets) != 1 || sets[0].Status != "ready" {
		t.Fatal(sets)
	}
	if w = call("POST", "/api/notifications/rules/"+draft.ID+"/activate", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("POST", "/api/notifications/rules/drafts", `{"base_revision":0,"rules":[]}`); w.Code != 409 {
		t.Fatal("stale editor was not rejected", w.Code)
	}
	if w = call("GET", "/api/notifications/rules/problems", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/notifications/rules/history/"+draft.ID, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("POST", "/api/notifications/rules/improvements", `{"kind":"unclear_rule","instructions":""}`); w.Code != 400 {
		t.Fatal("empty feedback accepted", w.Code)
	}
	for _, key := range []string{"notification_system_prompt", "notification_policy"} {
		if w = call("PUT", "/api/settings/"+key, `{"value":"obsolete"}`); w.Code != 404 {
			t.Fatal("old configuration remains writable", key, w.Code)
		}
	}
	if w = call("POST", "/api/notifications/rules/previews", `{"rule_set_id":"`+draft.ID+`"}`); w.Code != 400 {
		t.Fatal("empty preview accepted")
	}
	source, err := s.AddSource(context.Background(), "github", "GitHub", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO notification_events(source_id,external_id,version,payload,created_at) VALUES(?,'event','v1','{}',0)`, source.ID); err != nil {
		t.Fatal(err)
	}
	if w = call("GET", "/api/notifications?status=held", ""); w.Code != 400 {
		t.Fatal("removed category is still available", w.Code)
	}
	if w = call("POST", "/api/notifications/retry-held", ""); w.Code != 404 && w.Code != 405 {
		t.Fatal("removed retry route is still available", w.Code)
	}
	var page notifications.RawPage
	w = call("GET", "/api/notifications?status=pending", "")
	json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || len(page.Items) != 1 {
		t.Fatal(page)
	}
}

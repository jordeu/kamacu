package api

import (
	"bytes"
	"context"
	"encoding/json"
	"kamacu/internal/inbox"
	"kamacu/internal/notifications"
	"kamacu/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationAgentSettingsAndRules(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "agent.db"))
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
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return w
	}
	w := call("GET", "/api/notifications/config", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var c notifications.Config
	json.Unmarshal(w.Body.Bytes(), &c)
	if c.ProfileID != nil {
		t.Fatal("notification model silently inherited monitoring")
	}
	if _, err = db.Exec(`INSERT INTO monitor_profiles(id,name,backend,model,endpoint) VALUES(1,'Monitor','openai_compatible','small','http://localhost'),(2,'Notifications','openai_compatible','strong','http://localhost'); UPDATE monitor_settings SET default_profile_id=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	id := int64(2)
	c.ProfileID = &id
	w = call("PUT", "/api/notifications/config", string(mustJSONTest(c)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var monitoring int64
	db.QueryRow(`SELECT default_profile_id FROM monitor_settings WHERE id=1`).Scan(&monitoring)
	if monitoring != 1 {
		t.Fatal("changed monitoring selection")
	}
	set, err := s.SaveRules(context.Background(), 1, []notifications.NotificationRule{{ID: "R003", Text: "Group Renovate reviews and propose the recurring task", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if set.Status != "active" || set.Rules[0].ID != "R003" {
		t.Fatal(set)
	}
	for _, path := range []string{"/api/notifications/rules/" + set.ID + "/compile", "/api/notifications/rules/" + set.ID + "/activate", "/api/notifications/rules/drafts"} {
		if w = call("POST", path, `{}`); w.Code < 400 {
			t.Fatal("obsolete compiler endpoint is available", path)
		}
	}
	if w = call("PUT", "/api/notifications/rules", `{"base_revision":1,"rules":[]}`); w.Code != 409 {
		t.Fatal("stale edit accepted", w.Code)
	}
	if w = call("PUT", "/api/notifications/rules", `{"base_revision":2,"rules":[{"id":"X","text":"x","enabled":true,"clauses":[]}]}`); w.Code != 400 {
		t.Fatal("compiler clauses accepted", w.Code)
	}
	rules := []notifications.NotificationRule{}
	for i := 0; i < 20; i++ {
		rules = append(rules, notifications.NotificationRule{Text: strings.Repeat("a", 8000), Enabled: true})
	}
	w = call("PUT", "/api/notifications/rules", string(mustJSONTest(map[string]any{"base_revision": set.Revision, "rules": rules})))
	if w.Code != 200 {
		t.Fatal("valid plain rule list was rejected by the HTTP body limit", w.Code, w.Body.String())
	}
	if w = call("POST", "/api/notifications/config/test", `{}`); w.Code != 502 {
		t.Fatal("missing model backend reported success", w.Code)
	}
}
func mustJSONTest(v any) []byte { b, _ := json.Marshal(v); return b }

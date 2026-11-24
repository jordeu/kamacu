package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/inboxagent"
	"kamacu/internal/store"
)

func TestInboxAgentConfigurationRoutes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &inboxagent.Service{DB: db}
	mux := http.NewServeMux()
	InboxAgentRoutes(mux, s)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(raw)))
		return w
	}
	w := call("GET", "/api/inbox-agent/config", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var c inboxagent.Config
	if err = json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.ProfileID != nil || len(c.Rules) != 0 {
		t.Fatal("new installs must not answer automatically", c)
	}
	c.Rules = []inboxagent.Rule{{Text: "Only start a rereview in acme/repo when we previously approved that PR.", Enabled: true}}
	w = call("PUT", "/api/inbox-agent/config", c)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("PUT", "/api/inbox-agent/config", c); w.Code != 409 {
		t.Fatal("stale settings overwrote newer rules", w.Code)
	}
	if w = call("PUT", "/api/inbox-agent/config", map[string]any{"interval_seconds": 300}); w.Code != 400 {
		t.Fatal("unexpected periodic setting", w.Code)
	}
	for _, method := range []string{"GET", "POST"} {
		if w = call(method, "/api/inbox-agent/runs", nil); w.Code != 404 {
			t.Fatal("manual processing is exposed", w.Code)
		}
	}
	if w = call("POST", "/api/inbox-agent/config/test", nil); w.Code != 502 {
		t.Fatal("missing model should fail the connection test", w.Code)
	}
	saved, err := s.Config(context.Background())
	if err != nil || saved.Rules[0].ID == "" || saved.Revision != c.Revision+1 {
		t.Fatal(saved, err)
	}
}

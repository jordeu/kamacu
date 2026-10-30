package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/monitor"
	"kamacu/internal/observation"
	"kamacu/internal/store"
)

type sampleMonitor struct{}

func (sampleMonitor) Classify(_ context.Context, _ monitor.Profile, s observation.Snapshot, _ ...monitor.TaskContext) (monitor.Decision, error) {
	return monitor.Decision{State: "waiting", Kind: "question", Evidence: s.Screen, Question: "Would you like me to continue?", Summary: "Continue?", InputMode: "text"}, nil
}
func TestMonitorConfiguration(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	MonitorRoutes(mux, db, sampleMonitor{})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	status, p := doJSON(t, "POST", srv.URL+"/api/monitor/profiles", map[string]any{"name": "Local", "backend": "openai_compatible", "endpoint": "http://localhost:1234/v1", "model": "local", "credential_env": "TEST_KEY"})
	if status != 200 {
		t.Fatal(status, p)
	}
	id := p["id"]
	status, out := doJSON(t, "PUT", srv.URL+"/api/monitor/selection", map[string]any{"profile_id": id})
	if status != 204 {
		t.Fatal(status, out)
	}
	status, out = doJSON(t, "DELETE", srv.URL+"/api/monitor/profiles/1", nil)
	if status != 409 {
		t.Fatal("referenced profile deletion", status, out)
	}
	status, out = doJSON(t, "POST", srv.URL+"/api/monitor/test", p)
	if status != 200 || out["ok"] != true {
		t.Fatal(status, out)
	}
	status, out = doJSON(t, "PUT", srv.URL+"/api/monitor/selection", map[string]any{"workspace_id": 999, "profile_id": id})
	if status != 404 {
		t.Fatal("unknown workspace", status, out)
	}
	status, out = doJSON(t, "PUT", srv.URL+"/api/monitor/selection", map[string]any{"profile_id": nil})
	if status != 400 {
		t.Fatal("missing required default", status, out)
	}
}

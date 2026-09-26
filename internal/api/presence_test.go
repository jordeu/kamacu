package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kamacu/internal/presence"
)

// newPresenceServer wires the presence routes over a fresh Tracker with the
// standard test token, plus a plain (no token) client view of the routes to
// prove the GET gate from the browser side too.
func newPresenceServer(t *testing.T) (*httptest.Server, *presence.Tracker) {
	t.Helper()
	tr := presence.NewTracker()
	mux := http.NewServeMux()
	PresenceRoutes(mux, tr, testHookToken)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, tr
}

func postHeartbeat(t *testing.T, url string, body map[string]any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST heartbeat: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestPresenceHeartbeatRoundTrip: a valid beat returns 204 and lands in the
// tracker; the snapshot (with the token) reflects it.
func TestPresenceHeartbeatRoundTrip(t *testing.T) {
	srv, _ := newPresenceServer(t)

	if code := postHeartbeat(t, srv.URL+"/api/presence/heartbeat", map[string]any{
		"client_id":  "tab-1",
		"route":      "/projects/3/tasks/7",
		"project_id": 3,
		"task_id":    7,
		"session_id": "sess-a",
		"visible":    true,
	}); code != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d, want 204", code)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/presence", nil)
	req.Header.Set("X-Kamacu-Token", testHookToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/presence: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200", resp.StatusCode)
	}
	var snap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snap["active"] != true {
		t.Errorf("snapshot active = %v, want true", snap["active"])
	}
	viewing, _ := snap["viewing"].(map[string]any)
	if viewing == nil || viewing["route"] != "/projects/3/tasks/7" {
		t.Errorf("snapshot viewing = %v, want route /projects/3/tasks/7", snap["viewing"])
	}
}

// TestPresenceHeartbeatValidation: a beat without a client_id is a 400 —
// the tab identity is the upsert key and must never be empty.
func TestPresenceHeartbeatValidation(t *testing.T) {
	srv, _ := newPresenceServer(t)

	if code := postHeartbeat(t, srv.URL+"/api/presence/heartbeat", map[string]any{
		"route":   "/projects/3",
		"visible": true,
	}); code != http.StatusBadRequest {
		t.Fatalf("missing client_id status = %d, want 400", code)
	}

	if code := postHeartbeat(t, srv.URL+"/api/presence/heartbeat", map[string]any{
		"client_id": "tab-1",
	}); code != http.StatusBadRequest {
		t.Fatalf("missing visible status = %d, want 400 (explicit, not defaulted)", code)
	}
}

// TestPresenceSnapshotTokenGate: GET /api/presence is token-gated like the
// hooks receiver — missing or wrong token is a 401, never an open read.
func TestPresenceSnapshotTokenGate(t *testing.T) {
	srv, _ := newPresenceServer(t)
	_, _ = srv, new(presence.Tracker)

	for name, token := range map[string]string{
		"missing": "",
		"wrong":   "not-the-token",
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/presence", nil)
		if token != "" {
			req.Header.Set("X-Kamacu-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: GET /api/presence: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s token: status = %d, want 401", name, resp.StatusCode)
		}
	}
}

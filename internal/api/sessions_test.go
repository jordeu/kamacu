package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kangent/internal/session"
)

// newSessionServer starts an httptest server with only the session routes
// registered, backed by a real Manager (bash spawns are cheap). All spawned
// sessions are stopped on cleanup.
func newSessionServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager()
	mux := http.NewServeMux()
	SessionRoutes(mux, mgr)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		for _, info := range mgr.List() {
			if s, ok := mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
	})
	return srv, mgr
}

// exitSession drives a session to exit and waits for it.
func exitSession(t *testing.T, sess *session.Session) {
	t.Helper()
	if err := sess.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("write exit: %v", err)
	}
	select {
	case <-sess.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("session did not exit within 10s")
	}
}

func TestSessionListEmpty(t *testing.T) {
	srv, _ := newSessionServer(t)

	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	// Empty list must be JSON [] — never null.
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Errorf("empty list body = %q, want %q", got, "[]")
	}
}

func TestSessionCreateAndList(t *testing.T) {
	srv, _ := newSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201; body=%v", status, body)
	}
	if id, ok := body["id"].(string); !ok || id == "" {
		t.Errorf("id = %v, want non-empty string", body["id"])
	}
	if body["label"] != "bash #1" {
		t.Errorf("label = %q, want %q", body["label"], "bash #1")
	}
	if body["status"] != "running" {
		t.Errorf("status = %q, want %q", body["status"], "running")
	}
	if created, ok := body["createdAt"].(string); !ok || created == "" {
		t.Errorf("createdAt = %v, want non-empty string", body["createdAt"])
	}

	listStatus, list := doJSONList(t, srv.URL+"/api/sessions")
	if listStatus != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", listStatus)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0]["id"] != body["id"] {
		t.Errorf("listed id = %v, want %v", list[0]["id"], body["id"])
	}
}

func TestSessionStop(t *testing.T) {
	srv, mgr := newSessionServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d; body=%v", status, body)
	}
	id := body["id"].(string)

	status, _ = doJSON(t, "POST", srv.URL+"/api/sessions/"+id+"/stop", nil)
	if status != http.StatusAccepted {
		t.Fatalf("stop: status = %d, want 202", status)
	}

	// Stop is async (202) — wait for the session to actually exit.
	sess, ok := mgr.Get(id)
	if !ok {
		t.Fatal("session vanished from manager")
	}
	select {
	case <-sess.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("session did not exit after stop")
	}

	// Idempotent: stopping an already-exited session is still 202.
	status, _ = doJSON(t, "POST", srv.URL+"/api/sessions/"+id+"/stop", nil)
	if status != http.StatusAccepted {
		t.Fatalf("repeated stop: status = %d, want 202", status)
	}

	// Unknown id → 404 with the {"error"} contract.
	status, body = doJSON(t, "POST", srv.URL+"/api/sessions/nonexistent/stop", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown stop: status = %d, want 404", status)
	}
	if body["error"] != "session not found" {
		t.Errorf("error = %q, want %q", body["error"], "session not found")
	}
}

func TestSessionDelete(t *testing.T) {
	srv, mgr := newSessionServer(t)

	// Running session → 409.
	status, body := doJSON(t, "POST", srv.URL+"/api/sessions", nil)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d; body=%v", status, body)
	}
	runningID := body["id"].(string)
	status, body = doJSON(t, "DELETE", srv.URL+"/api/sessions/"+runningID, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete running: status = %d, want 409; body=%v", status, body)
	}
	if body["error"] != "session is still running. Stop it first." {
		t.Errorf("error = %q, want %q", body["error"], "session is still running. Stop it first.")
	}

	// Exited session → 204, and it disappears from the list.
	sess, ok := mgr.Get(runningID)
	if !ok {
		t.Fatal("session vanished from manager")
	}
	exitSession(t, sess)
	status, _ = doJSON(t, "DELETE", srv.URL+"/api/sessions/"+runningID, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete exited: status = %d, want 204", status)
	}
	listStatus, list := doJSONList(t, srv.URL+"/api/sessions")
	if listStatus != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", listStatus)
	}
	for _, item := range list {
		if item["id"] == runningID {
			t.Errorf("deleted session %s still listed", runningID)
		}
	}

	// Unknown id → 404.
	status, body = doJSON(t, "DELETE", srv.URL+"/api/sessions/nonexistent", nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete unknown: status = %d, want 404; body=%v", status, body)
	}
	if body["error"] != "session not found" {
		t.Errorf("error = %q, want %q", body["error"], "session not found")
	}
}

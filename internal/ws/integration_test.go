// Package ws_test exercises the full terminal engine end-to-end through the
// REAL route table — the same wiring shape as cmd/kangent/main.go (REST via
// api.SessionRoutes + the WS endpoint). If the route registration drifts from
// production, this test catches it.
package ws_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kangent/internal/api"
	"kangent/internal/session"
	"kangent/internal/store"
	"kangent/internal/tmux"
	"kangent/internal/ws"
)

// newTestServer builds the mux exactly as cmd/kangent/main.go does and serves
// it over httptest. Cleanup stops every session so no bash outlives the run.
func newTestServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mgr := session.NewManager()
	mux := http.NewServeMux()
	api.SessionRoutes(mux, mgr, db, tmux.Client{})
	mux.Handle("GET /api/sessions/{id}/ws", ws.NewHandler(mgr, nil))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		for _, info := range mgr.List() {
			if sess, ok := mgr.Get(info.ID); ok {
				sess.Stop()
			}
		}
	})
	return srv, mgr
}

func integrationCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// listSessions GETs /api/sessions and decodes the JSON array.
func listSessions(t *testing.T, srv *httptest.Server) []session.Info {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/sessions status = %d, want 200", resp.StatusCode)
	}
	var infos []session.Info
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode session list: %v", err)
	}
	return infos
}

// collectUntil reads '0' frames, accumulating payloads, until the output
// contains substr. Markers use the shell quote-splitting trick (mar''ker) so
// the terminal echo of the typed command never matches — only real output.
func collectUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, substr string) string {
	t.Helper()
	var out strings.Builder
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read while waiting for %q (have %q): %v", substr, out.String(), err)
		}
		if typ != websocket.MessageBinary || len(data) == 0 {
			continue
		}
		if data[0] != ws.FrameData {
			t.Fatalf("unexpected frame type %q while waiting for %q", data[0], substr)
		}
		out.Write(data[1:])
		if strings.Contains(out.String(), substr) {
			return out.String()
		}
	}
}

// TestIntegration drives the full lifecycle through the production route
// shape: spawn (201) → attach → type → output → detach → still running →
// reattach with replay → stop (202) → exited → delete (204) → empty list.
func TestIntegration(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := integrationCtx(t)

	// Spawn: POST /api/sessions → 201 + id.
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/sessions: %v", err)
	}
	var created session.Info
	if decodeErr := json.NewDecoder(resp.Body).Decode(&created); decodeErr != nil {
		t.Fatalf("decode created session: %v", decodeErr)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/sessions status = %d, want 201", resp.StatusCode)
	}
	if created.ID == "" {
		t.Fatal("created session has empty id")
	}

	wsURL := srv.URL + "/api/sessions/" + created.ID + "/ws"

	// Attach and type: input as a '0' frame, output back as '0' frames.
	conn1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	frame := append([]byte{ws.FrameData}, "echo integration-mar''ker\n"...)
	if err := conn1.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write input frame: %v", err)
	}
	collectUntil(t, ctx, conn1, "integration-marker")

	// Detach: close the WS. The session must keep running (TERM-05).
	conn1.Close(websocket.StatusNormalClosure, "")
	infos := listSessions(t, srv)
	if len(infos) != 1 || infos[0].ID != created.ID {
		t.Fatalf("list after detach = %+v, want the one spawned session", infos)
	}
	if infos[0].Status != session.StatusRunning {
		t.Fatalf("status after detach = %q, want running (TERM-05)", infos[0].Status)
	}

	// Reattach: the FIRST '0' frame is the ring replay with prior output.
	conn2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("re-dial ws: %v", err)
	}
	typ, data, err := conn2.Read(ctx)
	if err != nil {
		t.Fatalf("read first frame on reattach: %v", err)
	}
	if typ != websocket.MessageBinary || len(data) == 0 || data[0] != ws.FrameData {
		t.Fatalf("first frame on reattach: type=%v prefix=%q, want binary '0' frame", typ, data)
	}
	if !strings.Contains(string(data[1:]), "integration-marker") {
		t.Fatalf("replay frame does not contain prior output; got %q", data[1:])
	}
	conn2.Close(websocket.StatusNormalClosure, "")

	// Stop: POST /api/sessions/{id}/stop → 202 (async, D-14 grace window).
	resp, err = http.Post(srv.URL+"/api/sessions/"+created.ID+"/stop", "application/json", nil)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST stop status = %d, want 202", resp.StatusCode)
	}

	// Poll the list (bounded) until the session reports exited.
	deadline := time.Now().Add(10 * time.Second)
	for {
		infos = listSessions(t, srv)
		if len(infos) == 1 && infos[0].Status == session.StatusExited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session never reached exited after stop; list = %+v", infos)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Delete the exited session → 204.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL+"/api/sessions/"+created.ID, nil)
	if err != nil {
		t.Fatalf("build DELETE request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	// The list is empty again (JSON [] — decoded as a zero-length slice).
	if infos = listSessions(t, srv); len(infos) != 0 {
		t.Fatalf("list after delete = %+v, want empty", infos)
	}
}

// TestIntegrationEvilOriginRejected asserts the CSWSH rejection end-to-end
// through the same mux: a cross-origin browser upgrade never reaches 101.
// (Host-header rejection lives in cmd/kangent's hostCheck middleware, which
// is unexported — covered by its own table tests in main_test.go.)
func TestIntegrationEvilOriginRejected(t *testing.T) {
	srv, mgr := newTestServer(t)
	ctx := integrationCtx(t)

	sess, err := mgr.Spawn(session.SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	url := fmt.Sprintf("%s/api/sessions/%s/ws", srv.URL, sess.Info().ID)
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}},
	})
	if err == nil {
		conn.Close(websocket.StatusNormalClosure, "")
		t.Fatal("upgrade succeeded with Origin http://evil.example")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("handshake response = %+v, want HTTP 403", resp)
	}
}

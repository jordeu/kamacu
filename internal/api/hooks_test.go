package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/session"
)

// testHookToken is the per-instance token the hook test server is configured
// with. Production generates this with crypto/rand at startup.
const testHookToken = "test-hook-token-0123456789abcdef"

// writeFakeClaude writes an executable stub standing in for the claude binary
// (spawned via AgentConfig.ClaudeBin — agent tests never touch the real
// binary). It mirrors the verified v2.1.170 behaviors the engine relies on:
// bracketed-paste enable at startup, TERM trap -> exit 143. Each input line is
// answered with a bare BEL followed by a "bel-MARK:<line>" marker in ONE write,
// so once the marker is visible in Snapshot() the BEL has been processed.
func writeFakeClaude(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-claude")
	script := `#!/usr/bin/env bash
printf '\x1b[?2004h'
echo "fake claude ready"
trap 'exit 143' TERM
while IFS= read -r line; do
  printf '\abel-MARK:%s\n' "$line"
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude stub: %v", err)
	}
	return path
}

// newHookServer wires the hook receiver over a fresh Manager whose agent
// config points at the fake-claude stub.
func newHookServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL:   "http://127.0.0.1:7333",
		Token:     testHookToken,
		ClaudeBin: writeFakeClaude(t),
	})
	mux := http.NewServeMux()
	HookRoutes(mux, mgr, testHookToken)
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

// spawnHookAgent spawns a stub agent session for hook tests.
func spawnHookAgent(t *testing.T, mgr *session.Manager) *session.Session {
	t.Helper()
	sess, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, Cwd: t.TempDir(), TaskID: 1})
	if err != nil {
		t.Fatalf("spawn agent: %v", err)
	}
	return sess
}

// postHook POSTs body to the hook receiver with the given token header
// (omitted entirely when token is empty) and returns the status code.
func postHook(t *testing.T, url, token, body string) int {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Kangent-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// hookURL builds the receiver URL for a session id.
func hookURL(srv *httptest.Server, id string) string {
	return srv.URL + "/api/hooks/sessions/" + id
}

// TestHookTokenGate: the receiver answers 401 to a missing or wrong
// X-Kangent-Token and never mutates session state (research gap #2 — any
// webpage can fire a no-CORS POST at localhost; the token is the gate).
func TestHookTokenGate(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)
	body := `{"hook_event_name":"Notification","session_id":"u","notification_type":"permission_prompt"}`

	cases := []struct {
		name  string
		token string
	}{
		{"missing token", ""},
		{"wrong token", "wrong-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := postHook(t, hookURL(srv, sess.Info().ID), tc.token, body); got != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", got)
			}
			if got := sess.Info().AgentStatus; got == "waiting" {
				t.Errorf("rejected hook still flipped status to waiting")
			}
		})
	}
}

// TestHookEmptyConfiguredToken: a receiver configured with an empty token must
// reject EVERYTHING (defense: never run open), including requests that also
// send no token.
func TestHookEmptyConfiguredToken(t *testing.T) {
	mgr := session.NewManager()
	mux := http.NewServeMux()
	HookRoutes(mux, mgr, "")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if got := postHook(t, hookURL(srv, "any"), "", `{"hook_event_name":"Stop"}`); got != http.StatusUnauthorized {
		t.Errorf("empty configured token: status = %d, want 401", got)
	}
}

// TestHookNotificationSetsWaiting: a Notification hook POST with the correct
// token flips a running agent to waiting (D-47).
func TestHookNotificationSetsWaiting(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)

	body := `{"hook_event_name":"Notification","session_id":"u","notification_type":"permission_prompt"}`
	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, body); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Errorf("AgentStatus = %q, want %q", got, "waiting")
	}
}

// TestHookStopSetsIdle: a Stop hook POST flips the agent to idle (D-46 — turn
// end is idle, never a separate done state).
func TestHookStopSetsIdle(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)

	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, `{"hook_event_name":"Stop"}`); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "idle" {
		t.Errorf("AgentStatus = %q, want %q", got, "idle")
	}
}

// TestHookSessionStartMarksHooksAlive: SessionStart returns 204 and disables
// the BEL fallback. Observable contract (Pitfall 2): a bare BEL in PTY output
// sets waiting ONLY while hooks are not confirmed alive.
func TestHookSessionStartMarksHooksAlive(t *testing.T) {
	srv, mgr := newHookServer(t)

	// driveBEL makes the stub emit a bare BEL + marker and waits until the
	// marker is in the ring — the BEL effect is applied under the same lock,
	// so the status read after this is deterministic.
	driveBEL := func(t *testing.T, sess *session.Session, tag string) {
		t.Helper()
		if err := sess.WriteInput([]byte(tag + "\n")); err != nil {
			t.Fatalf("write input: %v", err)
		}
		want := []byte("bel-MARK:" + tag)
		deadline := time.Now().Add(10 * time.Second)
		for !bytes.Contains(sess.Snapshot(), want) {
			if time.Now().After(deadline) {
				t.Fatalf("stub never echoed %q:\n%s", want, sess.Snapshot())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Control: hooks-dead session — BEL sets waiting (fallback mode works in
	// this harness).
	dead := spawnHookAgent(t, mgr)
	driveBEL(t, dead, "ping-dead")
	if got := dead.Info().AgentStatus; got != "waiting" {
		t.Fatalf("hooks-dead BEL: AgentStatus = %q, want %q (fallback broken — alive assertion below would be vacuous)", got, "waiting")
	}

	// SessionStart-confirmed session — BEL is ignored.
	alive := spawnHookAgent(t, mgr)
	if got := postHook(t, hookURL(srv, alive.Info().ID), testHookToken, `{"hook_event_name":"SessionStart","source":"startup"}`); got != http.StatusNoContent {
		t.Fatalf("SessionStart status = %d, want 204", got)
	}
	driveBEL(t, alive, "ping-alive")
	if got := alive.Info().AgentStatus; got == "waiting" {
		t.Errorf("hooks-alive BEL flipped status to waiting; BEL must be ignored once SessionStart confirmed the pipeline")
	}
}

// TestHookUnknownSession: an unknown session id is a 404.
func TestHookUnknownSession(t *testing.T) {
	srv, _ := newHookServer(t)
	if got := postHook(t, hookURL(srv, "nonexistent"), testHookToken, `{"hook_event_name":"Stop"}`); got != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

// TestHookExitedSessionIgnored: hooks racing a session's exit are ignored with
// 200 — late curl deliveries are expected, never errors.
func TestHookExitedSessionIgnored(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)
	sess.Stop()
	select {
	case <-sess.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("agent session did not exit after Stop")
	}

	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`); got != http.StatusOK {
		t.Errorf("status = %d, want 200 (ignored)", got)
	}
	if got := sess.Info().AgentStatus; got != "exited" {
		t.Errorf("AgentStatus = %q, want %q", got, "exited")
	}
}

// TestHookMalformedBody: undecodable JSON is a 400.
func TestHookMalformedBody(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)

	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, `{not json`); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

// TestHookUnknownEventTolerated: future hook_event_name values are a no-op 204
// — the receiver switches on hook_event_name only and tolerates everything
// else (forward compatibility with new claude releases).
func TestHookUnknownEventTolerated(t *testing.T) {
	srv, mgr := newHookServer(t)
	sess := spawnHookAgent(t, mgr)

	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, `{"hook_event_name":"SomeFutureEvent","extra_field":42}`); got != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (tolerated no-op)", got)
	}
	if got := sess.Info().AgentStatus; got == "waiting" {
		t.Errorf("unknown event flipped status to waiting")
	}
}

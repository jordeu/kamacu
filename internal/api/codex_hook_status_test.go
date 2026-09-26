package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/session"
)

// This file is the cross-cutting integration proof for codex engine status:
// the UNCHANGED, engine-agnostic hook receiver (hooks.go) drives a codex-
// engine session through working/waiting/idle — exactly the loop the on-disk
// codex profile (internal/codex/kamacu.config.toml) drives in production
// (PermissionRequest -> waiting, Stop -> idle, SessionStart -> hooks-alive
// canary + codex session-id capture). hooks.go is not imported here; only
// SpawnOpts (AgentEngine="codex") and the public Session hook methods are
// exercised, over HTTP exactly as production does.

// writeFakeCodex writes an executable stub standing in for the codex binary.
// codex spawns via the custom command-render arm (AgentArgs) with `-p kamacu`
// prepended by the session layer; the stub ignores argv and mirrors the
// writeFakeOpencode BEL-on-input behavior so the hooksAlive canary can be
// proven for a codex-engine session too.
func writeFakeCodex(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-codex")
	script := `#!/usr/bin/env bash
echo "fake codex ready"
trap 'exit 143' TERM
while IFS= read -r line; do
  printf '\abel-MARK:%s\n' "$line"
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex stub: %v", err)
	}
	return path
}

// newCodexHookServer wires the UNCHANGED hook receiver over a fresh Manager
// whose agent config carries the hook token + base URL a codex process
// receives via env (D014). No ClaudeBin: codex spawns via AgentArgs.
func newCodexHookServer(t *testing.T) (*httptest.Server, *session.Manager) {
	t.Helper()
	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{
		BaseURL: "http://127.0.0.1:7333",
		Token:   testHookToken,
	})
	mux := http.NewServeMux()
	HookRoutes(mux, mgr, testHookToken) // UNCHANGED, engine-agnostic receiver
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

// spawnCodexAgent spawns a codex-engine agent session driven by the
// fake-codex stub. This is the only place the test touches the engine
// selection: everywhere else it talks to the engine-agnostic hook receiver.
func spawnCodexAgent(t *testing.T, mgr *session.Manager) *session.Session {
	t.Helper()
	sess, err := mgr.Spawn(session.SpawnOpts{
		Kind:        session.KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      1,
		AgentEngine: "codex",
		AgentArgs:   []string{writeFakeCodex(t)},
	})
	if err != nil {
		t.Fatalf("spawn codex agent: %v", err)
	}
	return sess
}

// TestCodexHookPermissionRequestSetsWaiting proves the codex waiting signal:
// codex fires a dedicated PermissionRequest event (verified against codex
// 0.157.0) where claude fires Notification(permission_prompt). The receiver
// maps it to the same SetWaiting transition.
func TestCodexHookPermissionRequestSetsWaiting(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)

	body := `{"hook_event_name":"PermissionRequest","session_id":"0d1a2b3c-1111-2222-3333-444455556666","tool_name":"Bash"}`
	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, body); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Errorf("AgentStatus = %q, want %q", got, "waiting")
	}
	// xterm sends these without user interaction after focus changes and
	// terminal queries. They must not acknowledge the permission request.
	for _, report := range []string{"\x1b[I", "\x1b[24;80R", "\x1b[?1;2c", "\x1b[O"} {
		if err := sess.WriteInput([]byte(report)); err != nil {
			t.Fatal(err)
		}
		if got := sess.Info().AgentStatus; got != "waiting" {
			t.Fatalf("after terminal report %q: status = %q, want waiting", report, got)
		}
	}
	sess.ClearWaitingOnAttach()
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Fatalf("after reconnect: status = %q, want waiting", got)
	}
	if err := sess.WriteInput([]byte("y")); err != nil {
		t.Fatal(err)
	}
	if got := sess.Info().AgentStatus; got != "working" {
		t.Fatalf("after answer: status = %q, want working", got)
	}
}

// TestCodexHookSessionStartCapturesCodexSessionID proves the restart-resume
// capture: codex's hook payloads carry codex's OWN session uuid (payload
// session_id — verified shape: {"session_id":"<uuid>","hook_event_name":…}).
// The receiver records it onto the session, where the api-layer capture
// poller persists it to tasks.codex_session_id.
func TestCodexHookSessionStartCapturesCodexSessionID(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)

	const codexID = "0d1a2b3c-aaaa-bbbb-cccc-ddddeeeeffff"
	body := `{"hook_event_name":"SessionStart","source":"startup","session_id":"` + codexID + `"}`
	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, body); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.CodexSessionID(); got != codexID {
		t.Errorf("CodexSessionID = %q, want %q", got, codexID)
	}
}

// TestCodexHookSessionIDFirstCaptureWins proves the first-wins posture: a
// later hook carrying a DIFFERENT session_id (e.g. the user picked another
// conversation via codex-side /resume) must not retarget kamacu's persisted
// resume key.
func TestCodexHookSessionIDFirstCaptureWins(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)

	const first = "0d1a2b3c-1111-1111-1111-111111111111"
	const second = "0d1a2b3c-2222-2222-2222-222222222222"
	postHook(t, hookURL(srv, sess.Info().ID), testHookToken,
		`{"hook_event_name":"Stop","session_id":"`+first+`"}`)
	postHook(t, hookURL(srv, sess.Info().ID), testHookToken,
		`{"hook_event_name":"SessionStart","source":"resume","session_id":"`+second+`"}`)
	if got := sess.CodexSessionID(); got != first {
		t.Errorf("CodexSessionID = %q, want first capture %q", got, first)
	}
}

// TestCodexHookSessionIDCaptureIsEngineGated proves the capture only records
// on codex-engine sessions: claude/opencode hook payloads also carry their own
// session_id (claude's Notification, opencode's plugin events), and recording
// those into the codex field would poison the codex resume key.
func TestCodexHookSessionIDCaptureIsEngineGated(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	// Claude-engine agent (spawnHookAgent uses the claude path with no engine).
	claude, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, Cwd: t.TempDir(), TaskID: 2})
	if err != nil {
		t.Fatalf("spawn claude agent: %v", err)
	}

	postHook(t, hookURL(srv, claude.Info().ID), testHookToken,
		`{"hook_event_name":"Notification","notification_type":"permission_prompt","session_id":"claude-own-uuid"}`)
	if got := claude.CodexSessionID(); got != "" {
		t.Errorf("claude-engine session CodexSessionID = %q, want \"\" (capture must be codex-gated)", got)
	}
}

// TestCodexHookFullTurnLoop drives the exact production loop of the on-disk
// codex profile through the receiver: SessionStart (hooks-alive + capture) ->
// working -> PermissionRequest -> waiting -> Stop -> idle.
func TestCodexHookFullTurnLoop(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)

	const codexID = "0d1a2b3c-3333-3333-3333-333333333333"
	steps := []struct {
		body   string
		status string
	}{
		{`{"hook_event_name":"SessionStart","source":"startup","session_id":"` + codexID + `"}`, "working"},
		{`{"hook_event_name":"PermissionRequest","tool_name":"Bash","session_id":"` + codexID + `"}`, "waiting"},
		{`{"hook_event_name":"Stop","session_id":"` + codexID + `"}`, "idle"},
	}
	for i, step := range steps {
		if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, step.body); got != http.StatusNoContent {
			t.Fatalf("step %d: status = %d, want 204", i, got)
		}
		if got := sess.Info().AgentStatus; got != step.status {
			t.Errorf("step %d (%s): AgentStatus = %q, want %q", i, step.body, got, step.status)
		}
	}
	if got := sess.CodexSessionID(); got != codexID {
		t.Errorf("CodexSessionID after loop = %q, want %q", got, codexID)
	}
}

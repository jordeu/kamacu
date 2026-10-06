package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/session"
)

// HTTP regression coverage for Codex lifecycle status and session-id capture.
// The receiver dispatches Codex events to its engine-specific reducer while
// preserving the Claude/OpenCode hook contract.

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

// newCodexHookServer wires the hook receiver over a fresh Manager
// whose agent config carries the hook token + base URL a codex process
// receives via env (D014). Both engine paths use isolated test stubs.
func newCodexHookServer(t *testing.T) (*httptest.Server, *session.Manager) {
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

// PermissionRequest runs before automatic review. Its telemetry alone must
// never claim a user is blocked, even when the request arrives after an answer.
func TestCodexHookPermissionRequestDoesNotSetWaiting(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)
	url := hookURL(srv, sess.Info().ID)
	steps := []struct{ body, want string }{
		{`{"hook_event_name":"UserPromptSubmit","turn_id":"t1"}`, "working"},
		{`{"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"Bash","tool_input":{"command":"sleep 30"}}`, "working"},
		{`{"hook_event_name":"PostToolUse","turn_id":"t1","tool_name":"Bash","tool_use_id":"call1","tool_input":{"command":"sleep 30"}}`, "working"},
		{`{"hook_event_name":"Stop","turn_id":"t1"}`, "idle"},
		{`{"hook_event_name":"PermissionRequest","turn_id":"t1"}`, "idle"},
		{`{"hook_event_name":"PostToolUse","turn_id":"t1"}`, "idle"},
		{`{"hook_event_name":"UserPromptSubmit","turn_id":"t2"}`, "working"},
		{`{"hook_event_name":"Stop","turn_id":"t1"}`, "working"},
		{`{"hook_event_name":"Interrupt","turn_id":"t2"}`, "idle"},
	}
	for _, step := range steps {
		if got := postHook(t, url, testHookToken, step.body); got != http.StatusNoContent {
			t.Fatalf("hook = %d", got)
		}
		if got := sess.Info().AgentStatus; got != step.want {
			t.Fatalf("%s: got %s, want %s", step.body, got, step.want)
		}
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
// working -> automatic approval -> working -> Stop -> idle.
func TestCodexHookFullTurnLoop(t *testing.T) {
	srv, mgr := newCodexHookServer(t)
	sess := spawnCodexAgent(t, mgr)

	const codexID = "0d1a2b3c-3333-3333-3333-333333333333"
	steps := []struct {
		body   string
		status string
	}{
		{`{"hook_event_name":"SessionStart","source":"startup","session_id":"` + codexID + `"}`, "working"},
		{`{"hook_event_name":"PermissionRequest","tool_name":"Bash","session_id":"` + codexID + `"}`, "working"},
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

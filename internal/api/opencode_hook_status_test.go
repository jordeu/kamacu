package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kamacu/internal/session"
)

// This file is the cross-cutting integration proof for M002/S01/T04: the
// UNCHANGED, engine-agnostic hook receiver (hooks.go) drives an opencode-
// engine session through working/waiting/idle — exactly the loop the on-disk
// opencode plugin (internal/opencode/kamacu-status.js) drives in production
// (permission.ask -> Notification, session.idle -> Stop, session.created ->
// SessionStart). hooks.go is not imported or modified here; only SpawnOpts
// (AgentEngine="opencode") and the public Session hook methods are exercised.
//
// The dependency-direction invariant is respected: this is the api package
// driving session.Manager.Spawn + Session.{SetWaiting,SetIdle,MarkHooksAlive}
// over HTTP exactly as production does.

// writeFakeOpencode writes an executable stub standing in for the opencode
// binary. opencode spawns via the custom command-render arm (AgentArgs), so
// the stub is referenced by absolute path (LookPath handles that). It mirrors
// writeFakeClaude's BEL-on-input behavior so the hooksAlive canary (Pitfall 2:
// a bare BEL sets waiting ONLY while hooks are not confirmed alive) can be
// proven for an opencode-engine session too. Each input line is answered with
// a bare BEL followed by an "bel-MARK:<line>" marker in ONE write.
func writeFakeOpencode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-opencode")
	script := `#!/usr/bin/env bash
echo "fake opencode ready"
trap 'exit 143' TERM
while IFS= read -r line; do
  printf '\abel-MARK:%s\n' "$line"
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake opencode stub: %v", err)
	}
	return path
}

// newOpencodeHookServer wires the UNCHANGED hook receiver over a fresh Manager
// whose agent config carries the hook token + base URL an opencode process
// would receive via env (D014). No ClaudeBin: opencode spawns via AgentArgs.
func newOpencodeHookServer(t *testing.T) (*httptest.Server, *session.Manager) {
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

// spawnOpencodeAgent spawns an opencode-engine agent session driven by the
// fake-opencode stub. This is the only place the test touches the engine
// selection: everywhere else it talks to the engine-agnostic hook receiver.
func spawnOpencodeAgent(t *testing.T, mgr *session.Manager) *session.Session {
	t.Helper()
	sess, err := mgr.Spawn(session.SpawnOpts{
		Kind:        session.KindAgent,
		Cwd:         t.TempDir(),
		TaskID:      1,
		AgentEngine: "opencode",
		AgentArgs:   []string{writeFakeOpencode(t)},
	})
	if err != nil {
		t.Fatalf("spawn opencode agent: %v", err)
	}
	return sess
}

// TestOpencodeHookKeepsHeuristicStates proves the T02 contract at the
// integration layer: an opencode-engine agent session reports one of the
// claude-style heuristic states (working/idle/waiting), NEVER the custom-
// engine "running" collapse. This is what lets the hook receiver's status
// transitions show up at all — under the old custom gate the receiver's POSTs
// would be silently ignored by agentStatusLocked.
func TestOpencodeHookKeepsHeuristicStates(t *testing.T) {
	_, mgr := newOpencodeHookServer(t)
	sess := spawnOpencodeAgent(t, mgr)
	defer sess.Stop()

	info := sess.Info()
	if info.Engine != "opencode" {
		t.Errorf("Engine = %q, want \"opencode\"", info.Engine)
	}
	switch info.AgentStatus {
	case "working", "idle", "waiting":
		// ok — heuristics intact, NOT collapsed to "running"
	default:
		t.Errorf("opencode AgentStatus = %q, want one of working/idle/waiting (T02 full-heuristics gate broken — hooks would be ignored under the old custom collapse)", info.AgentStatus)
	}
}

// TestOpencodeHookNotificationSetsWaiting: a Notification hook POST with the
// correct token flips an opencode-engine agent to waiting — the
// permission.ask -> Notification mapping the on-disk plugin emits. Mirrors
// TestHookNotificationSetsWaiting for the opencode engine.
func TestOpencodeHookNotificationSetsWaiting(t *testing.T) {
	srv, mgr := newOpencodeHookServer(t)
	sess := spawnOpencodeAgent(t, mgr)
	defer sess.Stop()

	body := `{"hook_event_name":"Notification","session_id":"u","notification_type":"permission_prompt"}`
	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, body); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Errorf("opencode AgentStatus = %q, want %q", got, "waiting")
	}
}

// TestOpencodeHookStopSetsIdle: a Stop hook POST flips the opencode-engine
// agent to idle (turn end) — the session.idle -> Stop mapping the on-disk
// plugin emits.
func TestOpencodeHookStopSetsIdle(t *testing.T) {
	srv, mgr := newOpencodeHookServer(t)
	sess := spawnOpencodeAgent(t, mgr)
	defer sess.Stop()

	if got := postHook(t, hookURL(srv, sess.Info().ID), testHookToken, `{"hook_event_name":"Stop"}`); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "idle" {
		t.Errorf("opencode AgentStatus = %q, want %q", got, "idle")
	}
}

// TestOpencodeHookSessionStartMarksHooksAlive: SessionStart returns 204 and
// disables the BEL fallback for an opencode-engine session (Pitfall 2). This
// is the hooksAlive canary the slice verification calls out as the diagnostic
// for "the plugin successfully reached the hook receiver" — if it never flips,
// the plugin/env path is broken. Mirrors TestHookSessionStartMarksHooksAlive
// for the opencode engine, proving the on-disk plugin's session.created ->
// SessionStart mapping lands in the same place.
func TestOpencodeHookSessionStartMarksHooksAlive(t *testing.T) {
	srv, mgr := newOpencodeHookServer(t)

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

	// Control: hooks-dead opencode session — BEL sets waiting (fallback works
	// for the opencode engine; the heuristics gate is open).
	dead := spawnOpencodeAgent(t, mgr)
	driveBEL(t, dead, "ping-dead")
	if got := dead.Info().AgentStatus; got != "waiting" {
		t.Fatalf("hooks-dead BEL: opencode AgentStatus = %q, want %q (fallback broken — alive assertion below would be vacuous)", got, "waiting")
	}

	// SessionStart-confirmed opencode session — BEL is ignored.
	alive := spawnOpencodeAgent(t, mgr)
	if got := postHook(t, hookURL(srv, alive.Info().ID), testHookToken, `{"hook_event_name":"SessionStart","source":"startup"}`); got != http.StatusNoContent {
		t.Fatalf("SessionStart status = %d, want 204", got)
	}
	driveBEL(t, alive, "ping-alive")
	if got := alive.Info().AgentStatus; got == "waiting" {
		t.Errorf("hooks-alive BEL flipped opencode status to waiting; BEL must be ignored once SessionStart confirmed the pipeline (hooksAlive canary broken)")
	}
}

// TestOpencodeHookFullTurnLoop exercises the exact event sequence the on-disk
// opencode plugin emits for one agent turn, end to end through the UNCHANGED
// receiver: SessionStart (session.created) -> Notification (permission.ask) ->
// Stop (session.idle). Each transition is observed on Info().AgentStatus,
// proving hooks.go needs no engine knowledge to drive an opencode task's
// status dot through working/waiting/idle.
func TestOpencodeHookFullTurnLoop(t *testing.T) {
	srv, mgr := newOpencodeHookServer(t)
	sess := spawnOpencodeAgent(t, mgr)
	defer sess.Stop()
	id := sess.Info().ID

	// 1. session.created -> SessionStart -> hooksAlive (BEL fallback off).
	if got := postHook(t, hookURL(srv, id), testHookToken, `{"hook_event_name":"SessionStart","source":"startup"}`); got != http.StatusNoContent {
		t.Fatalf("SessionStart: status = %d, want 204", got)
	}

	// 2. permission.ask -> Notification -> waiting.
	if got := postHook(t, hookURL(srv, id), testHookToken, `{"hook_event_name":"Notification","session_id":"u","notification_type":"permission_prompt"}`); got != http.StatusNoContent {
		t.Fatalf("Notification: status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "waiting" {
		t.Fatalf("after Notification: opencode AgentStatus = %q, want %q", got, "waiting")
	}

	// 3. session.idle -> Stop -> idle (turn end).
	if got := postHook(t, hookURL(srv, id), testHookToken, `{"hook_event_name":"Stop"}`); got != http.StatusNoContent {
		t.Fatalf("Stop: status = %d, want 204", got)
	}
	if got := sess.Info().AgentStatus; got != "idle" {
		t.Fatalf("after Stop: opencode AgentStatus = %q, want %q", got, "idle")
	}
}

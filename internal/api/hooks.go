package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"kamacu/internal/session"
)

// HookRoutes registers the Claude Code hook receiver (STAT-02). Each agent
// spawn injects a settings overlay whose hooks curl this endpoint with the
// per-server-instance token; the receiver translates hook events into agent
// status transitions (04-RESEARCH.md Pattern 3).
//
// Security note (verified middleware interplay): the hook curl's Host header
// is loopback and passes the port-agnostic hostCheck, and REST routes never
// check Origin — but a malicious webpage can fire a no-CORS fetch POST at
// localhost too. The token is therefore REQUIRED, never optional.
func HookRoutes(mux *http.ServeMux, mgr *session.Manager, token string) {
	h := &hookHandlers{mgr: mgr, token: token}
	mux.HandleFunc("POST /api/hooks/sessions/{id}", h.receive)
}

type hookHandlers struct {
	mgr   *session.Manager
	token string // per-instance random token; "" means reject everything
}

// receive handles POST /api/hooks/sessions/{id} (id = Kamacu session ID).
// The hook payload arrives verbatim on the body (curl --data-binary @-); the
// receiver switches on hook_event_name ONLY and tolerates unknown fields and
// events — claude adds both across releases. It never blocks: every dispatch
// is a mutex flip on the session.
func (h *hookHandlers) receive(w http.ResponseWriter, r *http.Request) {
	// Token first. An empty configured token rejects everything — the
	// receiver must never run open (defense in depth; main always generates
	// one). Constant-time compare keeps the gate timing-safe.
	got := r.Header.Get("X-Kamacu-Token")
	if h.token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// A hook racing the session's exit is expected (async curl) — ignore.
	if sess.Info().Status == session.StatusExited {
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload struct {
		HookEventName    string          `json:"hook_event_name"`
		SessionID        string          `json:"session_id"`
		NotificationType string          `json:"notification_type"`
		Source           string          `json:"source"`
		TurnID           string          `json:"turn_id"`
		ToolUseID        string          `json:"tool_use_id"`
		ToolName         string          `json:"tool_name"`
		ToolInput        json.RawMessage `json:"tool_input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Codex session-id capture (restart-resume key): codex's hook payloads
	// carry codex's OWN session uuid on every event (verified shape:
	// {"session_id":"<uuid>","hook_event_name":…}). Record it on codex-engine
	// sessions only — claude/opencode payloads also carry their own ids and
	// must never poison the codex resume column. First capture wins (a later
	// codex-side /resume must not retarget the persisted key); SetCodexSessionID
	// enforces that internally.
	if sess.Info().Engine == "codex" && payload.SessionID != "" {
		sess.SetCodexSessionID(payload.SessionID)
	}

	if sess.Info().Engine == "codex" {
		sess.ApplyCodexHook(session.CodexHook{Event: payload.HookEventName, SessionID: payload.SessionID, Source: payload.Source, TurnID: payload.TurnID, ToolUseID: payload.ToolUseID, ToolName: payload.ToolName, ToolInput: payload.ToolInput})
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch payload.HookEventName {
	case "Notification":
		sess.SetWaiting() // permission_prompt | elicitation_dialog (overlay matcher)
	case "PermissionRequest":
		// codex's dedicated approval-prompt event (claude uses Notification
		// with notification_type=permission_prompt) — same SetWaiting effect.
		sess.SetWaiting()
	case "Stop":
		sess.SetIdle()
	case "SessionStart":
		sess.MarkHooksAlive() // pipeline canary: from now on the BEL fallback is off
	default:
		// Unknown events are a tolerated no-op (forward compatibility).
	}
	w.WriteHeader(http.StatusNoContent)
}

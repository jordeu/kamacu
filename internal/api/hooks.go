package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"kangent/internal/session"
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

// receive handles POST /api/hooks/sessions/{id} (id = Kangent session ID).
// The hook payload arrives verbatim on the body (curl --data-binary @-); the
// receiver switches on hook_event_name ONLY and tolerates unknown fields and
// events — claude adds both across releases. It never blocks: every dispatch
// is a mutex flip on the session.
func (h *hookHandlers) receive(w http.ResponseWriter, r *http.Request) {
	// Token first. An empty configured token rejects everything — the
	// receiver must never run open (defense in depth; main always generates
	// one). Constant-time compare keeps the gate timing-safe.
	got := r.Header.Get("X-Kangent-Token")
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
		HookEventName    string `json:"hook_event_name"`
		SessionID        string `json:"session_id"`
		NotificationType string `json:"notification_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	switch payload.HookEventName {
	case "Notification":
		sess.SetWaiting() // permission_prompt | elicitation_dialog (overlay matcher)
	case "Stop":
		sess.SetIdle()
	case "SessionStart":
		sess.MarkHooksAlive() // pipeline canary: from now on the BEL fallback is off
	default:
		// Unknown events are a tolerated no-op (forward compatibility).
	}
	w.WriteHeader(http.StatusNoContent)
}

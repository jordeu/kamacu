package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"kamacu/internal/presence"
)

// PresenceRoutes registers the user-presence surface the UI beacon feeds
// and the MCP bridge reads:
//
//	POST /api/presence/heartbeat — UI-facing beat (loopback trust, the same
//	  model as every other UI POST; the payload is metadata only).
//	GET  /api/presence           — the snapshot, TOKEN-GATED with the same
//	  constant-time X-Kamacu-Token compare as the hooks receiver. Unlike the
//	  other read-only GETs (loopback-only binding is their boundary), the
//	  snapshot describes the user's attention and is consumed by the MCP
//	  bridge, which already sends the token on every request — so the extra
//	  gate costs nothing and keeps the surface closed.
func PresenceRoutes(mux *http.ServeMux, tr *presence.Tracker, token string) {
	h := &presenceHandlers{tracker: tr, token: token}
	mux.HandleFunc("POST /api/presence/heartbeat", h.heartbeat)
	mux.HandleFunc("GET /api/presence", h.snapshot)
}

type presenceHandlers struct {
	tracker *presence.Tracker
	token   string // per-instance random token; "" means reject the snapshot read
}

// heartbeat handles POST /api/presence/heartbeat. client_id (the tab
// identity, the upsert key) and visible are REQUIRED — visible is decoded
// through a pointer so an absent field is a 400 rather than a silent
// "visible:false" default hiding a beacon bug.
func (h *presenceHandlers) heartbeat(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ClientID  string `json:"client_id"`
		Route     string `json:"route"`
		ProjectID int64  `json:"project_id"`
		TaskID    int64  `json:"task_id"`
		SessionID string `json:"session_id"`
		Visible   *bool  `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.ClientID == "" {
		writeError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if payload.Visible == nil {
		writeError(w, http.StatusBadRequest, "visible is required")
		return
	}
	h.tracker.Heartbeat(presence.Beat{
		ClientID:  payload.ClientID,
		Route:     payload.Route,
		ProjectID: payload.ProjectID,
		TaskID:    payload.TaskID,
		SessionID: payload.SessionID,
		Visible:   *payload.Visible,
	})
	w.WriteHeader(http.StatusNoContent)
}

// snapshot handles GET /api/presence (the MCP bridge leg). Token first,
// timing-safe, never open — the hooks.go hardening contract.
func (h *presenceHandlers) snapshot(w http.ResponseWriter, r *http.Request) {
	got := r.Header.Get("X-Kamacu-Token")
	if h.token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, h.tracker.Snapshot())
}

package ws

import (
	"net/http"

	"kangent/internal/session"
)

// Handler upgrades GET /api/sessions/{id}/ws requests and bridges the
// connection to a session.
type Handler struct {
	mgr            *session.Manager
	originPatterns []string
}

// NewHandler returns a Handler serving sessions from mgr. originPatterns is
// the Origin allowlist passed to websocket.Accept (the request's own Host is
// always authorized).
func NewHandler(mgr *session.Manager, originPatterns []string) *Handler {
	return &Handler{mgr: mgr, originPatterns: originPatterns}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

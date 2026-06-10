package api

import (
	"net/http"

	"kangent/internal/session"
)

// SessionRoutes registers terminal session endpoints on mux. Sessions are
// memory-only this phase and are served by a *session.Manager rather than
// *sql.DB, so they get their own registration function alongside Routes.
func SessionRoutes(mux *http.ServeMux, mgr *session.Manager) {
}

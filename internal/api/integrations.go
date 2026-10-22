package api

import (
	"net/http"

	"kamacu/internal/integration"
)

// IntegrationRoutes manages files on the server host, like the existing agent
// settings. The server's same-origin middleware protects these browser writes.
func IntegrationRoutes(mux *http.ServeMux, manager *integration.Manager) {
	mux.HandleFunc("GET /api/integrations", func(w http.ResponseWriter, r *http.Request) {
		statuses, err := manager.Status(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, statuses)
	})
	mux.HandleFunc("POST /api/integrations/{engine}/{action}", func(w http.ResponseWriter, r *http.Request) {
		engine := r.PathValue("engine")
		action := r.PathValue("action")
		if engine != "claude" && engine != "codex" && engine != "opencode" {
			writeError(w, 400, "unsupported agent engine")
			return
		}
		var result integration.Status
		var err error
		switch action {
		case "enable", "repair":
			result, err = manager.Enable(r.Context(), engine)
		case "remove":
			result, err = manager.Disable(r.Context(), engine)
		default:
			writeError(w, 400, "unknown integration action")
			return
		}
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, result)
	})
}

package api

import (
	"net/http"

	"kamacu/internal/quota"
)

// UsageRoutes registers the quota proxy. Always 200 — the Result state
// field carries degradation; the browser never talks to Anthropic.
func UsageRoutes(mux *http.ServeMux, qs *quota.Service) {
	mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, qs.Get(r.Context(), r.URL.Query().Get("refresh") == "1"))
	})
}

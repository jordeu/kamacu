package api

import (
	"net/http"

	"kamacu/internal/github"
)

// githubStatus handles GET /api/github/status — an always-200 read of whether
// the host `gh` CLI is installed (call-time LookPath via github.Available()),
// modeled on the internal/quota degrade endpoints. `gh` is a soft dependency:
// this endpoint never errors and never blocks. The frontend gates the GitHub
// integration toggle's default/enable behavior on this flag (GHSET-01/GHSET-03).
func githubStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"gh_available": github.Available()})
}

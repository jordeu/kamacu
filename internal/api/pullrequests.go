package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"kangent/internal/github"
	"kangent/internal/settings"
)

// PullRequestRoutes registers GET /api/projects/{id}/pull-requests[?refresh=1].
//
// Always 200 — the Result.State field carries degradation (GHSET-03 / GHCOL-05);
// the frontend branches on state/stale, never on the HTTP status. This endpoint
// is the GHSET-02 backend enforcement point: it returns state=disabled when
// github_integration is off OR the project is unlinked, BEFORE ever touching gh.
// The frontend's useSettings() gate is convenience; this server gate is the one
// that actually prevents a gh spawn.
//
// Registered as a sibling of UsageRoutes (not inside Routes()) so the api
// package's newTestServer is unaffected and the PR endpoint test can register
// its own mux with a fake-runner Service.
func PullRequestRoutes(mux *http.ServeMux, db *sql.DB, svc *github.Service) {
	mux.HandleFunc("GET /api/projects/{id}/pull-requests", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		// GATE 1 (GHSET-02): integration toggle off -> disabled, never spawn gh.
		// settings.Get returns the code default "on" for an absent row, so any
		// non-"on" value (including a future "off") disables.
		val, err := settings.Get(db, settings.KeyGithubIntegration)
		if err != nil {
			writeJSON(w, http.StatusOK, github.Result{State: "error"})
			return
		}
		if val != "on" {
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}

		// GATE 2: project not linked -> disabled, never spawn gh. Fetch repo_path
		// alongside github_repo for cmd.Dir (Pitfall 6: gh host/account select).
		var repo sql.NullString
		var repoPath string
		qerr := db.QueryRow(`SELECT github_repo, repo_path FROM projects WHERE id = ?`, id).Scan(&repo, &repoPath)
		if errors.Is(qerr, sql.ErrNoRows) {
			// Unknown project still returns disabled (200), NOT 404 — the column
			// never blocks the board on a project-lookup miss (degrade-don't-break).
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}
		if qerr != nil {
			writeJSON(w, http.StatusOK, github.Result{State: "error"})
			return
		}
		if !repo.Valid || strings.TrimSpace(repo.String) == "" {
			writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
			return
		}

		force := r.URL.Query().Get("refresh") == "1"
		writeJSON(w, http.StatusOK, svc.Get(r.Context(), repo.String, repoPath, force))
	})
}

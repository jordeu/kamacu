package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"

	"kangent/internal/diff"
	"kangent/internal/worktree"
)

// DiffRoutes registers the read-only review endpoint (REVW-01). Computation is
// on-demand only — no caching, no polling (D-61). The diff is everything the
// task changed vs the merge-base of its base branch (D-59).
func DiffRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service) {
	h := &diffHandlers{db: db, wt: wt}
	mux.HandleFunc("GET /api/tasks/{id}/diff", h.get)
}

type diffHandlers struct {
	db *sql.DB
	wt *worktree.Service
}

// get handles GET /api/tasks/{id}/diff. The base is re-resolved at request time
// (D-59 is defined against the CURRENT base, not a stored creation-time value);
// errors relay git's trimmed stderr verbatim into the UI-SPEC error card rather
// than leaking a raw 500 or a blank tab (Pitfall 7).
func (h *diffHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	// One query for the task's worktree_path + its project's repo_path (mirror
	// worktreeHandlers.loadTaskRepo; the FK guarantees the project row exists).
	var wtPath sql.NullString
	var repo string
	err := h.db.QueryRow(
		`SELECT worktree_path,
		   (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE id = ?`, id).Scan(&wtPath, &repo)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// No worktree → the same 409 copy as every other worktree gate (D-64's
	// disabled tab means the client rarely hits this, but the server never
	// trusts the client).
	if !wtPath.Valid {
		writeError(w, http.StatusConflict, "task has no worktree")
		return
	}
	path := wtPath.String

	// The dir may have been deleted out from under us (Pitfall 7) — stat first
	// and relay a clear message into the error card rather than a raw git 128.
	if fi, statErr := os.Stat(path); statErr != nil || !fi.IsDir() {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("worktree directory is missing: %s", path))
		return
	}

	// Re-resolve the base (its 4-step chain covers most base weirdness; failure
	// routes to the error state).
	base, err := h.wt.ResolveBase(r.Context(), repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	d, err := diff.Compute(r.Context(), path, base)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.Base = base // totals bar + empty state share the resolved base
	writeJSON(w, http.StatusOK, d)
}

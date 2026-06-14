package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

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

	// One query for the task's worktree_path, source/pr_base_ref (the diff-base
	// discriminator — D-12/GHREV-03), + its project's repo_path (mirror
	// worktreeHandlers.loadTaskRepo; the FK guarantees the project row exists).
	var wtPath sql.NullString
	var repo string
	var source string
	var prBaseRef sql.NullString
	err := h.db.QueryRow(
		`SELECT worktree_path, source, pr_base_ref,
		   (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE id = ?`, id).Scan(&wtPath, &source, &prBaseRef, &repo)
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

	// Select the diff base. A github_pr review (D-12/GHREV-03) diffs against the
	// PR's OWN base (pr_base_ref), not the project default — otherwise a PR
	// targeting a non-default branch shows a misleading mega-diff. A manual task
	// (or a defensive github_pr row with NULL pr_base_ref) keeps the unchanged
	// ResolveBase chain. The base is re-read + re-resolved on every request (no
	// caching), so a retargeted PR picks up its new base on the next open.
	var base string
	if source == "github_pr" && prBaseRef.Valid && strings.TrimSpace(prBaseRef.String) != "" {
		baseName := strings.TrimSpace(prBaseRef.String)
		// Fetch the base FIRST so origin/<base> exists before merge-base
		// (RESEARCH Pitfall 3 — "Not a valid object name origin/<base>"
		// otherwise). Best-effort: ignore the fetch error and let merge-base
		// surface a clear message into the UI error card if the ref truly can't
		// resolve. Then prefer a local refs/heads/<base>, else origin/<base>
		// (the stable remote-tracking ref — never FETCH_HEAD, Pitfall 1).
		_ = h.wt.FetchRef(r.Context(), repo, baseName)
		base = resolvePRBase(r.Context(), path, baseName)
	} else {
		// Re-resolve the project base (its 4-step chain covers most base
		// weirdness; failure routes to the error state).
		base, err = h.wt.ResolveBase(r.Context(), repo)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	d, err := diff.Compute(r.Context(), path, base)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.Base = base // totals bar + empty state share the resolved base
	writeJSON(w, http.StatusOK, d)
}

// resolvePRBase resolves a PR base branch name to a commit-ish for merge-base,
// mirroring worktree.ResolveBase's local-then-remote shape (D-12): prefer a
// local refs/heads/<baseName> if present, else the remote-tracking
// origin/<baseName> (stable after FetchRef — NEVER FETCH_HEAD, RESEARCH Pitfall
// 1). show-ref runs in the worktree dir; worktrees share the common git dir, so
// refs/heads and refs/remotes resolve identically there. If neither ref exists,
// origin/<baseName> is returned and diff.Compute's merge-base relays git's clear
// "Not a valid object name" message into the UI error card (Pitfall 3 posture).
func resolvePRBase(ctx context.Context, wt, baseName string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", wt,
		"show-ref", "--verify", "--quiet", "refs/heads/"+baseName)
	if err := cmd.Run(); err == nil {
		return baseName // local branch tip
	}
	return "origin/" + baseName // remote-tracking ref (fetched above)
}

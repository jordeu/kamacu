package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"kamacu/internal/diff"
	"kamacu/internal/worktree"
)

// DiffRoutes registers the read-only review endpoint (REVW-01). Computation is
// on-demand only — no caching, no polling (D-61). The diff is everything the
// task changed vs the merge-base of its base branch (D-59).
func DiffRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service) {
	h := &diffHandlers{db: db, wt: wt}
	mux.HandleFunc("GET /api/tasks/{id}/diff", h.get)
	mux.HandleFunc("PUT /api/tasks/{id}/diff/viewed", h.setViewed)
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
	// discriminator — D-12/GHREV-03), + its project's repo_path and managed
	// marker (mirror worktreeHandlers.loadTaskRepo; the FK guarantees the
	// project row exists).
	var wtPath sql.NullString
	var repo string
	var source string
	var prBaseRef sql.NullString
	var managedInt int
	err := h.db.QueryRow(
		`SELECT worktree_path, source, pr_base_ref,
		   (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id),
		   (SELECT managed FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE id = ?`, id).Scan(&wtPath, &source, &prBaseRef, &repo, &managedInt)
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
	} else if managedInt != 0 {
		// Managed project (CKOUT-02 parity with provisioning): diff against
		// origin/<default> — the same ref new task branches are based on — so
		// the tab shows exactly what a PR against the default branch will
		// show. ResolveBaseFresh best-effort fetches the default branch first
		// (the managed-gated scoped D-24 exception), so the base reflects the
		// CURRENT origin tip at every open, then falls back to the local chain
		// when the ref can't resolve — never an error card for a stale base.
		base, err = h.wt.ResolveBaseFresh(r.Context(), repo)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		// Folder project: best-effort fetch of the default branch so the
		// merge-base reflects the latest upstream state (network-free
		// projects — folder repos with no remote, offline work — simply skip:
		// the fetch error is discarded and ResolveBase proceeds on the local
		// base; the fetch only updates refs/remotes/origin/<branch>, never
		// the working tree or local branches). The resolved BASE itself stays
		// the local chain — folder projects never rebase their notion of the
		// default branch onto origin.
		if defaultBranch, derr := h.wt.DefaultBranch(r.Context(), repo); derr == nil {
			_ = h.wt.FetchRef(r.Context(), repo, defaultBranch)
		}
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

	// Merge per-file "Viewed" state (DIFF-03/04). A file reads viewed iff a
	// diff_viewed row exists at its CURRENT rendered hash (keep-history, D-02):
	// a changed file mints a new hash with no matching row -> un-viewed; a revert
	// to a byte-identical diff restores the old hash -> its lingering row matches
	// again. One drained SELECT, cursor CLOSED before we touch d.Files -- safe
	// under SetMaxOpenConns(1) (icons.go discipline). Parameterized (?) only.
	type viewedKey struct{ path, hash string }
	viewed := map[viewedKey]bool{}
	rows, qErr := h.db.Query(`SELECT file_path, diff_hash FROM diff_viewed WHERE task_id = ?`, id)
	if qErr != nil {
		writeError(w, http.StatusInternalServerError, qErr.Error())
		return
	}
	for rows.Next() {
		var fp, hash string
		if scanErr := rows.Scan(&fp, &hash); scanErr != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, scanErr.Error())
			return
		}
		viewed[viewedKey{fp, hash}] = true
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, rowsErr.Error())
		return
	}
	rows.Close()
	for i := range d.Files {
		if viewed[viewedKey{d.Files[i].Path, d.Files[i].Hash}] {
			d.Files[i].Viewed = true
		}
	}

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

// setViewed handles the per-file Viewed write endpoint (DIFF-03). It toggles the
// exact keep-history row for (task, path, hash): viewed=true INSERTs it
// (idempotent via ON CONFLICT DO NOTHING), viewed=false DELETEs precisely that
// row. The client sends the hash it JUST rendered; we TRUST it and do NOT
// re-Compute (RESEARCH §Area 2) -- a stale/garbage-but-well-formed row never
// matches the current rendered hash on the next open (so it shows un-viewed,
// correct) and is cascade-pruned when the task is deleted. All SQL uses '?'
// placeholders only (T-22-01, never string-concatenated). Input is bounded +
// validated (T-22-03) so oversized/garbage path/hash is rejected 400, never
// persisted.
func (h *diffHandlers) setViewed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Path   string `json:"path"`
		Hash   string `json:"hash"`
		Viewed bool   `json:"viewed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if len(req.Path) > 4096 {
		writeError(w, http.StatusBadRequest, "path is too long")
		return
	}
	if !isDiffHash(req.Hash) {
		writeError(w, http.StatusBadRequest, "hash must be 64 lowercase hex characters")
		return
	}
	if req.Viewed {
		if _, err := h.db.Exec(
			`INSERT INTO diff_viewed(task_id, file_path, diff_hash) VALUES(?,?,?) ON CONFLICT(task_id, file_path, diff_hash) DO NOTHING`,
			id, req.Path, req.Hash); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if _, err := h.db.Exec(
			`DELETE FROM diff_viewed WHERE task_id=? AND file_path=? AND diff_hash=?`,
			id, req.Path, req.Hash); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// isDiffHash reports whether s is exactly 64 lowercase hex characters -- the
// shape of the sha256 hex fingerprint from internal/diff.hashFile. Bounds the
// stored hash and rejects garbage cheaply (T-22-03).
func isDiffHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

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
	"path/filepath"
	"strings"

	"kamacu/internal/diff"
	"kamacu/internal/worktree"
)

// DiffRoutes registers the read-only review endpoints (REVW-01). Computation is
// on-demand only — no caching, no polling (D-61). The diff is everything the
// task changed vs the merge-base of its base branch (D-59); the content
// endpoint backs the Diff tab's Markdown viewer (hunks carry only ±3 context
// lines, so the full texts are fetched on open).
func DiffRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service) {
	h := &diffHandlers{db: db, wt: wt}
	mux.HandleFunc("GET /api/tasks/{id}/diff", h.get)
	mux.HandleFunc("GET /api/tasks/{id}/diff/content", h.getContent)
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
	id, wtPath, repo, source, prBaseRef, managed, ok := h.diffTaskScope(w, r)
	if !ok {
		return
	}

	// The dir may have been deleted out from under us (Pitfall 7) — stat first
	// and relay a clear message into the error card rather than a raw git 128.
	if !worktreeDirExists(w, wtPath) {
		return
	}

	base, err := h.resolveDiffBase(r.Context(), wtPath, repo, source, prBaseRef, managed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	d, err := diff.Compute(r.Context(), wtPath, base)
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

// diffTaskScope is the task grounding BOTH diff endpoints share: the path id,
// the task's worktree_path, source/pr_base_ref (the diff-base discriminators —
// D-12/GHREV-03), + its project's repo_path and managed marker (mirrors
// worktreeHandlers.loadTaskRepo; the FK guarantees the project row exists).
// Errors are relayed exactly as the diff GET always relayed them; ok=false
// means the response is already written.
func (h *diffHandlers) diffTaskScope(w http.ResponseWriter, r *http.Request) (id int64, wtPath, repo, source string, prBaseRef sql.NullString, managed bool, ok bool) {
	id, ok = pathID(w, r)
	if !ok {
		return
	}
	var wtNull sql.NullString
	var managedInt int
	err := h.db.QueryRow(
		`SELECT worktree_path, source, pr_base_ref,
		   (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id),
		   (SELECT managed FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE id = ?`, id).Scan(&wtNull, &source, &prBaseRef, &repo, &managedInt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return 0, "", "", "", sql.NullString{}, false, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return 0, "", "", "", sql.NullString{}, false, false
	}
	// No worktree → the same 409 copy as every other worktree gate (D-64's
	// disabled tab means the client rarely hits this, but the server never
	// trusts the client).
	if !wtNull.Valid {
		writeError(w, http.StatusConflict, "task has no worktree")
		return 0, "", "", "", sql.NullString{}, false, false
	}
	return id, wtNull.String, repo, source, prBaseRef, managedInt != 0, true
}

// worktreeDirExists stats the worktree dir and relays the missing-dir message
// into the error card on failure (Pitfall 7 posture, shared by both endpoints).
func worktreeDirExists(w http.ResponseWriter, path string) bool {
	if fi, statErr := os.Stat(path); statErr != nil || !fi.IsDir() {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("worktree directory is missing: %s", path))
		return false
	}
	return true
}

// resolveDiffBase selects the diff base — the SAME chain the diff GET always
// used, factored out so the content endpoint resolves identically (the viewer
// must map hunks from a diff computed against exactly this base).
//
// A github_pr review (D-12/GHREV-03) diffs against the PR's OWN base
// (pr_base_ref), not the project default — otherwise a PR targeting a
// non-default branch shows a misleading mega-diff. A manual task (or a
// defensive github_pr row with NULL pr_base_ref) keeps the unchanged
// ResolveBase chain. The base is re-read + re-resolved on every request (no
// caching), so a retargeted PR picks up its new base on the next open.
func (h *diffHandlers) resolveDiffBase(ctx context.Context, wtPath, repo, source string, prBaseRef sql.NullString, managed bool) (string, error) {
	if source == "github_pr" && prBaseRef.Valid && strings.TrimSpace(prBaseRef.String) != "" {
		baseName := strings.TrimSpace(prBaseRef.String)
		// Fetch the base FIRST so origin/<base> exists and is CURRENT before
		// merge-base (RESEARCH Pitfall 3 — "Not a valid object name
		// origin/<base>" otherwise). Best-effort: ignore the fetch error and
		// let resolvePRBase fall back / merge-base surface a clear message
		// into the UI error card if the ref truly can't resolve. The base is
		// always resolved against the remote-tracking origin/<base> — never
		// FETCH_HEAD (Pitfall 1).
		_ = h.wt.FetchRef(ctx, repo, baseName)
		return resolvePRBase(ctx, wtPath, baseName), nil
	}
	if managed {
		// Managed project (CKOUT-02 parity with provisioning): diff against
		// origin/<default> — the same ref new task branches are based on — so
		// the tab shows exactly what a PR against the default branch will
		// show. ResolveBaseFresh best-effort fetches the default branch first
		// (the managed-gated scoped D-24 exception), so the base reflects the
		// CURRENT origin tip at every open, then falls back to the local chain
		// when the ref can't resolve — never an error card for a stale base.
		return h.wt.ResolveBaseFresh(ctx, repo)
	}
	// Folder project: best-effort fetch of the default branch so the
	// merge-base reflects the latest upstream state (network-free projects —
	// folder repos with no remote, offline work — simply skip: the fetch error
	// is discarded and ResolveBase proceeds on the local base; the fetch only
	// updates refs/remotes/origin/<branch>, never the working tree or local
	// branches). The resolved BASE itself stays the local chain — folder
	// projects never rebase their notion of the default branch onto origin.
	if defaultBranch, derr := h.wt.DefaultBranch(ctx, repo); derr == nil {
		_ = h.wt.FetchRef(ctx, repo, defaultBranch)
	}
	// Re-resolve the project base (its 4-step chain covers most base
	// weirdness; failure routes to the error state).
	return h.wt.ResolveBase(ctx, repo)
}

// getContent handles GET /api/tasks/{id}/diff/content?path=<relpath> — the
// full two-sided text of ONE file's diff, backing the Diff tab's Markdown
// viewer (MDV-01). Same task grounding + base chain as the diff GET, so the
// texts are byte-compatible with the hunks the client already holds. The
// response is computed on demand, never cached (D-61 posture).
func (h *diffHandlers) getContent(w http.ResponseWriter, r *http.Request) {
	_, wtPath, repo, source, prBaseRef, managed, ok := h.diffTaskScope(w, r)
	if !ok {
		return
	}
	if !worktreeDirExists(w, wtPath) {
		return
	}

	rel := r.URL.Query().Get("path")
	if !validDiffPath(w, rel) {
		return
	}

	base, err := h.resolveDiffBase(r.Context(), wtPath, repo, source, prBaseRef, managed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fc, err := diff.Content(r.Context(), wtPath, base, rel)
	if errors.Is(err, diff.ErrNotInDiff) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, diff.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fc)
}

// validDiffPath bounds + validates a diff file path before any disk or git
// call (T-22-03 posture, mirroring setViewed's bounds): non-empty, capped at
// 4096 bytes, relative, no backslash, no ".." segment. ok=false means the 400
// response is already written.
func validDiffPath(w http.ResponseWriter, p string) bool {
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return false
	}
	if len(p) > 4096 {
		writeError(w, http.StatusBadRequest, "path is too long")
		return false
	}
	if filepath.IsAbs(p) || strings.ContainsRune(p, '\\') || strings.Contains(p, "..") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return false
	}
	return true
}

// resolvePRBase resolves a PR base branch name to a commit-ish for merge-base.
// It prefers the remote-tracking origin/<baseName> — freshly updated by the
// FetchRef immediately before this call — because a managed clone's LOCAL
// refs/heads/<baseName> is frozen at clone time, and diffing against that
// stale tip sweeps the base branch's own drift into the review (the mega-diff
// bug: GitHub's Files-changed is three-dot against the CURRENT origin base,
// not the clone-time snapshot). The local branch is only a fallback when
// origin/<baseName> is absent entirely (e.g. the fetch failed offline). If
// neither ref exists, origin/<baseName> is returned and diff.Compute's
// merge-base relays git's clear "Not a valid object name" message into the UI
// error card (Pitfall 3 posture). show-ref runs in the worktree dir;
// worktrees share the common git dir, so refs/heads and refs/remotes resolve
// identically there.
func resolvePRBase(ctx context.Context, wt, baseName string) string {
	if err := exec.CommandContext(ctx, "git", "-C", wt,
		"show-ref", "--verify", "--quiet", "refs/remotes/origin/"+baseName).Run(); err == nil {
		return "origin/" + baseName // current remote-tracking tip (fetched above)
	}
	if err := exec.CommandContext(ctx, "git", "-C", wt,
		"show-ref", "--verify", "--quiet", "refs/heads/"+baseName).Run(); err == nil {
		return baseName // offline fallback: the local tip, stale but present
	}
	return "origin/" + baseName // neither exists: merge-base surfaces the clear error
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

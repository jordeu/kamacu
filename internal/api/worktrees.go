package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"kangent/internal/session"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
)

// WorktreeRoutes registers the per-task worktree lifecycle endpoints. The
// DELETE gates (D-32 sessions, D-33 dirt) are enforced HERE, at request time:
// git itself happily removes a worktree with live processes cwd'd inside
// (Pitfall 4, verified) and the client dialog's snapshot can be stale
// (Pitfall 8), so the server is the only real safety net.
//
// tmuxClient is the dedicated-socket tmux surface (D-92/D-93): the cleanup
// dialog count folds in live DETACHED tmux sessions and remove kills them
// before wt.Remove — git happily deletes a tree with a tmux server still
// cwd'd inside (the orphan-shell hole).
func WorktreeRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service, mgr *session.Manager, tmuxClient tmux.Client) {
	h := &worktreeHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient}
	mux.HandleFunc("POST /api/tasks/{id}/worktree", h.create)
	mux.HandleFunc("GET /api/tasks/{id}/worktree", h.get)
	mux.HandleFunc("DELETE /api/tasks/{id}/worktree", h.remove)
}

type worktreeHandlers struct {
	db         *sql.DB
	wt         *worktree.Service
	mgr        *session.Manager
	tmuxClient tmux.Client
}

// loadTaskRepo fetches a task plus its project's repo_path in one query. The
// FK guarantees the project row exists, so the scalar subquery never NULLs.
func (h *worktreeHandlers) loadTaskRepo(id int64) (Task, string, error) {
	var t Task
	var repo string
	err := h.db.QueryRow(
		`SELECT `+taskColumns+`,
		   (SELECT repo_path FROM projects WHERE projects.id = tasks.project_id)
		 FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Position,
			&t.CreatedAt, &t.UpdatedAt, &t.Branch, &t.WorktreePath, &t.WorktreeError,
			&t.Source, &t.PRNumber, &t.PRBaseRef, &repo)
	return t, repo, err
}

// runningSessions counts the task's sessions that are still running.
func (h *worktreeHandlers) runningSessions(taskID int64) int {
	n := 0
	for _, info := range h.mgr.ListByTask(taskID) {
		if info.Status == session.StatusRunning {
			n++
		}
	}
	return n
}

// liveTmuxNames returns the tmux session names of the task that are ALIVE on
// the dedicated socket — detached OR attached. It probes every tmux_sessions
// row of the task with has-session (the row is identity-only; tmux itself is
// the liveness authority). A name is collected only when has-session is
// CONCLUSIVELY alive (alive && err == nil): an inconclusive probe (tmux binary
// broken/hung) is never read as "alive" nor as "dead" (Pitfall 6 honesty), so
// a broken tmux neither inflates the count nor triggers a kill.
//
// This is the load-bearing detached-survivor list: StopAllForTask only reaches
// IN-MEMORY sessions, but a tmux session that survived a leave/restart has no
// in-memory session — only its DB row points at it.
func (h *worktreeHandlers) liveTmuxNames(ctx context.Context, taskID int64) []string {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		slog.Warn("listing tmux_sessions for task", "task", taskID, "error", err)
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Warn("scanning tmux_sessions row", "task", taskID, "error", err)
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterating tmux_sessions rows", "task", taskID, "error", err)
	}
	live := make([]string, 0, len(names))
	for _, name := range names {
		alive, err := h.tmuxClient.HasSession(ctx, name)
		if alive && err == nil {
			live = append(live, name)
		}
	}
	return live
}

// cleanupSessionCount folds live detached tmux sessions into the single
// running-sessions number the cleanup dialog shows (D-92). It is ONE honest
// count with no tmux-specific field (D-77): in-memory running sessions plus
// every live tmux row the Manager has NO live session for (HasLiveTmux false),
// so an in-memory tmux session — counted once by runningSessions — is never
// double-counted.
func (h *worktreeHandlers) cleanupSessionCount(ctx context.Context, taskID int64) int {
	count := h.runningSessions(taskID)
	for _, name := range h.liveTmuxNames(ctx, taskID) {
		if !h.mgr.HasLiveTmux(name) {
			count++
		}
	}
	return count
}

// create handles POST /api/tasks/{id}/worktree — the Retry (worktree_error
// set, D-25) and lazy-create (both NULL, D-26) paths. Idempotent: a task that
// already has a worktree gets 200 with the task as-is. Always 200 with the
// re-read task on a provisioning attempt — the client inspects worktree_error
// for the failed-state copy.
func (h *worktreeHandlers) create(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, repo, err := h.loadTaskRepo(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t.WorktreePath != nil {
		writeJSON(w, http.StatusOK, t) // already has one — idempotent
		return
	}
	// Slug recomputed from the CURRENT title; a kept branch from a previous
	// cleanup is reused inside Create (plan 03-01's reuse path).
	_, _, _ = provisionWorktree(r.Context(), h.db, h.wt, t.ID, t.Title, repo)
	t2, err := scanTask(h.db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t2)
}

// get handles GET /api/tasks/{id}/worktree — FRESH dialog state, computed at
// request time, never from cache (Pitfall 8): current dirty count and running
// session count alongside branch/path.
func (h *worktreeHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, _, err := h.loadTaskRepo(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t.WorktreePath == nil {
		writeError(w, http.StatusNotFound, "task has no worktree")
		return
	}
	path := *t.WorktreePath
	dirty := 0
	// A manually deleted dir is "clean": Remove self-heals the bookkeeping
	// (verified on git 2.43), so a missing tree must never block cleanup.
	if _, statErr := os.Stat(path); statErr == nil {
		dirty, err = h.wt.DirtyCount(r.Context(), path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	branch := ""
	if t.Branch != nil {
		branch = *t.Branch
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"branch":      branch,
		"path":        path,
		"dirty_files": dirty,
		// One honest number, no tmux-specific field (D-92/D-77): in-memory
		// running sessions PLUS live detached tmux survivors of this task.
		"running_sessions": h.cleanupSessionCount(r.Context(), id),
	})
}

// remove handles DELETE /api/tasks/{id}/worktree. Both gates are re-checked
// AT DELETE TIME — never trusting the dialog's snapshot (Pitfall 8). The
// ordering is load-bearing: StopAllForTask blocks through the SIGTERM grace
// BEFORE Remove runs, because git gives running sessions no protection at all
// (Pitfall 4). The branch always survives (D-34).
func (h *worktreeHandlers) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		StopSessions bool `json:"stop_sessions"`
		Force        bool `json:"force"`
	}
	// Empty body = both false (the clean variant sends no flags).
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	t, repo, err := h.loadTaskRepo(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t.WorktreePath == nil {
		writeError(w, http.StatusNotFound, "task has no worktree")
		return
	}
	path := *t.WorktreePath

	// Delegate the gate+stop+kill+remove+null-columns core to the shared helper
	// (D-04). The handler keeps the HTTP-only logic: body parse, loadTaskRepo,
	// the 404s above, and mapping the helper's (removed, reason) back to the
	// SAME 409 strings / 500 / 204 it returned inline before. The session count
	// and live tmux names are probed HERE (handler-owned methods) and passed in.
	count := h.cleanupSessionCount(r.Context(), id)
	live := h.liveTmuxNames(r.Context(), id)
	removed, reason, err := cleanupWorktreeGated(
		r.Context(), h.db, h.wt, h.mgr, h.tmuxClient, live,
		id, repo, path, count, req.StopSessions, req.Force)
	if err != nil {
		// Dialog shows: "Couldn't remove the worktree: {git error}".
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !removed {
		// The two gate reasons map to the SAME 409 strings as before:
		// "sessions running" (D-32) / "worktree has uncommitted changes" (D-33).
		writeError(w, http.StatusConflict, reason)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

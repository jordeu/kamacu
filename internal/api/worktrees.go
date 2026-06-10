package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"kangent/internal/session"
	"kangent/internal/worktree"
)

// WorktreeRoutes registers the per-task worktree lifecycle endpoints. The
// DELETE gates (D-32 sessions, D-33 dirt) are enforced HERE, at request time:
// git itself happily removes a worktree with live processes cwd'd inside
// (Pitfall 4, verified) and the client dialog's snapshot can be stale
// (Pitfall 8), so the server is the only real safety net.
func WorktreeRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service, mgr *session.Manager) {
	h := &worktreeHandlers{db: db, wt: wt, mgr: mgr}
	mux.HandleFunc("POST /api/tasks/{id}/worktree", h.create)
	mux.HandleFunc("GET /api/tasks/{id}/worktree", h.get)
	mux.HandleFunc("DELETE /api/tasks/{id}/worktree", h.remove)
}

type worktreeHandlers struct {
	db  *sql.DB
	wt  *worktree.Service
	mgr *session.Manager
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
			&t.CreatedAt, &t.UpdatedAt, &t.Branch, &t.WorktreePath, &t.WorktreeError, &repo)
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
		"branch":           branch,
		"path":             path,
		"dirty_files":      dirty,
		"running_sessions": h.runningSessions(id),
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

	// Gate 1 (D-32): never silently kill sessions.
	if h.runningSessions(id) > 0 && !req.StopSessions {
		writeError(w, http.StatusConflict, "sessions running")
		return
	}
	// Gate 2 (D-33): never silently destroy uncommitted work. Missing dir
	// counts as clean (Remove self-heals; never block cleanup on it).
	dirty := 0
	if _, statErr := os.Stat(path); statErr == nil {
		dirty, err = h.wt.DirtyCount(r.Context(), path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if dirty > 0 && !req.Force {
		writeError(w, http.StatusConflict, "worktree has uncommitted changes")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// Stop FIRST and block through the grace — this ordering IS the GIT-03
	// refuse-while-running enforcement (git removes trees under live cwds
	// without error, Pitfall 4).
	h.mgr.StopAllForTask(id)
	// Plain remove when clean; --force only when the user passed the dirty
	// gate. The clean-but-submodules --force fallback lives inside Remove.
	if err := h.wt.Remove(ctx, repo, path, dirty > 0); err != nil {
		// Dialog shows: "Couldn't remove the worktree: {git error}".
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Task lands in the D-26 absent state; the git branch itself survives (D-34).
	if _, err := h.db.Exec(
		`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

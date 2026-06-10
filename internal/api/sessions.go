package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"kangent/internal/session"
)

// SessionRoutes registers terminal session endpoints on mux. Sessions remain
// memory-only this phase; db is consulted only to resolve a task's worktree
// path when a spawn is task-scoped (TERM-04).
func SessionRoutes(mux *http.ServeMux, mgr *session.Manager, db *sql.DB) {
	s := &sessionHandlers{mgr: mgr, db: db}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
}

type sessionHandlers struct {
	mgr *session.Manager
	db  *sql.DB
}

// list handles GET /api/sessions — newest first, JSON [] when empty.
// ?task_id=N filters to that task's sessions (running AND exited — the
// exited-ghost handling is client-side per D-28).
func (h *sessionHandlers) list(w http.ResponseWriter, r *http.Request) {
	var infos []session.Info
	if q := r.URL.Query().Get("task_id"); q == "" {
		infos = h.mgr.List()
	} else {
		id, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task_id")
			return
		}
		infos = h.mgr.ListByTask(id)
	}
	if infos == nil {
		infos = []session.Info{}
	}
	writeJSON(w, http.StatusOK, infos)
}

// create handles POST /api/sessions — spawns a bash session, or with
// {"kind":"agent"} the task's Claude Code agent session (TERM-01). An
// optional {"task_id":N} body scopes the session to a task: it spawns in the
// task's worktree (TERM-04) with the per-task "Bash N" label. An empty body
// (the /terminal dev route sends none) spawns an unscoped dev session exactly
// as before. Agents require a task worktree and are limited to ONE running
// per task (D-38) — the API enforces both.
func (h *sessionHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64  `json:"task_id"`
		Kind   string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind := session.KindBash
	switch req.Kind {
	case "", "bash":
	case "agent":
		kind = session.KindAgent
	default:
		writeError(w, http.StatusBadRequest, "invalid kind")
		return
	}
	// Agents always run in a task worktree: no task means no worktree — the
	// same gate (and copy) as a worktree-less task.
	if kind == session.KindAgent && req.TaskID <= 0 {
		writeError(w, http.StatusConflict, "task has no worktree")
		return
	}
	opts := session.SpawnOpts{Kind: kind}
	if req.TaskID > 0 {
		var path sql.NullString
		err := h.db.QueryRow(`SELECT worktree_path FROM tasks WHERE id = ?`, req.TaskID).Scan(&path)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !path.Valid {
			writeError(w, http.StatusConflict, "task has no worktree") // D-30 server side
			return
		}
		opts.Cwd, opts.TaskID = path.String, req.TaskID
	}
	// One-agent-per-task gate (D-38), checked BEFORE spawning. An EXITED
	// agent never blocks — that is the "Start again" path (D-41).
	if kind == session.KindAgent {
		for _, info := range h.mgr.ListByTask(req.TaskID) {
			if info.Kind == session.KindAgent && info.Status == session.StatusRunning {
				writeError(w, http.StatusConflict, "agent session already running")
				return
			}
		}
	}
	// Spawn's stat pre-check covers a vanished worktree dir → same 500 path.
	sess, err := h.mgr.Spawn(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "couldn't start a session")
		return
	}
	// Persist the Phase 5 --resume key BEFORE replying (latest spawn wins).
	// A write failure degrades Phase 5 resume only — the session is usable.
	if kind == session.KindAgent {
		if _, err := h.db.Exec(`UPDATE tasks SET claude_session_id = ? WHERE id = ?`, sess.ClaudeSessionID(), req.TaskID); err != nil {
			slog.Warn("persisting claude_session_id", "task", req.TaskID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, sess.Info())
}

// stop handles POST /api/sessions/{id}/stop. Stop blocks up to the 5s
// SIGTERM grace (D-14), so it runs in a goroutine and the reply is 202
// immediately. Idempotent: stopping an already-exited session is a no-op
// that still returns 202.
func (h *sessionHandlers) stop(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	go sess.Stop()
	w.WriteHeader(http.StatusAccepted)
}

// delete handles DELETE /api/sessions/{id} — removes an EXITED session,
// freeing its replay ring. Running sessions must be stopped first.
func (h *sessionHandlers) delete(w http.ResponseWriter, r *http.Request) {
	err := h.mgr.Remove(r.PathValue("id"))
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, session.ErrStillRunning):
		writeError(w, http.StatusConflict, "session is still running. Stop it first.")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

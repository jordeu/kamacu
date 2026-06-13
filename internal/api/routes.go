package api

import (
	"database/sql"
	"net/http"

	"kangent/internal/session"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
)

// Routes registers all REST API endpoints on mux. wt provisions per-task
// worktrees during task creation (GIT-01); mgr lets task deletion stop the
// task's sessions before the row goes (D-42's one consistent rule).
// tmuxClient lets task deletion kill the task's live tmux sessions before the
// row delete (D-93) — StopAllForTask only reaches in-memory sessions, so a
// detached tmux survivor needs a direct kill.
func Routes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service, mgr *session.Manager, tmuxClient tmux.Client) {
	p := &projectHandlers{db: db}
	t := &taskHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/projects", p.list)
	mux.HandleFunc("POST /api/projects", p.create)
	mux.HandleFunc("PATCH /api/projects/{id}", p.update)
	mux.HandleFunc("GET /api/projects/{id}/github-origin", p.githubOrigin)
	mux.HandleFunc("GET /api/github/status", githubStatus)
	mux.HandleFunc("DELETE /api/projects/{id}", p.delete)
	mux.HandleFunc("GET /api/projects/{id}/tasks", t.listByProject)
	mux.HandleFunc("POST /api/projects/{id}/tasks", t.create)
	mux.HandleFunc("GET /api/tasks/{id}", t.get)
	mux.HandleFunc("PATCH /api/tasks/{id}", t.update)
	mux.HandleFunc("POST /api/tasks/{id}/move", t.move)
	mux.HandleFunc("DELETE /api/tasks/{id}", t.delete)
	SettingsRoutes(mux, db)
}

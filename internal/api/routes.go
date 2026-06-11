package api

import (
	"database/sql"
	"net/http"

	"kangent/internal/session"
	"kangent/internal/worktree"
)

// Routes registers all REST API endpoints on mux. wt provisions per-task
// worktrees during task creation (GIT-01); mgr lets task deletion stop the
// task's sessions before the row goes (D-42's one consistent rule).
func Routes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service, mgr *session.Manager) {
	p := &projectHandlers{db: db}
	t := &taskHandlers{db: db, wt: wt, mgr: mgr}
	mux.HandleFunc("GET /api/projects", p.list)
	mux.HandleFunc("POST /api/projects", p.create)
	mux.HandleFunc("PATCH /api/projects/{id}", p.update)
	mux.HandleFunc("DELETE /api/projects/{id}", p.delete)
	mux.HandleFunc("GET /api/projects/{id}/tasks", t.listByProject)
	mux.HandleFunc("POST /api/projects/{id}/tasks", t.create)
	mux.HandleFunc("GET /api/tasks/{id}", t.get)
	mux.HandleFunc("PATCH /api/tasks/{id}", t.update)
	mux.HandleFunc("POST /api/tasks/{id}/move", t.move)
	mux.HandleFunc("DELETE /api/tasks/{id}", t.delete)
	SettingsRoutes(mux, db)
}

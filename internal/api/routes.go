package api

import (
	"database/sql"
	"net/http"
)

// Routes registers all REST API endpoints on mux.
func Routes(mux *http.ServeMux, db *sql.DB) {
	p := &projectHandlers{db: db}
	t := &taskHandlers{db: db}
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
}

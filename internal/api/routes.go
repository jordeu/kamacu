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

// taskHandlers stubs — replaced by the real implementation in tasks.go
// (plan 01-02 Tasks 2 and 3). Each returns 501 until implemented.
type taskHandlers struct{ db *sql.DB }

func (t *taskHandlers) notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}

func (t *taskHandlers) listByProject(w http.ResponseWriter, r *http.Request) { t.notImplemented(w, r) }
func (t *taskHandlers) create(w http.ResponseWriter, r *http.Request)        { t.notImplemented(w, r) }
func (t *taskHandlers) get(w http.ResponseWriter, r *http.Request)           { t.notImplemented(w, r) }
func (t *taskHandlers) update(w http.ResponseWriter, r *http.Request)        { t.notImplemented(w, r) }
func (t *taskHandlers) move(w http.ResponseWriter, r *http.Request)          { t.notImplemented(w, r) }
func (t *taskHandlers) delete(w http.ResponseWriter, r *http.Request)        { t.notImplemented(w, r) }

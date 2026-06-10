package api

import (
	"errors"
	"net/http"

	"kangent/internal/session"
)

// SessionRoutes registers terminal session endpoints on mux. Sessions are
// memory-only this phase and are served by a *session.Manager rather than
// *sql.DB, so they get their own registration function alongside Routes.
func SessionRoutes(mux *http.ServeMux, mgr *session.Manager) {
	s := &sessionHandlers{mgr: mgr}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
}

type sessionHandlers struct{ mgr *session.Manager }

// list handles GET /api/sessions — newest first, JSON [] when empty.
func (h *sessionHandlers) list(w http.ResponseWriter, r *http.Request) {
	infos := h.mgr.List()
	if infos == nil {
		infos = []session.Info{}
	}
	writeJSON(w, http.StatusOK, infos)
}

// create handles POST /api/sessions — spawns a bash session.
func (h *sessionHandlers) create(w http.ResponseWriter, r *http.Request) {
	sess, err := h.mgr.Spawn()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "couldn't start a session")
		return
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

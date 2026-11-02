package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"kamacu/internal/notifications"
)

func NotificationRoutes(mux *http.ServeMux, service *notifications.Service) {

	mux.HandleFunc("POST /api/notifications/messages/{id}/continue", func(w http.ResponseWriter, r *http.Request) {
		q, err := service.Inbox.ContinueNotification(r.Context(), r.PathValue("id"))
		if err != nil {
			inboxError(w, err)
			return
		}
		writeJSON(w, 201, q)
	})
	mux.HandleFunc("GET /api/notifications/sources", func(w http.ResponseWriter, r *http.Request) {
		sources, err := service.Sources(r.Context())
		if err != nil {
			writeError(w, 500, "cannot load notification sources")
			return
		}
		writeJSON(w, 200, sources)
	})
	mux.HandleFunc("POST /api/notifications/sources", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string          `json:"provider"`
			Name     string          `json:"name"`
			Config   json.RawMessage `json:"config"`
		}
		if !inboxDecode(w, r, &body) {
			return
		}
		source, err := service.AddSource(r.Context(), body.Provider, body.Name, body.Config)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, source)
	})
	mux.HandleFunc("DELETE /api/notifications/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := service.RemoveSource(r.Context(), r.PathValue("id"))
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "notification source not found")
			return
		}
		if err != nil {
			writeError(w, 500, "cannot remove notification source")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PATCH /api/notifications/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !inboxDecode(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			writeError(w, 400, "enabled is required")
			return
		}
		err := service.EnableSource(r.Context(), r.PathValue("id"), *body.Enabled)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "notification source not found")
			return
		}
		if err != nil {
			writeError(w, 500, "cannot update notification source")
			return
		}
		writeJSON(w, 200, map[string]bool{"enabled": *body.Enabled})
	})
}

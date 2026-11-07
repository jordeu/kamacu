package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"kamacu/internal/notifications"
)

func NotificationRoutes(mux *http.ServeMux, service *notifications.Service) {
	notificationRuleRoutes(mux, service)

	mux.HandleFunc("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}
		if status != "pending" && status != "archived" {
			writeError(w, 400, "status must be pending or archived")
			return
		}
		offset := 0
		var err error
		if raw := r.URL.Query().Get("offset"); raw != "" {
			offset, err = strconv.Atoi(raw)
			if err != nil || offset < 0 {
				writeError(w, 400, "invalid offset")
				return
			}
		}
		page, err := service.ListRaw(r.Context(), status, offset)
		if err != nil {
			writeError(w, 500, "cannot load notifications")
			return
		}
		writeJSON(w, 200, page)
	})
	mux.HandleFunc("GET /api/notifications/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := service.Status(r.Context())
		if err != nil {
			writeError(w, 500, "cannot load notification status")
			return
		}
		writeJSON(w, 200, status)
	})
	mux.HandleFunc("POST /api/notifications/process", func(w http.ResponseWriter, r *http.Request) {
		process, err := service.RequestProcessing(r.Context())
		if errors.Is(err, notifications.ErrNoPending) || errors.Is(err, notifications.ErrNoActiveRules) {
			writeError(w, 409, err.Error())
			return
		}
		if err != nil {
			writeError(w, 500, "cannot start notification processing")
			return
		}
		writeJSON(w, 202, process)
	})
	mux.HandleFunc("PATCH /api/notifications/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeError(w, 400, "invalid notification id")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		if !inboxDecode(w, r, &body) {
			return
		}
		if body.Status != "pending" && body.Status != "archived" {
			writeError(w, 400, "status must be pending or archived")
			return
		}
		err = service.SetRawStatus(r.Context(), id, body.Status)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "notification not found")
			return
		}
		if errors.Is(err, notifications.ErrProcessing) {
			writeError(w, 409, err.Error())
			return
		}
		if err != nil {
			writeError(w, 500, "cannot update notification")
			return
		}
		writeJSON(w, 200, map[string]string{"status": body.Status})
	})

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

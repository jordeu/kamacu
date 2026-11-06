package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"kamacu/internal/notifications"
	"net/http"
)

func notificationRuleRoutes(mux *http.ServeMux, s *notifications.Service) {
	mux.HandleFunc("GET /api/notifications/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		set, err := s.RuleSet(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, set)
	})
	mux.HandleFunc("GET /api/notifications/rules", func(w http.ResponseWriter, r *http.Request) {
		sets, err := s.RuleSets(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, sets)
	})
	mux.HandleFunc("POST /api/notifications/rules/drafts", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Base  int64                            `json:"base_revision"`
			Rules []notifications.NotificationRule `json:"rules"`
		}
		if !ruleDecode(w, r, &body) {
			return
		}
		set, err := s.SaveRuleDraft(r.Context(), body.Base, body.Rules)
		if err != nil {
			status := 400
			if errors.Is(err, notifications.ErrRulesChanged) {
				status = 409
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, 201, set)
	})
	mux.HandleFunc("POST /api/notifications/rules/{id}/compile", func(w http.ResponseWriter, r *http.Request) {
		set, err := s.QueueRuleCompilation(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 202, set)
	})
	mux.HandleFunc("POST /api/notifications/rules/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
		set, err := s.ActivateRules(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, set)
	})
	mux.HandleFunc("POST /api/notifications/rules/previews", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SetID string  `json:"rule_set_id"`
			IDs   []int64 `json:"event_ids"`
		}
		if !ruleDecode(w, r, &body) {
			return
		}
		p, err := s.QueueRulePreview(r.Context(), body.SetID, body.IDs)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 202, p)
	})
	mux.HandleFunc("GET /api/notifications/rules/previews/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.RulePreview(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /api/notifications/retry-held", func(w http.ResponseWriter, r *http.Request) {
		n, err := s.RetryHeld(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]int64{"retried": n})
	})
}

func ruleDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		writeError(w, 400, "rule request exceeds limits")
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		writeError(w, 400, "rule request must be a JSON object")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		writeError(w, 400, "invalid rule request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		writeError(w, 400, "invalid trailing rule request data")
		return false
	}
	return true
}

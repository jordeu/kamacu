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
	mux.HandleFunc("GET /api/notifications/usage", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Metrics.Snapshot()) })
	fail := func(w http.ResponseWriter, err error) {
		code := 400
		if errors.Is(err, notifications.ErrRulesChanged) {
			code = 409
		}
		writeError(w, code, err.Error())
	}
	mux.HandleFunc("GET /api/notifications/config", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Config(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("PUT /api/notifications/config", func(w http.ResponseWriter, r *http.Request) {
		var c notifications.Config
		if !ruleDecode(w, r, &c) {
			return
		}
		c, err := s.SaveConfig(r.Context(), c)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("POST /api/notifications/config/test", func(w http.ResponseWriter, r *http.Request) {
		err := s.TestModel(r.Context())
		if err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/notifications/rules", func(w http.ResponseWriter, r *http.Request) {
		sets, err := s.RuleSets(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, sets)
	})
	mux.HandleFunc("GET /api/notifications/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		set, err := s.RuleSet(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, set)
	})
	mux.HandleFunc("PUT /api/notifications/rules", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Base  int64                            `json:"base_revision"`
			Rules []notifications.NotificationRule `json:"rules"`
		}
		if !ruleDecode(w, r, &body) {
			return
		}
		set, err := s.SaveRules(r.Context(), body.Base, body.Rules)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, set)
	})
	mux.HandleFunc("POST /api/notifications/rules/improvements", func(w http.ResponseWriter, r *http.Request) {
		var req notifications.RuleImprovementRequest
		if !ruleDecode(w, r, &req) {
			return
		}
		out, err := s.ImproveRules(r.Context(), req)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /api/notifications/rules/previews", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID     string  `json:"rule_set_id"`
			Events []int64 `json:"event_ids"`
		}
		if !ruleDecode(w, r, &body) {
			return
		}
		out, err := s.QueueRulePreview(r.Context(), body.ID, body.Events)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 202, out)
	})
	mux.HandleFunc("GET /api/notifications/rules/previews/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.RulePreview(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /api/notifications/batches/{id}", func(w http.ResponseWriter, r *http.Request) {
		var snapshot, result, config, trace, status string
		input := `'{}'`
		if r.URL.Query().Get("input") == "1" {
			input = "processor_snapshot"
		}
		err := s.DB.QueryRowContext(r.Context(), `SELECT `+input+`,processor_result,processor_config,processor_trace,status FROM notification_batches WHERE id=?`, r.PathValue("id")).Scan(&snapshot, &result, &config, &trace, &status)
		if err != nil {
			writeError(w, 404, "batch not found")
			return
		}
		writeJSON(w, 200, map[string]any{"status": status, "snapshot": json.RawMessage(snapshot), "result": json.RawMessage(result), "config": json.RawMessage(config), "reads": json.RawMessage(trace)})
	})
}
func ruleDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	// Up to 100 rules of 8000 bytes each, allowing JSON escape expansion.
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 6<<20))
	if err != nil {
		writeError(w, 400, "request exceeds limits")
		return false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		writeError(w, 400, "request must be an object")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		writeError(w, 400, "invalid request")
		return false
	}
	return true
}

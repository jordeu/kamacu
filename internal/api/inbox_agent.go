package api

import (
	"errors"
	"net/http"

	"kamacu/internal/inboxagent"
)

func InboxAgentRoutes(mux *http.ServeMux, s *inboxagent.Service) {
	fail := func(w http.ResponseWriter, err error) {
		code := http.StatusBadRequest
		if errors.Is(err, inboxagent.ErrChanged) {
			code = http.StatusConflict
		}
		writeError(w, code, err.Error())
	}
	mux.HandleFunc("GET /api/inbox-agent/config", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Config(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("PUT /api/inbox-agent/config", func(w http.ResponseWriter, r *http.Request) {
		var c inboxagent.Config
		if !ruleDecode(w, r, &c) {
			return
		}
		out, err := s.SaveConfig(r.Context(), c)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /api/inbox-agent/config/test", func(w http.ResponseWriter, r *http.Request) {
		if err := s.TestModel(r.Context()); err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/inbox-agent/usage", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Metrics.Snapshot()) })
}

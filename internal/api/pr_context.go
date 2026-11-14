package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"kamacu/internal/github"
	"kamacu/internal/notifications"
	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

var readPRFacts = func(ctx context.Context, repo string, n int64) notifications.PRFacts {
	return (notifications.GitHub{}).PRContext(ctx, notifications.Source{}, repo, n)
}

type prContextHandlers struct {
	db       *sql.DB
	mgr      *session.Manager
	statuses *github.PRStatusService
}

func (h prContextHandlers) task(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	links, err := store.PRLinks(r.Context(), h.db, id, "", 0)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	work, err := store.WorkForTask(r.Context(), h.db, id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	var sessions []session.Info
	if h.mgr != nil {
		sessions = h.mgr.ListByTask(id)
	}
	writeJSON(w, 200, map[string]any{"task_id": id, "links": links, "observed_work": work, "live_sessions": sessions, "pr_lookup": "Use get_pr_context for current verified GitHub facts for each linked PR. Observations may be stale; live sessions report actual process state."})
}
func (h prContextHandlers) correct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var l store.PRLink
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&l); err != nil {
		writeError(w, 400, "invalid link")
		return
	}
	l.TaskID = id
	l.Corrected = true
	l.Source = "explicit"
	l.ObservedAt = time.Now()
	if err := store.PutPRLink(r.Context(), h.db, l); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	h.task(w, r)
}
func (h prContextHandlers) pr(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.ParseInt(r.URL.Query().Get("number"), 10, 64)
	repo, n, err := store.PRIdentity(r.URL.Query().Get("repository"), n)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	related, err := store.RelatedPRWork(r.Context(), h.db, repo, n)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	observedSessions, err := store.RelatedPRSessions(r.Context(), h.db, repo, n)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	live := []session.Info{}
	if h.mgr != nil {
		for _, w := range observedSessions {
			if sess, ok := h.mgr.Get(w.SessionID); ok {
				live = append(live, sess.Info())
			}
		}
		for _, t := range related {
			live = append(live, h.mgr.ListByTask(t.Link.TaskID)...)
		}
	}
	facts := notifications.PRFacts{Repository: repo, Number: n, Stale: true, Error: "GitHub integration disabled", Source: "github_api"}
	enabled, e := settings.Get(h.db, settings.KeyGithubIntegration)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	if enabled == "on" {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		facts = readPRFacts(ctx, repo, n)
	}
	writeJSON(w, 200, map[string]any{"github": facts, "related_work": related, "related_sessions": observedSessions, "live_sessions": live, "observation_warning": "Terminal observations are unverified. A running process alone does not imply active work; inspect its monitor state, observation time and terminal evidence. Historical sessions may have ended."})
}
func prContextRoutes(mux *http.ServeMux, db *sql.DB, mgr *session.Manager) {
	h := prContextHandlers{db: db, mgr: mgr, statuses: github.NewPRStatusService(nil, nil)}
	mux.HandleFunc("GET /api/projects/{id}/task-pr-statuses", h.projectStatuses)
	mux.HandleFunc("GET /api/projects/{id}/pull-requests/{n}/status", h.pullRequestStatus)
	mux.HandleFunc("GET /api/pr-context", h.pr)
	mux.HandleFunc("GET /api/tasks/{id}/pr-context", h.task)
	mux.HandleFunc("PUT /api/tasks/{id}/pr-links", h.correct)
}

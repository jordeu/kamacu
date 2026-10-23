package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"kamacu/internal/inbox"
)

type inboxHandlers struct {
	service *inbox.Service
	token   string
	stop    func(string)
}

func InboxRoutes(mux *http.ServeMux, service *inbox.Service, token string, stop ...func(string)) {
	h := &inboxHandlers{service: service, token: token}
	if len(stop) > 0 {
		h.stop = stop[0]
	}
	mux.HandleFunc("GET /api/inbox", h.list)
	mux.HandleFunc("POST /api/inbox/{id}/answer", h.answer)
	mux.HandleFunc("POST /api/inbox/{id}/dismiss", h.dismiss)
	mux.HandleFunc("POST /api/agent-feedback/{action}", h.agent)
}
func inboxError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "could not update inbox"
	switch {
	case errors.Is(err, inbox.ErrInvalid):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, inbox.ErrIdentity), errors.Is(err, inbox.ErrOwner):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, inbox.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, inbox.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	}
	if status == http.StatusInternalServerError {
		slog.Error("inbox request failed", "error", err)
	}
	writeError(w, status, message)
}
func inboxDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 96<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid inbox request body")
		return false
	}
	return true
}
func (h *inboxHandlers) list(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view != "" && view != "pending" && view != "archived" {
		writeError(w, 400, "view must be pending or archived")
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, 400, "invalid offset")
			return
		}
	}
	page, err := h.service.List(r.Context(), nil, view == "archived", offset)
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

type inboxAnswer struct {
	Revision int64  `json:"revision"`
	Answer   string `json:"answer"`
	OptionID string `json:"option_id"`
}

func (h *inboxHandlers) answer(w http.ResponseWriter, r *http.Request) {
	var body inboxAnswer
	if !inboxDecode(w, r, &body) {
		return
	}
	out, err := h.service.Answer(r.Context(), nil, r.PathValue("id"), body.Revision, body.Answer, body.OptionID)
	if err != nil {
		inboxError(w, err)
		return
	}
	// Only the winning explicit exit option can stop the originating session.
	h.stopCompleted(out)
	writeJSON(w, 200, out)
}
func (h *inboxHandlers) stopCompleted(out inbox.Result) {
	if h.stop != nil && out.Outcome != "stale_revision" && out.Question.Kind == "completion" && out.Question.Status == "answered" && out.Question.Reason == "exit" {
		h.stop(out.Question.SessionID)
	}
}
func (h *inboxHandlers) dismiss(w http.ResponseWriter, r *http.Request) {
	var body inboxAnswer
	if !inboxDecode(w, r, &body) {
		return
	}
	out, err := h.service.Dismiss(r.Context(), r.PathValue("id"), body.Revision)
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *inboxHandlers) agent(w http.ResponseWriter, r *http.Request) {
	if h.token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Kamacu-Token")), []byte(h.token)) != 1 {
		writeError(w, 401, "unauthorized")
		return
	}
	// Identity belongs to the MCP process environment, never model arguments.
	taskID, identityErr := strconv.ParseInt(r.Header.Get("X-Kamacu-Task-ID"), 10, 64)
	if identityErr != nil {
		inboxError(w, inbox.ErrIdentity)
		return
	}
	who := inbox.Identity{TaskID: taskID, SessionID: r.Header.Get("X-Kamacu-Session-ID")}
	var body struct {
		inbox.Create
		inboxAnswer
		QuestionID    string `json:"question_id"`
		WaitSeconds   *int   `json:"wait_seconds"`
		UserRequested bool   `json:"user_requested"`
		Offset        int    `json:"offset"`
	}
	if !inboxDecode(w, r, &body) {
		return
	}
	var out any
	var err error
	switch r.PathValue("action") {
	case "complete":
		out, err = h.service.Complete(r.Context(), who, body.Create)
	case "ask":
		out, err = h.service.Create(r.Context(), who, body.Create)
	case "wait":
		seconds := 20
		if body.WaitSeconds != nil {
			seconds = *body.WaitSeconds
		}
		if seconds < 0 || seconds > 20 {
			writeError(w, 400, "wait_seconds must be between 0 and 20")
			return
		}
		out, err = h.service.Wait(r.Context(), who, body.QuestionID, time.Duration(seconds)*time.Second)
	case "resolve":
		out, err = h.service.Answer(r.Context(), &who, body.QuestionID, body.Revision, body.Answer, body.OptionID)
		if err == nil {
			h.stopCompleted(out.(inbox.Result))
		}
	case "reopen":
		out, err = h.service.Reopen(r.Context(), who, body.QuestionID, body.Revision, body.UserRequested)
	case "acknowledge":
		out, err = h.service.Acknowledge(r.Context(), who, body.QuestionID, body.Revision)
	case "list":
		if body.Offset < 0 {
			writeError(w, 400, "invalid offset")
			return
		}
		out, err = h.service.List(r.Context(), &who, false, body.Offset)
	default:
		writeError(w, 404, "unknown inbox action")
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			inboxError(w, err)
		}
		return
	}
	writeJSON(w, 200, out)
}

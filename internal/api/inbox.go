package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/session"
)

type inboxHandlers struct {
	service *inbox.Service
	token   string
	stop    func(string)
	deliver func(inbox.Question) error
}

func InboxRoutes(mux *http.ServeMux, service *inbox.Service, token string, stop ...func(string)) {
	inboxRoutes(mux, service, token, nil, stop...)
}

// InboxTerminalRoutes delivers browser answers directly to their originating agent.
func InboxTerminalRoutes(mux *http.ServeMux, service *inbox.Service, token string, mgr *session.Manager, stop func(string)) inbox.Responder {
	h := inboxRoutes(mux, service, token, func(q inbox.Question) error {
		sess, ok := mgr.Get(q.SessionID)
		if !ok {
			return fmt.Errorf("originating agent is unavailable")
		}
		info := sess.Info()
		if info.Kind != session.KindAgent || info.Status != session.StatusRunning || info.StopRequested || info.TaskID != q.TaskID || info.Global != (q.TaskID == 0) {
			return fmt.Errorf("originating agent is unavailable")
		}
		if q.Origin == "observer" {
			if q.DeliveryMode == "claude_question" {
				return sess.WriteObservedChoice(q.EvidenceRevision, q.InputGeneration, q.Answer)
			}
			body, submit := wrapInputForWrite(q.Answer)
			return sess.WriteObserved(q.EvidenceRevision, q.InputGeneration, []byte(body), []byte(submit))
		}
		return sendTerminalMessage(sess, feedbackMessage(q))
	}, stop)
	return h.respond
}

func feedbackMessage(q inbox.Question) string {
	// JSON escapes terminal control characters, including bracketed-paste delimiters.
	payload, _ := json.Marshal(struct {
		ID       string `json:"question_id"`
		Revision int64  `json:"revision"`
		Answer   string `json:"answer"`
		Reason   string `json:"reason"`
	}{q.ID, q.Revision, q.Answer, q.Reason})
	return "Kamacu inbox answer: " + string(payload) + "\nCall acknowledge_feedback with this question_id and revision before acting, and use the returned answer. For reason=seen, end your turn without further work or another completion report."
}

func inboxRoutes(mux *http.ServeMux, service *inbox.Service, token string, deliver func(inbox.Question) error, stop ...func(string)) *inboxHandlers {
	h := &inboxHandlers{service: service, token: token, deliver: deliver}
	if len(stop) > 0 {
		h.stop = stop[0]
	}
	mux.HandleFunc("GET /api/inbox", h.list)
	mux.HandleFunc("GET /api/inbox/tasks/{taskID}/completions", h.pendingCompletions)
	mux.HandleFunc("GET /api/inbox/catch-up", h.catchUp)
	mux.HandleFunc("GET /api/inbox/{id}/history", h.history)
	mux.HandleFunc("GET /api/inbox/{id}", h.get)
	mux.HandleFunc("POST /api/inbox/{id}/answer", h.answer)
	mux.HandleFunc("POST /api/inbox/{id}/dismiss", h.dismiss)
	mux.HandleFunc("POST /api/inbox/{id}/requeue", h.requeue)
	mux.HandleFunc("POST /api/agent-feedback/{action}", h.agent)
	return h
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
	order := r.URL.Query().Get("order")
	if order != "" && order != "newest" && order != "oldest" {
		writeError(w, 400, "order must be newest or oldest")
		return
	}
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
	page, err := h.service.ListWithOrder(r.Context(), nil, view == "archived", offset, order == "oldest" || (order == "" && view != "archived"))
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

func (h *inboxHandlers) get(w http.ResponseWriter, r *http.Request) {
	q, err := h.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
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
	out, err := h.respond(r.Context(), inbox.Response{ID: r.PathValue("id"), Revision: body.Revision, Answer: body.Answer, OptionID: body.OptionID}, nil)
	if err != nil {
		if out.Outcome == "accepted" {
			writeError(w, http.StatusBadGateway, err.Error())
		} else {
			inboxError(w, err)
		}
		return
	}
	writeJSON(w, 200, out)
}

// respond is the single delivery path for user and rule-authorized replies.
func (h *inboxHandlers) respond(ctx context.Context, req inbox.Response, guard inbox.AnswerGuard) (inbox.Result, error) {
	var out inbox.Result
	var err error
	if guard == nil {
		out, err = h.service.Answer(ctx, nil, req.ID, req.Revision, req.Answer, req.OptionID)
	} else {
		out, err = h.service.AnswerAutomatic(ctx, req, guard)
	}
	if err != nil {
		return out, err
	}
	deliveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if out.Outcome == "accepted" && out.Question.Origin != "notification" && out.Question.Reason != "exit" && inbox.CompletionTarget(out.Question.Reason) == "" && h.deliver != nil {
		if out.Question.Origin == "observer" {
			if err := h.service.RecordDelivery(deliveryCtx, out.Question.ID, "attempting"); err != nil {
				return out, err
			}
		}
		if err := h.deliver(out.Question); err != nil {
			if out.Question.Origin == "observer" {
				_ = h.service.RecordDelivery(deliveryCtx, out.Question.ID, "uncertain")
			}
			slog.Error("deliver inbox answer", "question", out.Question.ID, "error", err)
			return out, fmt.Errorf("Answer saved, but terminal delivery could not be confirmed. Open the terminal and review the saved answer before submitting it manually.")
		}
		if out.Question.Origin == "observer" {
			if err := h.service.RecordDelivery(deliveryCtx, out.Question.ID, "submitted"); err != nil {
				return out, err
			}
			out.Question.DeliveryStatus = "submitted"
		}
	}
	h.stopCompleted(out)
	return out, nil
}

func (h *inboxHandlers) stopCompleted(out inbox.Result) {
	if h.stop != nil && out.Outcome != "stale_revision" && out.Question.Kind == "completion" && out.Question.Status == "answered" && (out.Question.Reason == "exit" || inbox.CompletionTarget(out.Question.Reason) != "") {
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
func (h *inboxHandlers) requeue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Revision int64 `json:"revision"`
	}
	if !inboxDecode(w, r, &body) {
		return
	}
	out, err := h.service.Requeue(r.Context(), r.PathValue("id"), body.Revision)
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
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

func (h *inboxHandlers) pendingCompletions(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(r.PathValue("taskID"), 10, 64)
	if err != nil || taskID < 0 {
		writeError(w, http.StatusBadRequest, "invalid task ID")
		return
	}
	items, err := h.service.PendingCompletions(r.Context(), taskID)
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *inboxHandlers) catchUp(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.CatchUp(r.Context())
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
func (h *inboxHandlers) history(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(w, 400, "invalid offset")
			return
		}
	}
	page, err := h.service.History(r.Context(), r.PathValue("id"), offset)
	if err != nil {
		inboxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

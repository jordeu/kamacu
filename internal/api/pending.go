package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"kamacu/internal/session"
)

const pendingPrompt = "Check the current status of this task, tell me what is pending, and ask me what to do next. Wait for my answer before making changes."

type pendingFilter struct {
	Scope          string   `json:"scope"`
	ProjectID      int64    `json:"project_id"`
	WorkspaceID    int64    `json:"workspace_id"`
	Statuses       []string `json:"statuses"`
	TerminalStates []string `json:"terminal_states"`
}

func (f pendingFilter) valid() bool {
	if f.Scope != "project" && f.Scope != "workspace" && f.Scope != "anywhere" {
		return false
	}
	if f.Scope == "project" && f.ProjectID <= 0 || f.Scope == "workspace" && f.WorkspaceID <= 0 {
		return false
	}
	if len(f.Statuses) == 0 || len(f.TerminalStates) == 0 {
		return false
	}
	for _, s := range f.Statuses {
		if s != "in_review" && s != "in_progress" {
			return false
		}
	}
	for _, s := range f.TerminalStates {
		if s != "closed" && s != "active" {
			return false
		}
	}
	return true
}

type pendingItem struct {
	TaskID    int64  `json:"task_id"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	State     string `json:"state"`
	Detail    string `json:"detail,omitempty"`
}
type pendingJob struct {
	ID    string        `json:"id"`
	State string        `json:"state"`
	Items []pendingItem `json:"items"`
}
type pendingQueue struct {
	ctx      context.Context
	sessions *sessionHandlers
	mu       sync.Mutex
	job      pendingJob
	// One queue per server, shared by all tabs and scopes. A second run cannot
	// bypass either the concurrency limit or the startup spacing.
	interval     time.Duration
	poll         time.Duration
	nextStart    time.Time
	dispatchMu   sync.Mutex
	nextDispatch time.Time
}

func registerPendingRoutes(mux *http.ServeMux, sessions *sessionHandlers, ctx context.Context) {
	q := &pendingQueue{ctx: ctx, sessions: sessions, interval: 5 * time.Second, poll: time.Second, job: pendingJob{State: "idle", Items: []pendingItem{}}}
	mux.HandleFunc("POST /api/pending/preview", q.preview)
	mux.HandleFunc("POST /api/pending", q.start)
	mux.HandleFunc("GET /api/pending", q.status)
	mux.HandleFunc("POST /api/pending/cancel", q.cancel)
}

func decodePending(w http.ResponseWriter, r *http.Request) (pendingFilter, bool) {
	var f pendingFilter
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil || !f.valid() {
		writeError(w, 400, "select a valid scope, at least one card status and at least one terminal state")
		return f, false
	}
	return f, true
}

type pendingCandidate struct {
	pendingItem
	status, engine, conversation string
	worktree                     sql.NullString
	inbox                        bool
	live                         *session.Session
}

// One DB scan and one manager snapshot for previews, with a lazy transcript
// index shared across all cards. No per-card subprocesses or frontend fan-out.
func (h *sessionHandlers) pendingCandidates(f pendingFilter, taskID int64) ([]pendingCandidate, error) {
	rows, err := h.db.Query(`SELECT t.id, t.project_id, t.title, p.name, t.status, t.worktree_path, a.engine,
 CASE a.engine WHEN 'claude' THEN COALESCE(t.claude_session_id,'') WHEN 'codex' THEN COALESCE(t.codex_session_id,'') WHEN 'opencode' THEN COALESCE(t.opencode_session_id,'') ELSE '' END,
 EXISTS(SELECT 1 FROM inbox_questions q WHERE q.task_id=t.id AND q.status='pending')
 FROM tasks t JOIN projects p ON p.id=t.project_id JOIN agents a ON a.id=p.agent_id
 WHERE t.source='manual' AND t.snoozed_until IS NULL AND t.status IN ('in_review','in_progress')
 AND (?='anywhere' OR (?='project' AND t.project_id=?) OR (?='workspace' AND p.workspace_id=?))
 AND (?=0 OR t.id=?) ORDER BY t.id`, f.Scope, f.Scope, f.ProjectID, f.Scope, f.WorkspaceID, taskID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	live := map[int64]*session.Session{}
	for _, info := range h.mgr.List() {
		if info.Kind == session.KindAgent && !info.Global && info.Status == session.StatusRunning {
			if sess, ok := h.mgr.Get(info.ID); ok {
				live[info.TaskID] = sess
			}
		}
	}
	lookup := transcriptLookup(h.globRoot)
	if taskID > 0 {
		lookup = func(id string) bool { return transcriptExists(h.globRoot, id) }
	}
	out := []pendingCandidate{}
	for rows.Next() {
		var c pendingCandidate
		if err := rows.Scan(&c.TaskID, &c.ProjectID, &c.Title, &c.Project, &c.status, &c.worktree, &c.engine, &c.conversation, &c.inbox); err != nil {
			return nil, err
		}
		if !slices.Contains(f.Statuses, c.status) {
			continue
		}
		c.live = live[c.TaskID]
		c.State = "queued"
		if reason := c.reason(f, lookup); reason != "" {
			c.State = "skipped"
			c.Detail = reason
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (c pendingCandidate) reason(f pendingFilter, transcript func(string) bool) string {
	if c.live != nil {
		if !slices.Contains(f.TerminalStates, "active") {
			return "Active terminals excluded"
		}
		info := c.live.Info()
		if info.StopRequested {
			return "Agent is stopping"
		}
		if info.AgentStatus == "working" {
			return "Agent is working"
		}
		if info.AgentStatus == "waiting" && c.inbox {
			return "Agent already has a pending inbox message"
		}
		if info.AgentStatus == "error" {
			return "Agent needs attention"
		}
		return ""
	}
	if !slices.Contains(f.TerminalStates, "closed") {
		return "Closed terminals excluded"
	}
	if !c.worktree.Valid || c.worktree.String == "" {
		return "No worktree"
	}
	if c.conversation == "" {
		return "No previous conversation to resume"
	}
	if c.engine == "claude" && !transcript(c.conversation) {
		return "Previous conversation is unavailable"
	}
	return ""
}

func (h *sessionHandlers) pendingCandidate(f pendingFilter, taskID int64) (pendingCandidate, string, error) {
	candidates, err := h.pendingCandidates(f, taskID)
	if err != nil {
		return pendingCandidate{}, "", err
	}
	if len(candidates) == 0 {
		return pendingCandidate{}, "Card no longer matches the filters", nil
	}
	c := candidates[0]
	return c, c.Detail, nil
}

func (q *pendingQueue) preview(w http.ResponseWriter, r *http.Request) {
	f, ok := decodePending(w, r)
	if !ok {
		return
	}
	candidates, err := q.sessions.pendingCandidates(f, 0)
	if err != nil {
		writeError(w, 500, "could not find matching cards")
		return
	}
	eligible := 0
	for _, c := range candidates {
		if c.State == "queued" {
			eligible++
		}
	}
	writeJSON(w, 200, map[string]int{"eligible": eligible, "skipped": len(candidates) - eligible})
}
func (q *pendingQueue) start(w http.ResponseWriter, r *http.Request) {
	f, ok := decodePending(w, r)
	if !ok {
		return
	}
	q.mu.Lock()
	if q.job.State == "running" || q.job.State == "cancelling" {
		q.mu.Unlock()
		writeError(w, http.StatusConflict, "a status check is already running")
		return
	}
	candidates, err := q.sessions.pendingCandidates(f, 0)
	if err != nil {
		q.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "could not find matching cards")
		return
	}
	items := make([]pendingItem, len(candidates))
	for i, c := range candidates {
		items[i] = c.pendingItem
	}
	q.job = pendingJob{ID: uuid.NewString(), State: "running", Items: items}
	result := q.snapshotLocked()
	go q.run(func(i int, id int64) { q.check(f, i, id) })
	q.mu.Unlock()
	writeJSON(w, http.StatusAccepted, result)
}

// Copy the slice before unlocking; slow HTTP clients must not block workers or
// cancellation, and encoding a shared slice after unlocking would race updates.
func (q *pendingQueue) snapshotLocked() pendingJob {
	result := q.job
	result.Items = append([]pendingItem{}, q.job.Items...)
	return result
}
func (q *pendingQueue) status(w http.ResponseWriter, r *http.Request) {
	q.mu.Lock()
	result := q.snapshotLocked()
	q.mu.Unlock()
	writeJSON(w, http.StatusOK, result)
}
func (q *pendingQueue) cancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "a status check ID is required")
		return
	}
	q.mu.Lock()
	if req.ID != q.job.ID {
		q.mu.Unlock()
		writeError(w, http.StatusConflict, "the status check has changed; refresh its progress")
		return
	}
	if q.job.State == "running" {
		q.job.State = "cancelling"
		for i := range q.job.Items {
			if q.job.Items[i].State == "queued" {
				q.job.Items[i].State = "cancelled"
			}
		}
	}
	result := q.snapshotLocked()
	q.mu.Unlock()
	writeJSON(w, http.StatusOK, result)
}
func (q *pendingQueue) set(i int, state, detail string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.job.Items[i].State = state
	q.job.Items[i].Detail = detail
}
func (q *pendingQueue) cancelled() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.job.State == "cancelling" || q.ctx.Err() != nil
}

// The coordinator owns starts. Workers retain their slot until the prompted
// agent becomes idle/waiting/exited, rather than freeing it as soon as spawned.
func (q *pendingQueue) run(check func(int, int64)) {
	done := make(chan struct{}, 2)
	active := 0
	ticker := time.NewTicker(q.poll)
	defer ticker.Stop()
	for {
		q.mu.Lock()
		idx := -1
		if q.job.State == "running" && active < 2 && !time.Now().Before(q.nextStart) {
			for i, item := range q.job.Items {
				if item.State == "queued" {
					idx = i
					break
				}
			}
			if idx >= 0 {
				q.job.Items[idx].State = "checking"
				q.nextStart = time.Now().Add(q.interval)
			}
		}
		queued := false
		for _, item := range q.job.Items {
			if item.State == "queued" {
				queued = true
				break
			}
		}
		if idx < 0 && !queued && active == 0 {
			if q.job.State == "cancelling" {
				q.job.State = "cancelled"
			} else {
				q.job.State = "completed"
			}
			q.mu.Unlock()
			return
		}
		var taskID int64
		if idx >= 0 {
			taskID = q.job.Items[idx].TaskID
		}
		q.mu.Unlock()
		if idx >= 0 {
			active++
			go func(i int, id int64) { check(i, id); done <- struct{}{} }(idx, taskID)
		}
		select {
		case <-q.ctx.Done():
			return
		case <-done:
			active--
		case <-ticker.C:
		}
	}
}

// waitDispatch runs immediately before a real spawn or an active-terminal
// prompt. Scheduling alone cannot enforce spacing when startup locks delay
// several workers and then release them together.
func (q *pendingQueue) waitDispatch() bool {
	q.dispatchMu.Lock()
	defer q.dispatchMu.Unlock()
	for {
		if q.ctx.Err() != nil {
			return false
		}
		delay := time.Until(q.nextDispatch)
		if delay <= 0 {
			q.nextDispatch = time.Now().Add(q.interval)
			return true
		}
		timer := time.NewTimer(min(delay, 100*time.Millisecond))
		select {
		case <-q.ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (q *pendingQueue) check(f pendingFilter, i int, taskID int64) {
	if q.cancelled() {
		q.set(i, "cancelled", "")
		return
	}
	c, reason, err := q.sessions.pendingCandidate(f, taskID)
	if err != nil {
		q.set(i, "failed", "Could not check card")
		return
	}
	if reason != "" {
		q.set(i, "skipped", reason)
		return
	}
	sess := c.live
	reopened := sess == nil
	if reopened {
		var spawnErr *sessionCreateError
		sess, spawnErr = q.sessions.createSession(sessionCreateRequest{TaskID: taskID, Kind: "agent", Resume: true, Pending: &f, PendingBeforeSpawn: q.waitDispatch})
		if spawnErr != nil {
			state := "failed"
			if spawnErr.status == 409 {
				state = "skipped"
			}
			q.set(i, state, spawnErr.message)
			return
		}
	}
	var revision, input uint64
	if reopened {
		obs, err := sess.WaitInboxReady(q.ctx)
		if err != nil {
			q.set(i, "failed", err.Error())
			return
		}
		revision, input = obs.Revision, obs.Input
	} else {
		obs := sess.Observation()
		revision, input = obs.Revision, obs.Input
	}
	if !reopened && !q.waitDispatch() {
		q.set(i, "cancelled", "")
		return
	}
	// Serialize with reset/delete, and revalidate the live session and filters
	// after startup. The observed write also rejects keyboard/output races.
	unlock := q.sessions.mgr.LockTaskLifecycle(taskID)
	if q.ctx.Err() != nil {
		unlock()
		q.set(i, "cancelled", "")
		return
	}
	c, reason, err = q.sessions.pendingCandidate(f, taskID)
	if reopened && err == nil && c.live == sess {
		// Closed-only selection still owns the terminal it just reopened.
		activeFilter := f
		activeFilter.TerminalStates = []string{"active"}
		c, reason, err = q.sessions.pendingCandidate(activeFilter, taskID)
		if reason == "Agent is working" {
			reason = ""
		}
	}
	if err != nil || reason != "" || c.live != sess {
		unlock()
		if reason == "" {
			reason = "Card or agent changed before delivery"
		}
		q.set(i, "skipped", reason)
		return
	}
	body, submit := wrapInputForWrite(pendingPrompt)
	err = sess.WritePendingPrompt(revision, input, []byte(body), []byte(submit), reopened)
	unlock()
	if err != nil {
		if errors.Is(err, session.ErrObservedChanged) {
			q.set(i, "skipped", "Terminal changed or has no empty input prompt")
		} else {
			q.set(i, "failed", err.Error())
		}
		return
	}
	q.set(i, "running", "Prompt sent; waiting for the agent")
	ticker := time.NewTicker(q.poll)
	defer ticker.Stop()
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-sess.Done():
			info := sess.Info()
			if info.ExitCode != nil && *info.ExitCode != 0 {
				q.set(i, "failed", "Agent session ended with an error")
			} else {
				q.set(i, "done", "Agent session ended")
			}
			return
		case <-ticker.C:
			info := sess.Info()
			if sess.PendingCheckFinished() {
				q.set(i, "done", "Agent is ready for your next instruction")
				return
			}
			if info.AgentStatus == "error" {
				q.set(i, "failed", "Agent needs attention")
				return
			}
		}
	}
}

package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"kamacu/internal/diagnostics"
	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/presence"
	"kamacu/internal/session"
	"kamacu/internal/store"
	"kamacu/internal/terminalchoice"
)

type Service struct {
	DB         *sql.DB
	Sessions   *session.Manager
	Inbox      *inbox.Service
	Presence   *presence.Tracker
	Classifier Classifier
	jobs       sync.WaitGroup
	mu         sync.Mutex
	states     map[string]*watch
	workers    chan struct{}
}
type watch struct {
	taskContext          TaskContext
	inflightTask         TaskContext
	revision, input      uint64
	profile              string
	lastAttempt          time.Time
	lastChange           time.Time
	observedRevision     uint64
	busy                 bool
	cancel               context.CancelFunc
	inflightInput        uint64
	inflightProfile      string
	decision             Decision
	evidence             observation.Snapshot
	episode, fingerprint string
	seen                 bool
	active               bool
	episodeInput         uint64
}

func New(db *sql.DB, m *session.Manager, i *inbox.Service, p *presence.Tracker, c Classifier) *Service {
	return &Service{DB: db, Sessions: m, Inbox: i, Presence: p, Classifier: c, states: map[string]*watch{}, workers: make(chan struct{}, 4)}
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		s.mu.Lock()
		for _, w := range s.states {
			if w.cancel != nil {
				w.cancel()
			}
		}
		s.mu.Unlock()
		s.jobs.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}
func (s *Service) tick(ctx context.Context) {
	// The scheduler alone mutates state, except classify results under mu.

	infos := s.Sessions.List()
	processes := map[string]bool{}
	for _, info := range infos {
		processes[info.ID] = info.Status == session.StatusRunning
	}
	if err := store.ReconcileSessionStates(ctx, s.DB, processes, time.Now()); err != nil {
		slog.Warn("reconcile session work lifecycle", "error", err)
	}
	// Oldest attempt first prevents busy recent sessions starving other workspaces.
	s.mu.Lock()
	sort.SliceStable(infos, func(i, j int) bool {
		var a, b time.Time
		if w := s.states[infos[i].ID]; w != nil {
			a = w.lastAttempt
		}
		if w := s.states[infos[j].ID]; w != nil {
			b = w.lastAttempt
		}
		return a.Before(b)
	})
	live := map[string]bool{}
	for _, info := range infos {
		if info.Status == session.StatusRunning {
			live[info.ID] = true
		}
	}
	for id, w := range s.states {
		if !live[id] {
			if w.cancel != nil {
				w.cancel()
			}
			if !w.busy {
				delete(s.states, id)
			}
		}
	}
	s.mu.Unlock()
	for _, info := range infos {
		if info.Kind != session.KindAgent || info.Status != session.StatusRunning {
			continue
		}
		sess, ok := s.Sessions.Get(info.ID)
		if !ok {
			continue
		}
		obs := sess.Observation()
		p, err := Resolve(ctx, s.DB, info.TaskID)
		task, taskErr := s.taskContext(ctx, info)
		if taskErr != nil {
			err = taskErr
		}
		if obs.UnsupportedSize {
			err = fmt.Errorf("terminal exceeds the supported monitoring size (300 columns, 100 rows)")
		}
		if err != nil {
			sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: err.Error()})
			s.mu.Lock()
			if w := s.states[info.ID]; w != nil && w.cancel != nil {
				w.cancel()
			}
			s.mu.Unlock()
			_ = s.Inbox.HideObserved(ctx, info.ID, true)
			continue
		}
		s.mu.Lock()
		w := s.states[info.ID]
		if w == nil {
			w = &watch{}
			s.states[info.ID] = w
			var raw string
			var seen, active bool
			e := s.DB.QueryRowContext(ctx, `SELECT episode_id,fingerprint,decision,seen,active,input_generation FROM monitor_episodes WHERE session_id=?`, info.ID).Scan(&w.episode, &w.fingerprint, &raw, &seen, &active, &w.episodeInput)
			if e == nil {
				_ = json.Unmarshal([]byte(raw), &w.decision)
				w.seen = seen
				w.active = active
			}
		}
		if w.busy && w.cancel != nil && (w.inflightInput != obs.Input || w.inflightProfile != p.Key() || w.inflightTask != task) {
			w.cancel()
		}
		viewed := s.Presence.ViewingSession(info.ID)
		// Hide immediately even if inference is busy. Never expose an obsolete
		// question while its evidence or input generation has changed.
		stale := obs.Input != w.input || obs.Revision != w.revision || w.profile != p.Key() || w.taskContext != task
		if err := s.visibility(ctx, info.ID, viewed || stale || w.seen || w.decision.State == "unknown"); err != nil {
			slog.Warn("monitor visibility", "error", err)
		}
		if !stale && w.active && w.decision.Kind != "none" && w.decision.State != "unknown" && !viewed && !w.seen {
			s.publish(ctx, info, w)
		}
		if obs.Revision != w.observedRevision {
			w.lastChange = time.Now()
			w.observedRevision = obs.Revision
		}
		due := !w.busy && stale && obs.Screen != "" && time.Since(w.lastAttempt) > 5*time.Second && (time.Since(w.lastChange) > 2*time.Second || time.Since(w.lastAttempt) > 10*time.Second)
		if due {
			select {
			case s.workers <- struct{}{}:
				w.busy = true
				w.lastAttempt = time.Now()
				jobCtx, cancel := context.WithCancel(ctx)
				w.cancel = cancel
				w.inflightInput = obs.Input
				w.inflightProfile = p.Key()
				w.inflightTask = task
				s.jobs.Add(1)
				go func() { defer s.jobs.Done(); s.classify(jobCtx, sess, p, obs, task) }()
			default:
			}
		}
		s.mu.Unlock()
	}
}
func (s *Service) classify(parent context.Context, sess *session.Session, p Profile, obs observation.Snapshot, task TaskContext) {
	defer func() { <-s.workers }()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	start := time.Now()
	ctx = diagnostics.WithGroup(ctx, diagnostics.Group{Kind: "session", ID: sess.Info().ID, SessionID: sess.Info().ID, StartedAt: sess.Info().CreatedAt, TaskID: task.ID, TaskTitle: task.Title, ProjectID: task.ProjectID, ProjectName: task.ProjectName})
	inferenceTask := task
	if task.ID == 0 {
		work, e := store.WorkForSession(ctx, s.DB, sess.Info().ID)
		if e == nil {
			raw, _ := json.Marshal(work)
			inferenceTask.PreviousWork = string(raw)
		}
	}
	if task.ID > 0 {
		links, e := store.PRLinks(ctx, s.DB, task.ID, "", 0)
		work, we := store.WorkForTask(ctx, s.DB, task.ID)
		if e == nil && we == nil {
			// Prior context is a compact continuation aid, not another terminal
			// transcript. Full evidence remains available through MCP.
			type priorLink struct {
				Repository string
				Number     int64
				Role       string
				Corrected  bool
			}
			type priorWork struct {
				SessionID  string
				Summary    string
				State      string
				Stale      bool
				ObservedAt time.Time
			}
			priorLinks := []priorLink{}
			for _, l := range links {
				priorLinks = append(priorLinks, priorLink{l.Repository, l.Number, l.Role, l.Corrected})
			}
			priorSessions := []priorWork{}
			for _, w := range work[:min(3, len(work))] {
				priorSessions = append(priorSessions, priorWork{w.SessionID, w.Summary, w.State, w.Stale, w.ObservedAt})
			}
			raw, _ := json.Marshal(map[string]any{"links": priorLinks, "sessions": priorSessions})
			inferenceTask.PreviousWork = string(raw)
		}
	}
	slog.Debug("classifying terminal", "session", sess.Info().ID, "profile", p.ID, "revision", obs.Revision)
	d, err := s.Classifier.Classify(ctx, p, obs, inferenceTask)
	info := sess.Info()
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.states[info.ID]
	w.busy = false
	if w.cancel != nil {
		defer w.cancel()
		w.cancel = nil
	}
	current := sess.Observation()
	effective, e := Resolve(parent, s.DB, info.TaskID)
	currentTask, taskErr := s.taskContext(parent, info)
	if info.Status != session.StatusRunning || info.StopRequested || taskErr != nil || currentTask != task || e != nil || effective.Key() != p.Key() || current.Input != obs.Input || current.UnsupportedSize {
		slog.Debug("discarding obsolete terminal classification", "session", info.ID)
		return
	}
	// Provider failures must back off even when terminal output kept changing.
	if err != nil {
		slog.Debug("terminal classification failed; backing off", "session", info.ID, "error", err, "backoff_seconds", 20)
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: err.Error()})
		_ = s.Inbox.HideObserved(parent, info.ID, true)
		w.lastAttempt = time.Now().Add(20 * time.Second)
		return
	}
	// An old question can remain visible after progress. Never accept an old
	// screen merely because its evidence excerpt is still present.
	if current.Revision != obs.Revision {
		slog.Debug("terminal changed during classification", "session", info.ID, "revision", obs.Revision, "current_revision", current.Revision)
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "ready", Detail: "Terminal changed during analysis; waiting for stable evidence"})
		_ = s.Inbox.HideObserved(parent, info.ID, true)
		return
	}
	if err = validateDecision(d, obs); err != nil {
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: err.Error()})
		w.lastAttempt = time.Now().Add(20 * time.Second)
		return
	}
	var taskID *int64
	if info.TaskID > 0 {
		id := info.TaskID
		taskID = &id
	}
	if err = store.PutSessionState(parent, s.DB, info.ID, taskID, d.State, start); err != nil {
		s.persistenceFailure(parent, sess, w)
		return
	}
	if d.WorkSummary != "" {
		if err = store.PutSessionWork(parent, s.DB, store.SessionWork{SessionID: info.ID, TaskID: taskID, State: d.State, Summary: d.WorkSummary, Evidence: d.WorkEvidence, Uncertainty: d.WorkUncertainty, ObservedAt: obs.EvidenceAt(d.WorkEvidence)}); err != nil {
			s.persistenceFailure(parent, sess, w)
			return
		}
	}
	for _, link := range d.PRLinks {
		observedLink := store.PRLink{TaskID: info.TaskID, Repository: link.Repository, Number: link.Number, Role: link.Role, Source: "terminal:" + info.ID, Evidence: link.Evidence, Uncertainty: link.Uncertainty, ObservedAt: obs.EvidenceAt(link.Evidence)}
		if err = store.PutSessionPRLink(parent, s.DB, info.ID, observedLink); err != nil {
			s.persistenceFailure(parent, sess, w)
			return
		}
		if info.TaskID <= 0 {
			continue
		}
		if err = store.PutPRLink(parent, s.DB, store.PRLink{TaskID: info.TaskID, Repository: link.Repository, Number: link.Number, Role: link.Role, Source: "terminal:" + info.ID, Evidence: link.Evidence, Uncertainty: link.Uncertainty, ObservedAt: obs.EvidenceAt(link.Evidence)}); err != nil {
			s.persistenceFailure(parent, sess, w)
			return
		}
	}
	freshRequest := w.episodeInput != current.Input && !obs.InputAt.IsZero() && obs.EvidenceAt(d.Evidence).After(obs.InputAt)
	w.revision = current.Revision
	w.input = current.Input
	w.profile = p.Key()
	w.taskContext = task
	w.evidence = current
	w.evidence.At = obs.EvidenceAt(d.Evidence)
	w.decision = d
	observedAt := obs.At
	if obs.InputAt.After(observedAt) {
		observedAt = obs.InputAt
	}
	sess.SetMonitorStatus(session.MonitorStatus{State: d.State, Health: "ready", ObservedAt: observedAt})
	slog.Debug("terminal classified", "session", info.ID, "state", d.State, "latency_ms", time.Since(start).Milliseconds())
	if d.Kind == "none" || d.State == "unknown" {
		if d.State == "working" || d.State == "idle" {
			if err := s.Inbox.SupersedeObserved(parent, info.ID); err != nil {
				s.persistenceFailure(parent, sess, w)
				return
			}
			w.active = false
			w.fingerprint = ""
			if err := s.persist(parent, info.ID, w); err != nil {
				s.persistenceFailure(parent, sess, w)
			}
		}
		return
	}
	// Rewording a question or switching between a question and a native menu
	// updates the same attention episode. Only observed progress, fresh input
	// followed by new evidence, or a completion boundary creates a new one.
	fingerprint := "attention"
	if d.Kind == "completion" {
		fingerprint = "completion"
	}
	if !w.active || w.fingerprint != fingerprint || freshRequest {
		if w.active {
			if err := s.Inbox.SupersedeObserved(parent, info.ID); err != nil {
				s.persistenceFailure(parent, sess, w)
				return
			}
		}
		w.episode = uuid.NewString()
		w.episodeInput = current.Input
		w.fingerprint = fingerprint
		w.active = true
		w.seen = false
	}
	if d.Kind == "completion" && s.Presence.ViewingAt(info.ID, obs.EvidenceAt(d.Evidence)) {
		w.seen = true
	}
	if err := s.persist(parent, info.ID, w); err != nil {
		s.persistenceFailure(parent, sess, w)
		return
	}
	if !s.Presence.ViewingSession(info.ID) && !w.seen {
		s.publish(parent, info, w)
	}
}
func (s *Service) persist(ctx context.Context, id string, w *watch) error {
	raw, _ := json.Marshal(w.decision)
	_, e := s.DB.ExecContext(ctx, `INSERT INTO monitor_episodes(session_id,episode_id,fingerprint,decision,seen,active,input_generation) VALUES(?,?,?,?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET episode_id=excluded.episode_id,fingerprint=excluded.fingerprint,decision=excluded.decision,seen=excluded.seen,active=excluded.active,input_generation=excluded.input_generation`, id, w.episode, w.fingerprint, string(raw), w.seen, w.active, w.episodeInput)
	if e != nil {
		slog.Warn("persist monitor episode", "error", e)
	}
	return e
}
func (s *Service) publish(ctx context.Context, info session.Info, w *watch) {
	sess, ok := s.Sessions.Get(info.ID)
	if !ok || sess.Info().Status != session.StatusRunning || sess.Info().StopRequested {
		return
	}
	current := sess.Observation()
	if current.Revision != w.revision || current.Input != w.input {
		return
	}
	if s.Presence.ViewingSession(info.ID) {
		return
	}
	task, err := s.taskContext(ctx, info)
	if err != nil || task != w.taskContext {
		return
	}
	d := w.decision
	kind := "question"
	question := d.Question
	mode := d.InputMode
	var options []inbox.Option
	if d.Kind == "question" && mode == "text" {
		for i, label := range d.Options {
			options = append(options, inbox.Option{ID: fmt.Sprintf("choice-%d", i+1), Label: label})
		}
	}
	if d.Kind == "completion" {
		kind = "completion"
		question = "Work completed"
		mode = "none"
		options = inbox.CompletionOptions(info.TaskID, w.taskContext.Source, w.taskContext.Status, d.CompletionActions)
	} else if d.Kind == "permission" || d.Kind == "error" {
		mode = "terminal"
		if question == "" {
			question = "Please review this in the terminal."
		}
	}
	// Only the deterministic terminal profile can enable native choice delivery.
	if info.Engine == "claude" && d.Kind == "question" && d.State == "waiting" {
		if dialog, ok := terminalchoice.Claude(current.Screen); ok {
			mode = terminalchoice.Mode
			question = dialog.Question
			options = nil
			for i, label := range dialog.Options {
				options = append(options, inbox.Option{ID: fmt.Sprintf("choice-%d", i+1), Label: label})
			}
		}
	}
	if mode != "text" && mode != terminalchoice.Mode {
		mode = "terminal"
	}
	summary := d.Summary
	if summary == "" {
		summary = question
	}
	message := d.Context
	if strings.TrimSpace(message) == "" {
		// Older saved decisions have only evidence. Relay that text directly.
		message = d.Evidence
	}
	if strings.TrimSpace(message) == "" {
		message = summary
	}
	abstract := []rune(summary)
	if len(abstract) > 480 {
		abstract = abstract[:480]
	}
	err = s.Presence.WhileAway(info.ID, func() error {
		_, err := s.Inbox.Observe(ctx, inbox.Identity{TaskID: info.TaskID, SessionID: info.ID}, inbox.Create{Key: "observer:" + w.episode, Abstract: string(abstract), Summary: summary, Question: question, Context: message, Options: options}, kind, mode, w.revision, w.input)
		return err
	})
	if err != nil {
		slog.Warn("publish observation", "error", err)
	}
}

// Restoring an existing card is a publication too. Serialize it with new
// heartbeats so an older scheduler read cannot undo a just-arrived hide.
func (s *Service) visibility(ctx context.Context, id string, hide bool) error {
	if hide {
		return s.Inbox.HideObserved(ctx, id, true)
	}
	return s.Presence.WhileAway(id, func() error { return s.Inbox.HideObserved(ctx, id, false) })
}
func (s *Service) persistenceFailure(ctx context.Context, sess *session.Session, w *watch) {
	w.revision = 0
	w.decision = Decision{State: "unknown", Kind: "none", InputMode: "none"}
	_ = s.Inbox.HideObserved(ctx, sess.Info().ID, true)
	sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: "Could not save monitoring state"})
}

func (s *Service) taskContext(ctx context.Context, info session.Info) (TaskContext, error) {
	task := TaskContext{ID: info.TaskID, Engine: info.Engine, Scope: "task"}
	if info.Global || info.TaskID == 0 {
		task.Scope = "scratchpad"
		return task, nil
	}
	err := s.DB.QueryRowContext(ctx, `SELECT t.title,t.description,t.status,t.source,p.id,p.name FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=?`, info.TaskID).Scan(&task.Title, &task.Description, &task.Status, &task.Source, &task.ProjectID, &task.ProjectName)
	return task, err
}

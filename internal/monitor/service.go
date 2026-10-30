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
	"kamacu/internal/inbox"
	"kamacu/internal/observation"
	"kamacu/internal/presence"
	"kamacu/internal/session"
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
		if w.busy && w.cancel != nil && (w.inflightInput != obs.Input || w.inflightProfile != p.Key()) {
			w.cancel()
		}
		viewed := s.Presence.ViewingSession(info.ID)
		// Hide immediately even if inference is busy. Never expose an obsolete
		// question while its evidence or input generation has changed.
		stale := obs.Input != w.input || obs.Revision != w.revision || w.profile != p.Key()
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
				s.jobs.Add(1)
				go func() { defer s.jobs.Done(); s.classify(jobCtx, sess, p, obs) }()
			default:
			}
		}
		s.mu.Unlock()
	}
}
func (s *Service) classify(parent context.Context, sess *session.Session, p Profile, obs observation.Snapshot) {
	defer func() { <-s.workers }()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	start := time.Now()
	d, err := s.Classifier.Classify(ctx, p, obs)
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
	if info.Status != session.StatusRunning || e != nil || effective.Key() != p.Key() || current.Input != obs.Input || current.UnsupportedSize {
		return
	}
	// Provider failures must back off even when terminal output kept changing.
	if err != nil {
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: err.Error()})
		_ = s.Inbox.HideObserved(parent, info.ID, true)
		w.lastAttempt = time.Now().Add(20 * time.Second)
		return
	}
	// An old question can remain visible after progress. Never accept an old
	// screen merely because its evidence excerpt is still present.
	if current.Revision != obs.Revision {
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "ready", Detail: "Terminal changed during analysis; waiting for stable evidence"})
		_ = s.Inbox.HideObserved(parent, info.ID, true)
		return
	}
	if err = validateDecision(d, obs); err != nil {
		sess.SetMonitorStatus(session.MonitorStatus{State: "unknown", Health: "unavailable", Detail: err.Error()})
		w.lastAttempt = time.Now().Add(20 * time.Second)
		return
	}
	freshRequest := w.episodeInput != current.Input && !obs.InputAt.IsZero() && obs.EvidenceAt(d.Evidence).After(obs.InputAt)
	w.revision = current.Revision
	w.input = current.Input
	w.profile = p.Key()
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
	if !ok || sess.Info().Status != session.StatusRunning {
		return
	}
	current := sess.Observation()
	if current.Revision != w.revision || current.Input != w.input {
		return
	}
	if s.Presence.ViewingSession(info.ID) {
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
		options = []inbox.Option{{ID: "exit", Label: inbox.CompletionExit}}
	} else if d.Kind == "permission" || d.Kind == "error" {
		mode = "terminal"
		if question == "" {
			question = "Please review this in the terminal."
		}
	}
	if mode != "text" {
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
	err := s.Presence.WhileAway(info.ID, func() error {
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

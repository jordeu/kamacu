package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"kamacu/internal/inbox"
	"kamacu/internal/monitor"
	"kamacu/internal/settings"
)

// Model validates response contracts before reporting or recording the outcome.
// Implementations must support concurrent calls from independent projects.
type Model interface {
	GenerateValidatedJSON(context.Context, monitor.Profile, string, any, func([]byte) error) ([]byte, error)
}
type Service struct {
	DB             *sql.DB
	Inbox          *inbox.Service
	Actions        ActionRuntime
	Metrics        *monitor.Metrics
	Model          Model
	Tools          Tools
	SessionTask    func(string) (int64, bool)
	TaskHasSession func(int64) bool
	TaskSession    func(int64) string
	Providers      map[string]Provider
	// workMu serializes ticks; source CRUD has its own short lock.
	workMu         sync.Mutex
	progressMu     sync.RWMutex
	progressRun    *notificationRun
	progress       Progress
	handoffMu      sync.Mutex
	SessionChanges func() <-chan struct{}
	actionMu       sync.Mutex
	sourceMu       sync.Mutex
}

func New(db *sql.DB, box *inbox.Service, model Model, tools Tools) *Service {
	return &Service{DB: db, Inbox: box, Model: model, Tools: tools, Providers: map[string]Provider{"github": GitHub{DB: db}}}
}

const sourceColumns = `id,provider,name,config,enabled,account,checkpoint,next_poll,last_success,last_error,created_at,(SELECT count(*) FROM notification_events e WHERE e.source_id=notification_sources.id AND e.raw_status='pending'),COALESCE((SELECT last_error FROM notification_events e WHERE e.source_id=notification_sources.id AND e.raw_status='pending' AND e.last_error!='' ORDER BY e.id DESC LIMIT 1),'')`

func scanSource(row interface{ Scan(...any) error }) (s Source, err error) {
	var config string
	err = row.Scan(&s.ID, &s.Provider, &s.Name, &config, &s.Enabled, &s.Account, &s.Checkpoint, &s.NextPoll, &s.LastSuccess, &s.LastError, &s.CreatedAt, &s.PendingEvents, &s.ProcessingError)
	s.Config = json.RawMessage(config)
	return
}
func (s *Service) Sources(ctx context.Context) ([]Source, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE retired=0 ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		v, e := scanSource(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) AddSource(ctx context.Context, provider, name string, config json.RawMessage) (Source, error) {
	s.sourceMu.Lock()
	defer s.sourceMu.Unlock()
	p, ok := s.Providers[provider]
	if !ok {
		return Source{}, errors.New("unknown notification provider")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Source{}, errors.New("source name must contain 1–120 bytes")
	}
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	if err := p.Validate(config); err != nil {
		return Source{}, err
	}
	// Sources added for the same gh identity would duplicate every question.
	if provider == "github" {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_sources WHERE provider='github' AND retired=0`).Scan(&n); err != nil {
			return Source{}, err
		}
		if n > 0 {
			return Source{}, errors.New("a GitHub source is already registered")
		}
	}
	now := time.Now().UTC()
	v := Source{ID: uuid.NewString(), Provider: provider, Name: name, Config: config, Enabled: true, Checkpoint: now.Format(time.RFC3339), CreatedAt: now.UnixMilli()}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO notification_sources(id,provider,name,config,checkpoint,created_at) VALUES(?,?,?,?,?,?)`, v.ID, v.Provider, v.Name, string(v.Config), v.Checkpoint, v.CreatedAt)
	return v, err
}
func (s *Service) EnableSource(ctx context.Context, id string, enabled bool) error {
	s.sourceMu.Lock()
	defer s.sourceMu.Unlock()
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_sources SET enabled=? WHERE id=? AND retired=0`, enabled, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Recover never repeats an execution whose last tool call may have taken effect.
func (s *Service) Recover(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE notification_rule_previews SET status='failed',error='Preview was interrupted; request another preview.' WHERE status='running'`); err != nil {
		return err
	}

	if _, err := s.DB.ExecContext(ctx, `UPDATE notification_batches SET status='failed',error='Processing was interrupted. Notifications remain pending; start processing again.',updated_at=? WHERE status='running'`, time.Now().UnixMilli()); err != nil {
		return err
	}
	return s.recoverActionRuns(ctx, true)
}

func (s *Service) Run(ctx context.Context) {
	reconcileDone := make(chan struct{})
	go func() { defer close(reconcileDone); s.runHandoffReconciliation(ctx) }()
	defer func() { <-reconcileDone }()
	actionDone := make(chan struct{})
	defer func() { <-actionDone }()
	go func() {
		defer close(actionDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.executeNext(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("Inbox action execution", "error", err)
				}
			}
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("notification processing", "error", err)
			}
		}
	}
}
func (s *Service) Tick(ctx context.Context) error {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	// Inbox actions have their own worker and never block source collection.
	if err := s.processRulePreview(ctx); err != nil {
		return err
	}
	sources, err := s.Sources(ctx)
	if err != nil {
		return err
	}
	gate, err := settings.Get(s.DB, settings.KeyGithubIntegration)
	if err != nil {
		return err
	}
	repos, err := s.repositories(ctx)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if !source.Enabled || (source.Provider == "github" && gate != "on") {
			continue
		}
		p, ok := s.Providers[source.Provider]
		if !ok {
			continue
		}
		if source.NextPoll <= time.Now().UnixMilli() {
			callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			batch, e := p.Collect(callCtx, source, repos)
			cancel()
			if e != nil {
				_, err = s.DB.ExecContext(ctx, `UPDATE notification_sources SET last_error=?,next_poll=? WHERE id=?`, e.Error(), time.Now().Add(time.Minute).UnixMilli(), source.ID)
			} else {
				err = s.ingest(ctx, source, batch)
			}
			if err != nil {
				return err
			}
		}
	}
	return s.processNext(ctx)
}
func (s *Service) repositories(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT github_repo FROM projects WHERE github_repo IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var repo string
		if err = rows.Scan(&repo); err != nil {
			return nil, err
		}
		out[strings.ToLower(repo)] = true
	}
	return out, rows.Err()
}
func (s *Service) ingest(ctx context.Context, source Source, batch Batch) error {
	slog.Debug("ingesting notifications", "source", source.ID, "events", len(batch.Events))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Disabling during an in-flight poll must not import more events.
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM notification_sources WHERE id=?`, source.ID).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	for _, event := range batch.Events {
		data, e := json.Marshal(event)
		if e != nil {
			return e
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO notification_events(source_id,external_id,version,payload,created_at) VALUES(?,?,?,?,?) ON CONFLICT(source_id,external_id,version) DO NOTHING`, source.ID, event.ExternalID, event.Version, string(data), time.Now().UnixMilli())
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE notification_sources SET account=?,checkpoint=?,last_success=?,last_error='',next_poll=? WHERE id=?`, batch.Account, batch.Checkpoint, time.Now().UnixMilli(), time.Now().Add(max(time.Minute, batch.PollAfter)).UnixMilli(), source.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveSource disconnects collection but preserves notification history.
func (s *Service) RemoveSource(ctx context.Context, id string) error {
	s.sourceMu.Lock()
	defer s.sourceMu.Unlock()
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_sources SET enabled=0,retired=1 WHERE id=? AND retired=0`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

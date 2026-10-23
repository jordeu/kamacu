// Package inbox owns durable feedback questions. All state transitions are
// serialized, including session exit and ownership transfers. Pending questions
// remain live until their owning session stops; agent polling is not required.
package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalid  = errors.New("invalid feedback request")
	ErrIdentity = errors.New("a live Kamacu task or Scratchpad agent session is required")
	ErrNotFound = errors.New("question not found")
	ErrOwner    = errors.New("question belongs to another session; explicitly reopen an unanswered question to take ownership")
	ErrConflict = errors.New("question cannot be changed in its current state")
)

type Identity struct {
	TaskID    int64  `json:"task_id"`
	SessionID string `json:"session_id"`
}
type Session struct {
	TaskID int64
	Global bool
	Live   bool
	Done   <-chan struct{}
}
type Lookup func(string) Session
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Question struct {
	Kind           string   `json:"kind"`
	ID             string   `json:"id"`
	TaskID         int64    `json:"task_id"`
	ProjectID      int64    `json:"project_id"`
	TaskTitle      string   `json:"task_title"`
	ProjectName    string   `json:"project_name"`
	SessionID      string   `json:"session_id"`
	Abstract       string   `json:"abstract"`
	Summary        string   `json:"summary"`
	Context        string   `json:"context"`
	Question       string   `json:"question"`
	Options        []Option `json:"options"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason"`
	Answer         string   `json:"answer"`
	AnswerSource   string   `json:"answer_source"`
	Revision       int64    `json:"revision"`
	CreatedAt      int64    `json:"created_at"`
	UpdatedAt      int64    `json:"updated_at"`
	HeartbeatAt    int64    `json:"heartbeat_at"`
	AcknowledgedAt *int64   `json:"acknowledged_at"`
}
type Create struct {
	Key      string   `json:"request_key"`
	Abstract string   `json:"abstract"`
	Summary  string   `json:"summary"`
	Context  string   `json:"context"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
}
type Result struct {
	Outcome  string   `json:"outcome"`
	Question Question `json:"question"`
}
type Page struct {
	Items        []Question `json:"items"`
	PendingCount int        `json:"pending_count"`
	HasMore      bool       `json:"has_more"`
}
type Service struct {
	db       *sql.DB
	lookup   Lookup
	mu       sync.Mutex
	changed  chan struct{}
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	watching map[string]bool
}

func New(db *sql.DB, lookup Lookup) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{db: db, lookup: lookup, changed: make(chan struct{}), now: time.Now, ctx: ctx, cancel: cancel, watching: map[string]bool{}}
}
func (s *Service) Close() { s.cancel() }
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil {
				slog.Error("expire inbox questions", "error", err)
			}
		}
	}
}
func (s *Service) signal() { close(s.changed); s.changed = make(chan struct{}) }
func (s *Service) valid(who Identity) error {
	if who.TaskID < 0 || who.SessionID == "" {
		return ErrIdentity
	}
	live := s.lookup(who.SessionID)
	if !live.Live || live.TaskID != who.TaskID || live.Global != (who.TaskID == 0) {
		return ErrIdentity
	}
	return nil
}

const selectQuestion = `SELECT q.id,COALESCE(q.task_id,0),COALESCE(t.project_id,0),COALESCE(t.title,'Scratchpad'),COALESCE(p.name,''),q.session_id,q.summary,q.context,q.question,q.options,q.status,q.reason,q.answer,q.answer_source,q.revision,q.created_at,q.updated_at,q.heartbeat_at,q.acknowledged_at,q.kind,q.abstract FROM inbox_questions q LEFT JOIN tasks t ON t.id=q.task_id LEFT JOIN projects p ON p.id=t.project_id `

func scan(row interface{ Scan(...any) error }) (Question, error) {
	var q Question
	var opts string
	err := row.Scan(&q.ID, &q.TaskID, &q.ProjectID, &q.TaskTitle, &q.ProjectName, &q.SessionID, &q.Summary, &q.Context, &q.Question, &opts, &q.Status, &q.Reason, &q.Answer, &q.AnswerSource, &q.Revision, &q.CreatedAt, &q.UpdatedAt, &q.HeartbeatAt, &q.AcknowledgedAt, &q.Kind, &q.Abstract)
	if errors.Is(err, sql.ErrNoRows) {
		return q, ErrNotFound
	}
	if err != nil {
		return q, err
	}
	err = json.Unmarshal([]byte(opts), &q.Options)
	return q, err
}
func (s *Service) get(ctx context.Context, id string) (Question, error) {
	return scan(s.db.QueryRowContext(ctx, selectQuestion+`WHERE q.id=?`, id))
}
func outcome(q Question) string {
	if q.Status == "unanswered" {
		return "expired"
	}
	return q.Status
}
func result(q Question) Result { return Result{outcome(q), q} }

// transition writes the state and its history together. Call with mu held.
func (s *Service) transition(ctx context.Context, id, kind, session string, query string, args ...any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_events(question_id,kind,session_id,created_at) VALUES(?,?,?,?)`, id, kind, session, s.now().UnixMilli()); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		s.signal()
	}
	return err
}
func (s *Service) reconcile(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,heartbeat_at FROM inbox_questions WHERE status='pending'`)
	if err != nil {
		return err
	}
	type stale struct{ id, session, reason string }
	var expired []stale
	for rows.Next() {
		var id, sid string
		var beat int64
		if err = rows.Scan(&id, &sid, &beat); err != nil {
			rows.Close()
			return err
		}
		reason := ""
		if !s.lookup(sid).Live {
			reason = "session_stopped"
		}
		if reason != "" {
			expired = append(expired, stale{id, sid, reason})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, q := range expired {
		if err = s.transition(ctx, q.id, q.reason, q.session, `UPDATE inbox_questions SET status='unanswered',reason=?,updated_at=? WHERE id=?`, q.reason, s.now().UnixMilli(), q.id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcile(ctx)
}
func (s *Service) watch(sid string) {
	if s.watching[sid] {
		return
	}
	done := s.lookup(sid).Done
	if done == nil {
		return
	}
	s.watching[sid] = true
	go func() {
		select {
		case <-s.ctx.Done():
			return
		case <-done:
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watching, sid)
		if err := s.reconcile(s.ctx); err != nil && s.ctx.Err() == nil {
			slog.Error("archive stopped agent questions", "error", err)
		}
	}()
}
func validate(c Create) error {
	if strings.TrimSpace(c.Abstract) == "" || !utf8.ValidString(c.Abstract) || utf8.RuneCountInString(c.Abstract) > 180 {
		return fmt.Errorf("%w: abstract is required and must be at most 180 characters", ErrInvalid)
	}
	for _, field := range []struct {
		v string
		n int
	}{{c.Key, 200}, {c.Summary, 4000}, {c.Context, 32000}, {c.Question, 4000}} {
		if strings.TrimSpace(field.v) == "" || len(field.v) > field.n {
			return fmt.Errorf("%w: request key, summary, context and question are required and must fit their length limits", ErrInvalid)
		}
	}
	if len(c.Options) > 10 {
		return fmt.Errorf("%w: at most 10 options", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, o := range c.Options {
		if strings.TrimSpace(o.ID) == "" || len(o.ID) > 100 || strings.TrimSpace(o.Label) == "" || len(o.Label) > 500 || seen[o.ID] {
			return fmt.Errorf("%w: option IDs must be unique and labels nonempty", ErrInvalid)
		}
		seen[o.ID] = true
	}
	return nil
}

const CompletionSeen = "Seen"
const CompletionExit = "Seen and exit session"

// Complete uses the same durable answer protocol, with server-defined actions.
func (s *Service) Complete(ctx context.Context, who Identity, c Create) (Result, error) {
	c.Question = "Work completed"
	c.Options = []Option{{ID: "seen", Label: CompletionSeen}, {ID: "exit", Label: CompletionExit}}
	return s.create(ctx, who, c, "completion")
}
func (s *Service) Create(ctx context.Context, who Identity, c Create) (Result, error) {
	return s.create(ctx, who, c, "question")
}
func (s *Service) create(ctx context.Context, who Identity, c Create, kind string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.valid(who); err != nil {
		return Result{}, err
	}
	if err := validate(c); err != nil {
		return Result{}, err
	}
	if err := s.reconcile(ctx); err != nil {
		return Result{}, err
	}
	q, err := scan(s.db.QueryRowContext(ctx, selectQuestion+`WHERE q.task_id IS NULLIF(?,0) AND q.created_session_id=? AND q.request_key=?`, who.TaskID, who.SessionID, c.Key))
	if err == nil {
		return Result{"existing", q}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Result{}, err
	}
	q, err = scan(s.db.QueryRowContext(ctx, selectQuestion+`WHERE q.session_id=? AND q.status='pending'`, who.SessionID))
	if err == nil {
		return Result{"existing_pending", q}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Result{}, err
	}
	if c.Options == nil {
		c.Options = []Option{}
	}
	opts, _ := json.Marshal(c.Options)
	id := uuid.NewString()
	now := s.now().UnixMilli()
	err = s.transition(ctx, id, "created", who.SessionID, `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,summary,context,question,options,status,created_at,updated_at,heartbeat_at,kind,abstract) VALUES(?,NULLIF(?,0),?,?,?,?,?,?,?,'pending',?,?,?,?,?)`, id, who.TaskID, who.SessionID, who.SessionID, c.Key, c.Summary, c.Context, c.Question, string(opts), now, now, now, kind, c.Abstract)
	if err != nil {
		return Result{}, err
	}
	s.watch(who.SessionID)
	q, err = s.get(ctx, id)
	return Result{"created", q}, err
}
func (s *Service) owned(ctx context.Context, who Identity, id string) (Question, error) {
	if err := s.valid(who); err != nil {
		return Question{}, err
	}
	q, err := s.get(ctx, id)
	if err != nil {
		return q, err
	}
	if q.TaskID != who.TaskID || q.SessionID != who.SessionID {
		return q, ErrOwner
	}
	return q, nil
}
func (s *Service) poll(ctx context.Context, who Identity, id string, heartbeat bool) (Result, <-chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return Result{}, nil, err
	}
	q, err := s.owned(ctx, who, id)
	if err != nil {
		return Result{}, nil, err
	}
	if heartbeat && q.Status == "pending" {
		q.HeartbeatAt = s.now().UnixMilli()
		_, err = s.db.ExecContext(ctx, `UPDATE inbox_questions SET heartbeat_at=? WHERE id=?`, q.HeartbeatAt, id)
	}
	return result(q), s.changed, err
}
func (s *Service) Wait(ctx context.Context, who Identity, id string, wait time.Duration) (Result, error) {
	if wait < 0 || wait > 20*time.Second {
		return Result{}, fmt.Errorf("%w: wait_seconds must be between 0 and 20", ErrInvalid)
	}
	r, ch, err := s.poll(ctx, who, id, true)
	if err != nil || r.Question.Status != "pending" || wait == 0 {
		return r, err
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-s.ctx.Done():
			return Result{}, context.Canceled
		case <-timer.C:
			return r, nil
		case <-ch:
			r, ch, err = s.poll(ctx, who, id, false)
			if err != nil || r.Question.Status != "pending" {
				return r, err
			}
		}
	}
}

// Answer is shared by the browser and the session-reporting tool. nil identity
// means browser; browser callers must supply the revision rendered with the form.
func (s *Service) Answer(ctx context.Context, who *Identity, id string, revision int64, answer, optionID string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return Result{}, err
	}
	q, err := s.get(ctx, id)
	if who != nil {
		q, err = s.owned(ctx, *who, id)
	}
	if err != nil {
		return Result{}, err
	}
	if revision != q.Revision {
		return Result{"stale_revision", q}, nil
	}
	if q.Status != "pending" {
		return Result{"already_resolved", q}, nil
	}
	if optionID != "" {
		if answer != "" {
			return Result{}, fmt.Errorf("%w: choose an option or provide text", ErrInvalid)
		}
		for _, o := range q.Options {
			if o.ID == optionID {
				answer = o.Label
				break
			}
		}
		if answer == "" {
			return Result{}, fmt.Errorf("%w: unknown option", ErrInvalid)
		}
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || len(answer) > 16000 {
		return Result{}, fmt.Errorf("%w: answer must contain 1–16000 bytes", ErrInvalid)
	}
	source := "inbox"
	var ack any
	if who != nil {
		source = "session"
		ack = s.now().UnixMilli()
	}
	reason := ""
	if q.Kind == "completion" {
		reason = optionID
		// Kamacu handles exit directly; no agent remains to acknowledge it.
		if optionID == "exit" {
			ack = s.now().UnixMilli()
		}
	}
	err = s.transition(ctx, id, "answered_"+source, q.SessionID, `UPDATE inbox_questions SET status='answered',answer=?,answer_source=?,updated_at=?,acknowledged_at=?,reason=? WHERE id=?`, answer, source, s.now().UnixMilli(), ack, reason, id)
	if err != nil {
		return Result{}, err
	}
	q, err = s.get(ctx, id)
	return Result{"accepted", q}, err
}
func (s *Service) Dismiss(ctx context.Context, id string, revision int64) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return Result{}, err
	}
	q, err := s.get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if revision != q.Revision {
		return Result{"stale_revision", q}, nil
	}
	if q.Status != "pending" {
		return Result{"already_resolved", q}, nil
	}
	err = s.transition(ctx, id, "dismissed", q.SessionID, `UPDATE inbox_questions SET status='dismissed',updated_at=? WHERE id=?`, s.now().UnixMilli(), id)
	if err != nil {
		return Result{}, err
	}
	q, err = s.get(ctx, id)
	return result(q), err
}
func (s *Service) Reopen(ctx context.Context, who Identity, id string, revision int64, userRequested bool) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.valid(who); err != nil {
		return Result{}, err
	}
	if err := s.reconcile(ctx); err != nil {
		return Result{}, err
	}
	q, err := s.get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if q.TaskID != who.TaskID {
		return Result{}, ErrOwner
	}
	if q.Revision != revision {
		return Result{"stale_revision", q}, nil
	}
	if q.Status == "pending" && q.SessionID == who.SessionID {
		return result(q), nil
	}
	if q.Status != "unanswered" && !(q.Status == "dismissed" && userRequested) {
		return Result{}, ErrConflict
	}
	var pending int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_questions WHERE session_id=? AND status='pending'`, who.SessionID).Scan(&pending); err != nil {
		return Result{}, err
	}
	if pending != 0 {
		return Result{}, fmt.Errorf("%w: resolve the session's existing pending question first", ErrConflict)
	}
	now := s.now().UnixMilli()
	err = s.transition(ctx, id, "reopened", who.SessionID, `UPDATE inbox_questions SET session_id=?,status='pending',reason='',revision=revision+1,updated_at=?,heartbeat_at=?,acknowledged_at=NULL WHERE id=?`, who.SessionID, now, now, id)
	if err != nil {
		return Result{}, err
	}
	s.watch(who.SessionID)
	q, err = s.get(ctx, id)
	return Result{"reopened", q}, err
}
func (s *Service) Acknowledge(ctx context.Context, who Identity, id string, revision int64) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return Result{}, err
	}
	if err := s.valid(who); err != nil {
		return Result{}, err
	}
	q, err := s.get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	// Answers are durable task results. A resumed agent has a new session ID
	// and must be able to acknowledge them without reopening or changing the
	// original owner. Other states still require session ownership.
	if q.TaskID != who.TaskID || (q.SessionID != who.SessionID && q.Status != "answered") {
		return Result{}, ErrOwner
	}
	if q.Revision != revision {
		return Result{"stale_revision", q}, nil
	}
	if q.Status == "pending" {
		return Result{}, ErrConflict
	}
	if q.AcknowledgedAt == nil {
		err = s.transition(ctx, id, "acknowledged", who.SessionID, `UPDATE inbox_questions SET acknowledged_at=? WHERE id=?`, s.now().UnixMilli(), id)
		if err != nil {
			return Result{}, err
		}
		q, err = s.get(ctx, id)
	}
	return result(q), err
}

// Get returns a message independently of inbox filters and pagination.
func (s *Service) Get(ctx context.Context, id string) (Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return Question{}, err
	}
	return s.get(ctx, id)
}

func (s *Service) List(ctx context.Context, who *Identity, archived bool, offset int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := Page{Items: []Question{}}
	if who != nil {
		if err := s.valid(*who); err != nil {
			return page, err
		}
	}
	if err := s.reconcile(ctx); err != nil {
		return page, err
	}
	where := "WHERE q.status='pending'"
	args := []any{}
	if archived {
		where = "WHERE q.status!='pending'"
	}
	if who != nil {
		where = "WHERE q.task_id IS NULLIF(?,0)"
		args = append(args, who.TaskID)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_questions WHERE status='pending'`).Scan(&page.PendingCount); err != nil {
		return page, err
	}
	pageSize := 50
	if who != nil {
		pageSize = 10
	}
	args = append(args, pageSize+1, offset)
	rows, err := s.db.QueryContext(ctx, selectQuestion+where+` ORDER BY q.updated_at DESC,q.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, q)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > pageSize {
		page.Items = page.Items[:pageSize]
		page.HasMore = true
	}
	return page, nil
}

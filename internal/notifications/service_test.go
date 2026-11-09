package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/monitor"
	"kamacu/internal/store"
)

type fakeProvider struct {
	mu             sync.Mutex
	batch          Batch
	err            error
	enrichErr      error
	identityErr    error
	identityChecks int
	failIdentityAt int
	enrichCalls    int
	contextText    string
}

func (p *fakeProvider) CheckIdentity(context.Context, Source) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.identityChecks++
	if p.failIdentityAt == 0 || p.identityChecks >= p.failIdentityAt {
		return p.identityErr
	}
	return nil
}
func (p *fakeProvider) Validate(json.RawMessage) error { return nil }
func (p *fakeProvider) Collect(context.Context, Source, map[string]bool) (Batch, error) {
	return p.batch, p.err
}
func (p *fakeProvider) Enrich(_ context.Context, _ Source, e Event) (Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enrichCalls++
	e.Context = "Review requested"
	if p.contextText != "" {
		e.Context = p.contextText
	}
	e.EvidenceComplete = true
	e.SubjectState = "open"
	draft := false
	e.PRFacts = &PRFacts{Available: true, State: "open", Draft: &draft}
	e.SubjectTitle = "Change"
	return e, p.enrichErr
}

type fakeModel struct {
	mu sync.Mutex

	responses  []any
	prompts    []string
	respond    func(string) (any, error)
	onGenerate func(string)
}

func (m *fakeModel) GenerateJSON(_ context.Context, _ monitor.Profile, prompt string, _ any) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, prompt)

	if m.onGenerate != nil {
		m.onGenerate(prompt)
	}
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		return json.Marshal(response)
	}
	if m.respond != nil {
		response, err := m.respond(prompt)
		if err != nil {
			return nil, err
		}
		return json.Marshal(response)
	}

	return nil, errors.New("no model response")
}

func (m *fakeModel) GenerateValidatedJSON(ctx context.Context, p monitor.Profile, prompt string, schema any, validate func([]byte) error) ([]byte, error) {
	raw, err := m.GenerateJSON(ctx, p, prompt, schema)
	if err == nil && validate != nil {
		err = validate(raw)
	}
	return raw, err
}

type fakeTools struct {
	mu        sync.Mutex
	calls     []string
	arguments []json.RawMessage
	err       error
	result    string
	results   map[string]string
}

func (f *fakeTools) List(context.Context) ([]Tool, error) {
	return []Tool{{Name: "send_session_message"}, {Name: "start_task_agent"}, {Name: "open_review"}, {Name: "create_task"}, {Name: "delete_task"}, {Name: "move_task"}, {Name: "list_tasks"}, {Name: "list_sessions"}, {Name: "get_session"}, {Name: "get_session_output"}}, nil
}
func (f *fakeTools) Call(_ context.Context, name string, args json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	f.arguments = append(f.arguments, append(json.RawMessage(nil), args...))
	if result, ok := f.results[name]; ok {
		return result, f.err
	}
	return f.result, f.err
}
func fixture(t *testing.T) (*Service, *fakeModel, *fakeTools, Source) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO projects(id,name,repo_path,github_repo) VALUES(1,'Repo','/repo','acme/repo'),(2,'Other','/other','acme/other')`, `INSERT INTO tasks(id,project_id,title,status,position) VALUES(1,1,'Implement','in_progress',1),(2,2,'Other','todo',1)`, `INSERT INTO monitor_profiles(id,name,backend,model,endpoint) VALUES(1,'Global','openai_compatible','test','https://model.test/v1')`, `UPDATE monitor_settings SET default_profile_id=1`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{} })
	t.Cleanup(box.Close)
	model := &fakeModel{}
	tools := &fakeTools{result: `{"bytes_written":20}`, results: map[string]string{"open_review": `{"task":{"id":1},"pr":{"number":7}}`, "create_task": `{"id":3}`}}
	s := New(db, box, model, tools)
	s.SessionTask = func(id string) (int64, bool) {
		if id == "correct" {
			return 1, true
		}
		if id == "other" {
			return 2, true
		}
		return 0, false
	}
	source, err := s.AddSource(context.Background(), "github", "GitHub", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	s.Providers["github"] = &fakeProvider{}
	configureTestModel(t, s)
	return s, model, tools, source
}
func event() Event {
	return Event{ExternalID: "thread-1", Version: "v1", Repository: "acme/repo", Title: "Change", URL: "https://github.com/acme/repo/pull/7", SubjectType: "PullRequest", Context: "Review requested"}
}
func seed(t *testing.T, s *Service, source Source, e Event) int64 {
	t.Helper()
	if err := s.ingest(context.Background(), source, Batch{Events: []Event{e}, Account: "alice", Checkpoint: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.DB.QueryRow(`SELECT id FROM notification_events WHERE external_id=? AND version=?`, e.ExternalID, e.Version).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func ask() inbox.Create {
	return inbox.Create{Abstract: "New review feedback", Summary: "New review feedback", Context: "A reviewer requested changes.", Question: "Ask the implementation session to check and fix?", Options: []inbox.Option{{ID: "reply_1", Label: "Check and fix"}, {ID: "reply_2", Label: "Summarize first"}}}
}
func publish(t *testing.T, s *Service, id int64) inbox.Question {
	t.Helper()
	q, err := s.Inbox.PublishNotification(context.Background(), id, "GitHub", event().URL, ask())
	if err != nil {
		t.Fatal(err)
	}
	attachTestPlan(t, s, q.ID)
	return q
}

func TestCollectionDedupPauseAndFailedCheckpoint(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	seed(t, s, source, event())
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_events`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate ingestion")
	}
	sources, _ := s.Sources(ctx)
	checkpoint := sources[0].Checkpoint
	s.DB.Exec(`UPDATE notification_sources SET next_poll=0`)
	s.Providers["github"] = &fakeProvider{err: errors.New("offline")}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	sources, _ = s.Sources(ctx)
	if sources[0].Checkpoint != checkpoint || sources[0].LastError != "offline" {
		t.Fatalf("failed poll: %+v", sources)
	}
	if err := s.EnableSource(ctx, source.ID, false); err != nil {
		t.Fatal(err)
	}
	e := event()
	e.Version = "v2"
	if err := s.ingest(ctx, source, Batch{Events: []Event{e}}); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT count(*) FROM notification_events`).Scan(&count)
	if count != 1 {
		t.Fatal("imported while paused")
	}
	if err := s.EnableSource(ctx, "missing", true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestContinuePreservesAnswersAndNewerPendingDecision(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Proceed", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inbox.ContinueNotification(ctx, q.ID); !errors.Is(err, inbox.ErrConflict) {
		t.Fatal("allowed continuation while queued", err)
	}
	if err := s.finishRun(ctx, q.ID, "failed", "No action was taken.", nil); err != nil {
		t.Fatal(err)
	}
	next, err := s.Inbox.ContinueNotification(ctx, q.ID)
	if err != nil || next.ID == q.ID || !strings.Contains(next.Context, "No action was taken") {
		t.Fatalf("continue: %+v %v", next, err)
	}
	again, err := s.Inbox.ContinueNotification(ctx, q.ID)
	if err != nil || again.ID != next.ID || again.Revision != next.Revision {
		t.Fatalf("replaced pending: %+v %v", again, err)
	}
	original, _ := s.Inbox.Get(ctx, q.ID)
	if original.Answer != "Proceed" {
		t.Fatal("rewrote original answer")
	}
}

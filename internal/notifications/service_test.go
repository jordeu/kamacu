package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/monitor"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

type fakeProvider struct {
	batch          Batch
	err            error
	enrichErr      error
	identityErr    error
	identityChecks int
	failIdentityAt int
	enrichCalls    int
}

func (p *fakeProvider) CheckIdentity(context.Context, Source) error {
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
	p.enrichCalls++
	e.Context = "Review requested"
	return e, p.enrichErr
}

type fakeModel struct {
	steps      []Step
	responses  []any
	prompts    []string
	onGenerate func(string)
}

func (m *fakeModel) GenerateJSON(_ context.Context, _ monitor.Profile, prompt string, _ any) ([]byte, error) {
	m.prompts = append(m.prompts, prompt)
	if m.onGenerate != nil {
		m.onGenerate(prompt)
	}
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		return json.Marshal(response)
	}
	if len(m.steps) == 0 {
		return nil, errors.New("no model response")
	}
	step := m.steps[0]
	m.steps = m.steps[1:]
	return json.Marshal(step)
}

type fakeTools struct {
	calls  []string
	err    error
	result string
}

func (f *fakeTools) List(context.Context) ([]Tool, error) {
	return []Tool{{Name: "send_session_message"}}, nil
}
func (f *fakeTools) Call(_ context.Context, name string, _ json.RawMessage) (string, error) {
	f.calls = append(f.calls, name)
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
	tools := &fakeTools{result: `{"bytes_written":20}`}
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
func ask() Step {
	return Step{Action: "ask", Summary: "New review feedback", Context: "A reviewer requested changes.", Question: "Ask the implementation session to check and fix?", Options: []string{"Check and fix", "Summarize first"}}
}
func publish(t *testing.T, s *Service, id int64) inbox.Question {
	t.Helper()
	q, err := s.Inbox.PublishNotification(context.Background(), id, "GitHub", event().URL, ask().question())
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestExecutionUsesMCPAndRecordsOutcome(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if err := settings.Set(s.DB, settings.KeyNotificationPrompt, "Prefer the existing implementation session."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Check and fix", ""); err != nil {
		t.Fatal(err)
	}
	model.steps = []Step{{Action: "tool", ToolName: "send_session_message", Arguments: `{"session_id":"correct","message":"Check and fix review feedback on https://github.com/acme/repo/pull/7"}`}, {Action: "done", Message: "Instructions sent to the implementation session."}}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(ctx, q.ID)
	if got.DeliveryStatus != "succeeded" || !strings.Contains(got.ExecutionResult, "Instructions sent") {
		t.Fatalf("outcome: %+v", got)
	}
	if len(tools.calls) != 1 || !strings.Contains(model.prompts[0], "Prefer the existing implementation session.") {
		t.Fatal("missing tool call or custom prompt")
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 1 {
		t.Fatal("execution replayed")
	}
}
func TestExecutionFollowupAndRestartRecovery(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Which session?", ""); err != nil {
		t.Fatal(err)
	}
	model.steps = []Step{ask()}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(page.Items) != 1 || page.Items[0].ID == q.ID {
		t.Fatalf("followup: %+v", page)
	}
	followup := page.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, followup.ID, followup.Revision, "Proceed", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?`, followup.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(ctx, followup.ID)
	if got.DeliveryStatus != "uncertain" || len(tools.calls) != 0 {
		t.Fatalf("recovery replayed: %+v", got)
	}
}
func TestExecutionNeverRetriesFailedMutation(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Proceed", "")
	tools.err = errors.New("connection lost")
	model.steps = []Step{{Action: "tool", ToolName: "send_session_message", Arguments: `{"session_id":"correct","message":"Check feedback"}`}}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(ctx, q.ID)
	if len(tools.calls) != 1 || got.DeliveryStatus != "uncertain" {
		t.Fatalf("unsafe retry: %+v %v", got, tools.calls)
	}
}
func TestToolRestrictions(t *testing.T) {
	s, _, _, _ := fixture(t)
	for _, tc := range []struct{ name, args string }{
		{"delete_task", `{"task_id":1}`}, {"post_pr_review", `{}`}, {"start_task_agent", `{"task_id":2}`},
		{"send_session_message", `{"session_id":"other"}`}, {"send_session_message", `{"session_id":"stopped"}`},
		{"open_review", `{"project_id":1,"pr_number":8}`}, {"list_sessions", `{}`},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			if err := s.validateTool(context.Background(), event(), tc.name, json.RawMessage(tc.args)); err == nil {
				t.Fatal("accepted out-of-scope tool")
			}
		})
	}
	if err := s.validateTool(context.Background(), event(), "open_review", json.RawMessage(`{"project_id":1,"pr_number":7}`)); err != nil {
		t.Fatal(err)
	}
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
func TestDuplicateMutationIsRejectedWithinRun(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Fix feedback", ""); err != nil {
		t.Fatal(err)
	}
	step := Step{Action: "tool", ToolName: "send_session_message", Arguments: `{"session_id":"correct","message":"Check feedback"}`}
	model.steps = []Step{step, step}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(ctx, q.ID)
	if len(tools.calls) != 1 || got.DeliveryStatus != "uncertain" {
		t.Fatalf("repeated action: %+v %v", got, tools.calls)
	}
}

func TestExecutionRevalidatesAccountBeforeEnrichmentAndEveryMutation(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			s, model, tools, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			mismatch := errors.New("active gh account changed")
			provider := &fakeProvider{identityErr: mismatch, failIdentityAt: failAt}
			s.Providers["github"] = provider
			if failAt == 1 {
				if _, err := s.DB.Exec(`UPDATE notification_sources SET last_error=? WHERE id=?`, mismatch.Error(), source.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Start review", ""); err != nil {
				t.Fatal(err)
			}
			model.steps = []Step{{Action: "tool", ToolName: "open_review", Arguments: `{"project_id":1,"pr_number":7}`}, {Action: "tool", ToolName: "start_task_agent", Arguments: `{"task_id":1}`}}
			if err := s.executeNext(ctx); err != nil {
				t.Fatal(err)
			}
			expectedCalls := 0
			if failAt == 3 {
				expectedCalls = 1
			}
			if len(tools.calls) != expectedCalls {
				t.Fatalf("actions after account change: %v", tools.calls)
			}
			if failAt == 1 && (provider.enrichCalls != 0 || len(model.prompts) != 0) {
				t.Fatal("enriched or inferred with wrong account")
			}
			got, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			expectedState := "failed"
			if expectedCalls > 0 {
				expectedState = "uncertain"
			}
			if got.DeliveryStatus != expectedState || !strings.Contains(got.ExecutionResult, mismatch.Error()) {
				t.Fatalf("outcome: %+v", got)
			}
			sources, err := s.Sources(ctx)
			if err != nil || sources[0].LastError != mismatch.Error() {
				t.Fatalf("source health: %+v %v", sources, err)
			}
		})
	}
}
func TestReplyPromptRetainsDisplayedContextAndPriorToolResults(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	first := ask()
	first.Context = "Session previous-session-ABC was selected for the implementation."
	q, err := s.Inbox.PublishNotification(ctx, id, "GitHub", event().URL, first.question())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Prepare it", ""); err != nil {
		t.Fatal(err)
	}
	history := []trace{{Tool: "start_task_agent", Result: `{"id":"new-session-XYZ","taskId":1}`}}
	if err = s.finishRun(ctx, q.ID, "succeeded", "A follow-up is needed.", history); err != nil {
		t.Fatal(err)
	}
	followup := ask()
	followup.Summary = "Choose the prepared session"
	followup.Context = "The first session is new-session-XYZ; the second is previous-session-ABC."
	followup.Options = []string{"Use the first session", "Use the second session"}
	next, err := s.Inbox.PublishNotification(ctx, id, "GitHub", event().URL, followup.question())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Inbox.Answer(ctx, nil, next.ID, next.Revision, "Use the first session", ""); err != nil {
		t.Fatal(err)
	}
	model.steps = []Step{{Action: "done", Message: "Ready"}}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	prompt := model.prompts[0]
	for _, required := range []string{first.Context, followup.Context, followup.Summary, "Use the first session", "execution_history", "new-session-XYZ", "start_task_agent"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("missing reply context %q", required)
		}
	}
	if strings.Index(prompt, q.ID) > strings.Index(prompt, "CURRENT QUESTION AND USER REPLY") {
		t.Fatal("prior turn missing from conversation")
	}
}

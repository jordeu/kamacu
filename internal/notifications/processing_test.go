package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"kamacu/internal/inbox"
)

func actionFor(ids ...int64) BatchAction {
	a := ask()
	return BatchAction{EventIDs: ids, Summary: a.Summary, Context: a.Context, Question: a.Question, Options: a.Options}
}

// Fixtures emit the new evidence-backed intent contract, never planner output.
func respondWithIntent(model *fakeModel, kind string) {
	model.respond = func(prompt string) (any, error) {
		if _, raw, ok := strings.Cut(prompt, projectEvidenceMarker); ok {
			raw, _, _ = strings.Cut(raw, "\nCORRECTION")
			var payload struct {
				Subjects []projectSubjectData `json:"subjects"`
			}
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				return nil, err
			}
			subjects := []map[string]any{}
			for _, subject := range payload.Subjects {
				item := subject.Evidence[0]
				quote := item.Text
				if len(quote) > 100 {
					quote = strings.ToValidUTF8(quote[:100], "")
				}
				subjects = append(subjects, map[string]any{"subject_id": subject.ID, "findings": []intentFinding{{Intent: kind, EvidenceID: item.ID, Quote: quote}}})
			}
			return map[string]any{"subjects": subjects}, nil
		}
		if strings.Contains(prompt, projectCardsMarker) {
			return fixtureProjectCards(prompt, nil)
		}
		_, raw, ok := strings.Cut(prompt, "\nEVIDENCE (data):\n")
		if !ok {
			return nil, errors.New("missing classifier evidence")
		}
		var payload struct {
			Evidence []SubjectEvidence `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
		if len(payload.Evidence) == 0 {
			return nil, errors.New("empty evidence")
		}
		item := payload.Evidence[0]
		quote := item.Text
		if len(quote) > 100 {
			quote = strings.ToValidUTF8(quote[:100], "")
		}
		return intentResponse{Findings: []intentFinding{{Intent: kind, EvidenceID: item.ID, Quote: quote}}}, nil
	}
}
func process(t *testing.T, s *Service, model *fakeModel, kind string) Processing {
	t.Helper()
	ctx := context.Background()
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if model.respond == nil && len(model.responses) == 0 {
		respondWithIntent(model, kind)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return *status.Processing
}

func TestTriageGroupsSubjectRevisionsAndProjectReviews(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{groupTestRule()})
	first := event()
	id1 := seed(t, s, source, first)
	first.Version = "v2"
	id2 := seed(t, s, source, first)
	other := event()
	other.ExternalID = "other"
	other.URL = "https://github.com/acme/repo/pull/8"
	id3 := seed(t, s, source, other)
	tools.result = `[]`
	status := process(t, s, model, "review_request")
	if status.Status != "completed" || status.ActionCount != 1 || len(model.prompts) != 2 {
		t.Fatalf("%+v calls=%d", status, len(model.prompts))
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, page.Items[0].ID)
	if err != nil || len(ids) != 3 {
		t.Fatalf("%v %v", ids, err)
	}
	for _, id := range []int64{id1, id2, id3} {
		found := false
		for _, got := range ids {
			found = found || got == id
		}
		if !found {
			t.Fatal("lost provenance", id)
		}
	}
	for _, prompt := range model.prompts {
		if strings.Contains(prompt, "AVAILABLE MCP TOOLS") {
			t.Fatal("classifier received tool planning context")
		}
	}
	for _, call := range tools.calls {
		if call != "list_sessions" {
			t.Fatal("unexpected triage tool", call)
		}
	}
	if !strings.Contains(model.prompts[1], `"workflow":"clarify"`) {
		t.Fatal("lost project workflow")
	}
}

func TestTriageMultipleIntentsShareOneSubjectCard(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.responses = []any{intentResponse{Findings: []intentFinding{{Intent: "review_feedback", EvidenceID: "context", Quote: "Review requested"}, {Intent: "direct_question", EvidenceID: "context", Quote: "Review requested"}}}}
	status := process(t, s, model, "")
	if status.Status != "completed" || status.ActionCount != 1 {
		t.Fatalf("%+v", status)
	}
	page, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	var plan string
	if err := s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, page.Items[0].ID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, `"intent":"review_feedback"`) || !strings.Contains(plan, `"intent":"direct_question"`) {
		t.Fatal("card lost typed activity provenance")
	}

}

func TestTriageInvalidOutputLeavesSnapshotPending(t *testing.T) {
	for _, response := range []any{
		intentResponse{},
		map[string]any{"tool_name": "delete_task", "arguments": "{}"},
		intentResponse{Findings: []intentFinding{{Intent: "routine", EvidenceID: "invented", Quote: "Review requested"}}},
		intentResponse{Findings: []intentFinding{{Intent: "routine", EvidenceID: "context", Quote: "invented"}}},
		intentResponse{Findings: []intentFinding{{Intent: "policy_ignore", EvidenceID: "context", Quote: "Review requested"}}},
	} {
		t.Run(fmt.Sprintf("%v", response), func(t *testing.T) {
			s, model, _, source := fixture(t)
			seed(t, s, source, event())
			model.responses = []any{response}
			status := process(t, s, model, "")
			if status.Status != "failed" {
				t.Fatal(status)
			}
			raw, _ := s.ListRaw(context.Background(), "pending", 0)
			if raw.PendingCount != 1 {
				t.Fatal("invalid output archived snapshot")
			}
		})
	}
}

func TestTriageDoubleClickAndNewArrivals(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	first, err := s.RequestProcessing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RequestProcessing(ctx)
	if err != nil || first.ID != second.ID {
		t.Fatal("double click created new snapshot")
	}
	later := event()
	later.ExternalID = "new"
	id := seed(t, s, source, later)
	respondWithIntent(model, "routine")
	if err = s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	raw, _ := s.ListRaw(ctx, "pending", 0)
	if raw.PendingCount != 1 || raw.Items[0].ID != id {
		t.Fatal("consumed new arrival", raw)
	}
}

func TestTriageRejectsConcurrentRelatedInboxChanges(t *testing.T) {
	for _, change := range []string{"answer", "followup", "update"} {
		t.Run(change, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			update := event()
			update.Version = "v2"
			seed(t, s, source, update)
			done := false
			model.onGenerate = func(string) {
				if done {
					return
				}
				done = true
				switch change {
				case "answer":
					if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Proceed", ""); err != nil {
						t.Fatal(err)
					}
				case "followup":
					if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Explain", ""); err != nil {
						t.Fatal(err)
					}
					if _, err := s.Inbox.PublishNotificationFollowup(ctx, q.ID, ask().question()); err != nil {
						t.Fatal(err)
					}
				case "update":
					c := ask().question()
					c.Context = "Changed during inference"
					if _, err := s.Inbox.PublishNotification(ctx, q.NotificationID, "GitHub", event().URL, c); err != nil {
						t.Fatal(err)
					}
				}
			}
			status := process(t, s, model, "direct_question")
			if status.Status != "failed" {
				t.Fatalf("concurrent change accepted: %+v", status)
			}
			raw, _ := s.ListRaw(ctx, "pending", 0)
			if raw.PendingCount != 2 {
				t.Fatal("conflict archived raw evidence")
			}
		})
	}
}

func TestTriageLaterRepositoryFailureDoesNotPartiallyPublish(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	other := event()
	other.ExternalID = "other"
	other.Repository = "acme/other"
	other.URL = "https://github.com/acme/other/pull/9"
	seed(t, s, source, other)
	model.responses = []any{intentResponse{Findings: []intentFinding{{Intent: "direct_question", EvidenceID: "context", Quote: "Review requested"}}}, intentResponse{}}
	status := process(t, s, model, "")
	if status.Status != "failed" {
		t.Fatal(status)
	}
	raw, _ := s.ListRaw(context.Background(), "pending", 0)
	page, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if raw.PendingCount != 2 || len(page.Items) != 0 {
		t.Fatal("partial publication")
	}
}

func TestTriagePreservesExistingTemplateQuestion(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if status := process(t, s, model, "direct_question"); status.Status != "completed" {
		t.Fatal(status)
	}
	before, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	update := event()
	update.Version = "v2"
	seed(t, s, source, update)
	if status := process(t, s, model, "direct_question"); status.Status != "completed" || status.ActionCount != 0 {
		t.Fatal(status)
	}
	after, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(after.Items) != 1 || after.Items[0].ID != before.Items[0].ID || after.Items[0].Revision != before.Items[0].Revision {
		t.Fatal("duplicate replaced useful question")
	}
}

func TestTriageUnknownContextCannotBeSilentlyDiscarded(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	s.Providers["github"] = &fakeProvider{enrichErr: ErrContextUnavailable}
	status := process(t, s, model, "routine")
	if status.Status != "completed" || status.ActionCount != 1 {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if !strings.Contains(page.Items[0].Context, "Current subject context is unavailable") {
		t.Fatal("missing uncertainty")
	}
}

type changingFactsProvider struct {
	*fakeProvider
	facts   PRFacts
	onFacts func(*PRFacts)
}

func (p *changingFactsProvider) PRContext(_ context.Context, _ Source, repo string, n int64) PRFacts {
	if p.onFacts != nil {
		p.onFacts(&p.facts)
	}
	f := p.facts
	f.Repository = repo
	f.Number = n
	return f
}

func TestTriageRefreshesFactsBeforePublication(t *testing.T) {
	s, model, _, source := fixture(t)
	e := event()
	e.Reason = "review_requested"
	seed(t, s, source, e)
	p := &changingFactsProvider{fakeProvider: &fakeProvider{}, facts: PRFacts{Available: true, State: "open"}}
	s.Providers["github"] = p
	model.onGenerate = func(string) { p.facts.State = "merged" }
	status := process(t, s, model, "review_request")
	if status.Status != "completed" || status.ActionCount != 0 {
		t.Fatal("published stale review request", status)
	}
}

func TestTriageBatchValidationPreservesMembershipAndRevisions(t *testing.T) {
	events := []RawNotification{{ID: 1}, {ID: 2}}
	for _, d := range []BatchDecision{{Actions: []BatchAction{actionFor(1)}}, {DiscardIDs: []int64{1, 2, 3}}, {Actions: []BatchAction{actionFor(1)}, DiscardIDs: []int64{1, 2}}, {DiscardIDs: []int64{1, 2}, SupersedeIDs: []string{"unknown"}}} {
		if _, _, err := validateBatch(d, events, nil); err == nil {
			t.Fatal("invalid batch accepted", d)
		}
	}
	_, refs, err := validateBatch(BatchDecision{DiscardIDs: []int64{1, 2}, SupersedeIDs: []string{"q"}}, events, []inbox.Question{{ID: "q", Revision: 7}})
	if err != nil || len(refs) != 1 || refs[0].Revision != 7 {
		t.Fatal("lost revision", refs, err)
	}
}

func TestTriageIdentityMismatchDoesNotPublish(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	p := &fakeProvider{identityErr: errors.New("account changed"), failIdentityAt: 2}
	s.Providers["github"] = p
	status := process(t, s, model, "direct_question")
	if status.Status != "failed" {
		t.Fatal("identity mismatch accepted")
	}
}

func TestCollectionDoesNotRunLLMOrEnrich(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	p := &fakeProvider{batch: Batch{Events: []Event{event()}, Account: "alice", Checkpoint: source.Checkpoint}}
	s.Providers["github"] = p
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 0 || p.enrichCalls != 0 {
		t.Fatal("collection invoked inference or enrichment")
	}
	raw, err := s.ListRaw(ctx, "pending", 0)
	if err != nil || len(raw.Items) != 1 || raw.PendingCount != 1 {
		t.Fatalf("raw: %+v %v", raw, err)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 0 {
		t.Fatal("collection created inbox cards")
	}
}
func TestManualArchiveAndPaginationDoNotUseModel(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	for i := range 52 {
		e := event()
		e.ExternalID = fmt.Sprint(i)
		seed(t, s, source, e)
	}
	page, err := s.ListRaw(ctx, "pending", 0)
	if err != nil || len(page.Items) != 50 || !page.HasMore || page.PendingCount != 52 {
		t.Fatalf("page: %+v %v", page, err)
	}
	id := page.Items[0].ID
	if err = s.SetRawStatus(ctx, id, "archived"); err != nil {
		t.Fatal(err)
	}
	archived, _ := s.ListRaw(ctx, "archived", 0)
	if len(archived.Items) != 1 || archived.PendingCount != 51 {
		t.Fatal("archive failed")
	}
	if err = s.SetRawStatus(ctx, id, "pending"); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 0 {
		t.Fatal("listing or archiving used the model")
	}
}
func TestManualNotificationMigrationPreservesPreviouslyProcessedHistory(t *testing.T) {
	s, _, _, source := fixture(t)
	migration, err := os.ReadFile("../store/migrations/00032_manual_notification_processing.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(migration), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal("rollback fixture to previous schema", err)
	}
	for _, state := range []string{"queued", "ignored", "published"} {
		e := event()
		e.ExternalID = state
		raw, _ := json.Marshal(e)
		if _, err = s.DB.Exec(`INSERT INTO notification_events(source_id,external_id,version,payload,state,created_at) VALUES(?,?,?, ?,?,0)`, source.ID, state, e.Version, string(raw), state); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal("apply migration", err)
	}
	// Inspect the migration's schema directly: newer ListRaw also requires the
	// rule snapshot column, which this historical migration does not create.
	var pending, archived int
	if err = s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='pending' AND external_id='queued'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending after upgrade: %d %v", pending, err)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='archived'`).Scan(&archived); err != nil || archived != 2 {
		t.Fatalf("history after upgrade: %d %v", archived, err)
	}
}
func TestQuestionSnapshotStatusMigrationPreservesQueuedRequests(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	p, err := s.RequestProcessing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`ALTER TABLE notification_batch_questions DROP COLUMN status`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../store/migrations/00033_notification_question_snapshot_status.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	if _, err = s.DB.Exec(up); err != nil {
		t.Fatal(err)
	}
	var captured string
	if err = s.DB.QueryRow(`SELECT status FROM notification_batch_questions WHERE batch_id=? AND question_id=?`, p.ID, q.ID).Scan(&captured); err != nil || captured != "pending" {
		t.Fatalf("queued snapshot changed on upgrade: %s %v", captured, err)
	}
	respondWithIntent(model, "routine")
	if err = s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" {
		t.Fatal(status.Processing.Error)
	}
}

func TestHardEnrichmentFailuresRemainFatal(t *testing.T) {
	for _, cause := range []error{&GitHubHTTPError{StatusCode: 401}, &GitHubHTTPError{StatusCode: 403}, &GitHubHTTPError{StatusCode: 429}, errors.New("transport unavailable"), context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			seed(t, s, source, event())
			s.Providers["github"] = &fakeProvider{enrichErr: cause}
			p := process(t, s, model, "direct_question")
			if p.Status != "failed" || len(model.prompts) != 0 {
				t.Fatal("hard failure was treated as partial evidence")
			}
			raw, _ := s.ListRaw(ctx, "pending", 0)
			if len(raw.Items) != 1 {
				t.Fatal("hard failure consumed notification")
			}
		})
	}
}

func TestTypedNoopSkipsEnrichmentToolsAndInference(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if status := process(t, s, model, "needs_context"); status.Status != "completed" {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	p := s.Providers["github"].(*fakeProvider)
	before := p.enrichCalls
	model.prompts = nil
	model.respond = nil
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Inbox.Get(ctx, q.ID)
	if p.enrichCalls != before || len(model.prompts) != 0 || len(tools.calls) != 0 || after.DeliveryStatus != "succeeded" {
		t.Fatalf("no-op invoked work: %+v", after)
	}
}

func TestFreeTextNoopPhraseDoesNotBypassExecution(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if status := process(t, s, model, "needs_context"); status.Status != "completed" {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	model.prompts = nil
	model.respond = nil
	model.steps = []Step{{Action: "done", Message: "Investigated the second instruction"}}
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Leave this activity alone, but inspect the other issue", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 2 {
		t.Fatal("free text did not go through reply planning and execution")
	}
}

func TestPostClassificationRulesDoNotSuppressMergedFeedback(t *testing.T) {
	e := event()
	e.Reason = "subscribed"
	e.EvidenceComplete = true
	e.PRFacts = &PRFacts{Available: true, State: "merged"}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, viewer: "alice", findings: []intentFinding{{Intent: "review_request"}, {Intent: "review_feedback"}}}
	if err := func() error {
		s, _, _, _ := fixture(t)
		return s.evaluateActivities(context.Background(), []*triageSubject{subject}, RuleSet{})
	}(); err != nil {
		t.Fatal(err)
	}
	if len(subject.findings) != 1 || subject.findings[0].Intent != "review_feedback" || subject.reason != "" {
		t.Fatal("suppressed feedback with obsolete request", subject)
	}
}

func TestNewFeedbackReplacesSingleSubjectTemplate(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if status := process(t, s, model, "review_feedback"); status.Status != "completed" {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	previous := page.Items[0]
	update := event()
	update.Version = "v2"
	seed(t, s, source, update)
	s.Providers["github"].(*fakeProvider).contextText = "Please fix the new race"
	if status := process(t, s, model, "review_feedback"); status.Status != "completed" || status.ActionCount != 1 {
		t.Fatal(status)
	}
	old, _ := s.Inbox.Get(ctx, previous.ID)
	if old.Reason != "notification_superseded" {
		t.Fatal("old feedback card remained pending", old)
	}
}

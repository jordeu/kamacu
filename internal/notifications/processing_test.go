package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func actionFor(ids ...int64) BatchAction {
	a := ask()
	return BatchAction{EventIDs: ids, Summary: a.Summary, Context: a.Context, Question: a.Question, Options: a.Options}
}
func process(t *testing.T, s *Service, model *fakeModel, d BatchDecision) Processing {
	t.Helper()
	p, err := s.RequestProcessing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	model.responses = []any{d}
	if err = s.processNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Processing.ID != p.ID {
		t.Fatal("unexpected processing id")
	}
	return *status.Processing
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
func TestManualProcessingGroupsAllPendingAndDiscardsObsoleteVersions(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	old := event()
	old.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	id1 := seed(t, s, source, old)
	update := old
	update.Version = "v2"
	update.Title = "More review feedback"
	id2 := seed(t, s, source, update)
	merged := old
	merged.ExternalID = "merged-pr"
	merged.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/8"
	merged.URL = "https://github.com/acme/repo/pull/8"
	merged.Title = "Merged PR"
	id3 := seed(t, s, source, merged)
	raw, _ := s.ListRaw(ctx, "pending", 0)
	if len(raw.Items) != 3 {
		t.Fatal("collection automatically discarded an older notification")
	}
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id1, id2)}, DiscardIDs: []int64{id3}})
	if p.Status != "completed" || p.ActionCount != 1 || p.NotificationCount != 3 {
		t.Fatalf("processing: %+v", p)
	}
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || len(cards.Items) != 1 {
		t.Fatalf("cards: %+v %v", cards, err)
	}
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, cards.Items[0].ID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("provenance: %v %v", ids, err)
	}
	raw, _ = s.ListRaw(ctx, "pending", 0)
	archived, _ := s.ListRaw(ctx, "archived", 0)
	if raw.PendingCount != 0 || len(archived.Items) != 3 {
		t.Fatal("processed snapshot was not archived")
	}
	for _, id := range []int64{id1, id2, id3} {
		if !strings.Contains(model.prompts[0], fmt.Sprintf(`"id":%d`, id)) {
			t.Fatalf("model did not see notification %d", id)
		}
	}
	if _, err = s.RequestProcessing(ctx); !errors.Is(err, ErrNoPending) {
		t.Fatal("reprocessed archived notifications", err)
	}
}
func TestSeveralActionsFromSameThreadRemainIndependent(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	a, b := actionFor(id), actionFor(id)
	a.Question = "Review this PR?"
	b.Question = "Ask the implementation session to fix feedback?"
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{a, b}})
	if p.Status != "completed" {
		t.Fatal(p.Error)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 2 || cards.Items[0].ID == cards.Items[1].ID {
		t.Fatalf("distinct actions coalesced: %+v", cards)
	}
	first, second := cards.Items[0], cards.Items[1]
	if _, err := s.Inbox.Answer(ctx, nil, first.ID, first.Revision, "Explain first", ""); err != nil {
		t.Fatal(err)
	}
	followup := ask()
	followup.Context = "Follow-up for the first action only"
	model.steps = []Step{followup}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	other, _ := s.Inbox.Get(ctx, second.ID)
	if other.Question != second.Question || other.Revision != second.Revision {
		t.Fatal("follow-up overwrote the other action")
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(page.Items) != 2 {
		t.Fatalf("followup cards: %+v", page)
	}
}
func TestGroupedReplyAllowsEveryRelatedTargetAndPreservesProvenance(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	first := event()
	first.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	id1 := seed(t, s, source, first)
	second := event()
	second.ExternalID = "other-repo"
	second.Repository = "acme/other"
	second.URL = "https://github.com/acme/other/pull/9"
	second.SubjectURL = "https://api.github.com/repos/acme/other/pulls/9"
	id2 := seed(t, s, source, second)
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id1, id2)}})
	if p.Status != "completed" {
		t.Fatal(p.Error)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	related, err := s.loadActionContext(ctx, q.ID)
	if err != nil || len(related.Events) != 2 {
		t.Fatalf("context: %+v %v", related, err)
	}
	for _, args := range []string{`{"project_id":1,"pr_number":7}`, `{"project_id":2,"pr_number":9}`} {
		if err = s.validateActionTool(ctx, related.Events, "open_review", json.RawMessage(args)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.validateActionTool(ctx, related.Events, "open_review", json.RawMessage(`{"project_id":2,"pr_number":7}`)); err == nil {
		t.Fatal("allowed unrelated PR")
	}
	if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Which target first?", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.finishRun(ctx, q.ID, "succeeded", "Please clarify", nil); err != nil {
		t.Fatal(err)
	}
	next, err := s.Inbox.ContinueNotification(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, next.ID)
	if err != nil || len(ids) != 2 {
		t.Fatal("continuation lost group provenance", ids, err)
	}
}
func TestProcessingSnapshotSurvivesDoubleClickAndLeavesNewArrivalsPending(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id1 := seed(t, s, source, event())
	p, err := s.RequestProcessing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.RequestProcessing(ctx)
	if err != nil || again.ID != p.ID {
		t.Fatal("double click created another run")
	}
	if err = s.SetRawStatus(ctx, id1, "archived"); !errors.Is(err, ErrProcessing) {
		t.Fatal("changed an active snapshot", err)
	}
	later := event()
	later.ExternalID = "new-arrival"
	id2 := seed(t, s, source, later)
	model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(id1)}}}
	if err = s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	raw, _ := s.ListRaw(ctx, "pending", 0)
	if len(raw.Items) != 1 || raw.Items[0].ID != id2 {
		t.Fatalf("new arrivals were consumed: %+v", raw)
	}
}
func TestInvalidOrInterruptedBatchLeavesEverythingPending(t *testing.T) {
	for _, kind := range []string{"missing", "unknown", "overlap", "interrupted", "model_failure"} {
		t.Run(kind, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			id := seed(t, s, source, event())
			p, err := s.RequestProcessing(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				model.responses = []any{BatchDecision{}}
			case "unknown":
				model.responses = []any{BatchDecision{DiscardIDs: []int64{id + 100}}}
			case "overlap":
				model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(id)}, DiscardIDs: []int64{id}}}
			case "interrupted":
				s.DB.Exec(`UPDATE notification_batches SET status='running' WHERE id=?`, p.ID)
				if err = s.Recover(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.processNext(ctx); err != nil {
				t.Fatal(err)
			}
			status, _ := s.Status(ctx)
			if status.Processing.Status != "failed" || status.Processing.Error == "" {
				t.Fatalf("failure not visible: %+v", status)
			}
			raw, _ := s.ListRaw(ctx, "pending", 0)
			cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if len(raw.Items) != 1 || len(cards.Items) != 0 {
				t.Fatal("failed run partially committed")
			}
			retry, err := s.RequestProcessing(ctx)
			if err != nil || retry.ID == p.ID {
				t.Fatal("failed run cannot be retried manually", err)
			}
		})
	}
}
func TestBatchSupersedesObsoleteCardsAndPreservesConcurrentReplies(t *testing.T) {
	for _, change := range []string{"none", "answered", "changed"} {
		t.Run(change, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			update := event()
			update.Version = "v2"
			id := seed(t, s, source, update)
			p, err := s.RequestProcessing(ctx)
			if err != nil {
				t.Fatal(err)
			}
			model.onGenerate = func(string) {
				if change == "answered" {
					if _, e := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Proceed", ""); e != nil {
						t.Fatal(e)
					}
				}
				if change == "changed" {
					c := ask().question()
					c.Context = "Updated while processing"
					if _, e := s.Inbox.PublishNotification(ctx, id, "GitHub", event().URL, c); e != nil {
						t.Fatal(e)
					}
				}
			}
			model.responses = []any{BatchDecision{DiscardIDs: []int64{q.NotificationID, id}, SupersedeIDs: []string{q.ID}}}
			if err = s.processNext(ctx); err != nil {
				t.Fatal(err)
			}
			status, _ := s.Status(ctx)
			old, _ := s.Inbox.Get(ctx, q.ID)
			switch change {
			case "none":
				if status.Processing.Status != "completed" || old.Reason != "notification_superseded" || old.Status != "unanswered" {
					t.Fatalf("obsolete card: %+v %+v", status, old)
				}
			case "answered":
				if status.Processing.Status != "failed" || old.Status != "answered" || old.Answer != "Proceed" {
					t.Fatal("overwrote concurrent answer")
				}
			case "changed":
				if status.Processing.Status != "failed" || old.Status != "pending" || old.Revision != q.Revision+1 {
					t.Fatal("overwrote changed decision")
				}
			}
			if !strings.Contains(model.prompts[0], q.ID) {
				t.Fatal("model did not receive existing inbox decisions")
			}
			_ = p
		})
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
func TestBatchIdentityMismatchDoesNotPublish(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	provider := &fakeProvider{identityErr: errors.New("account changed"), failIdentityAt: 3}
	s.Providers["github"] = provider
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id)}})
	if p.Status != "failed" {
		t.Fatal("identity change did not stop publication")
	}
	if len(model.prompts) != 1 {
		t.Fatal("expected an identity change after inference")
	}
	page, _ := s.ListRaw(ctx, "pending", 0)
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(page.Items) != 1 || len(cards.Items) != 0 {
		t.Fatal("identity failure partially committed")
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
	pending, err := s.ListRaw(context.Background(), "pending", 0)
	if err != nil || len(pending.Items) != 1 || pending.Items[0].Event.ExternalID != "queued" {
		t.Fatalf("pending after upgrade: %+v %v", pending, err)
	}
	archived, err := s.ListRaw(context.Background(), "archived", 0)
	if err != nil || len(archived.Items) != 2 {
		t.Fatalf("history after upgrade: %+v %v", archived, err)
	}
}
func TestBatchCannotSupersedeUnrelatedInboxQuestion(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if err := s.SetRawStatus(ctx, q.NotificationID, "archived"); err != nil {
		t.Fatal(err)
	}
	unrelated := event()
	unrelated.ExternalID = "unrelated-thread"
	unrelated.URL = "https://github.com/acme/repo/pull/99"
	id := seed(t, s, source, unrelated)
	p := process(t, s, model, BatchDecision{DiscardIDs: []int64{id}, SupersedeIDs: []string{q.ID}})
	if p.Status != "failed" {
		t.Fatal("allowed unrelated supersession")
	}
	old, _ := s.Inbox.Get(ctx, q.ID)
	if old.Status != "pending" {
		t.Fatal("unrelated pending question changed")
	}
}

func TestBatchRejectsChangedInboxDependenciesEvenWithoutSuperseding(t *testing.T) {
	for _, change := range []string{"dismissed", "answered", "edited"} {
		t.Run(change, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			updated := event()
			updated.Version = "v2"
			id2 := seed(t, s, source, updated)
			model.onGenerate = func(string) {
				switch change {
				case "dismissed":
					if _, err := s.Inbox.Dismiss(ctx, q.ID, q.Revision); err != nil {
						t.Fatal(err)
					}
				case "answered":
					if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Explain first", ""); err != nil {
						t.Fatal(err)
					}
				case "edited":
					c := ask().question()
					c.Context = "Changed during inference"
					if _, err := s.Inbox.PublishNotification(ctx, q.NotificationID, "GitHub", event().URL, c); err != nil {
						t.Fatal(err)
					}
				}
			}
			p := process(t, s, model, BatchDecision{DiscardIDs: []int64{q.NotificationID, id2}})
			if p.Status != "failed" || !strings.Contains(p.Error, "inbox questions changed") {
				t.Fatalf("committed stale dependency: %+v", p)
			}
			raw, _ := s.ListRaw(ctx, "pending", 0)
			if len(raw.Items) != 2 {
				t.Fatal("stale plan consumed raw notifications")
			}
			saved, _ := s.Inbox.Get(ctx, q.ID)
			switch change {
			case "dismissed":
				if saved.Status != "dismissed" {
					t.Fatal("overwrote dismissal")
				}
			case "answered":
				if saved.Status != "answered" || saved.Answer != "Explain first" {
					t.Fatal("overwrote answer")
				}
			case "edited":
				if saved.Context != "Changed during inference" {
					t.Fatal("overwrote edited question")
				}
			}
		})
	}
}
func TestBatchRejectsRelatedFollowupsOpenedBeforeOrDuringInference(t *testing.T) {
	for _, timing := range []string{"before", "during"} {
		t.Run(timing, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			id1 := seed(t, s, source, event())
			p1 := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id1)}})
			if p1.Status != "completed" {
				t.Fatal(p1.Error)
			}
			cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			original := cards.Items[0]
			if _, err := s.Inbox.Answer(ctx, nil, original.ID, original.Revision, "Explain first", ""); err != nil {
				t.Fatal(err)
			}
			if err := s.finishRun(ctx, original.ID, "succeeded", "Explanation complete.", nil); err != nil {
				t.Fatal(err)
			}
			newer := event()
			newer.Version = "v2"
			id2 := seed(t, s, source, newer)
			if _, err := s.RequestProcessing(ctx); err != nil {
				t.Fatal(err)
			}
			beforeCalls := len(model.prompts)
			var followupID string
			openFollowup := func() {
				q, err := s.Inbox.ContinueNotification(ctx, original.ID)
				if err != nil {
					t.Fatal(err)
				}
				followupID = q.ID
			}
			if timing == "before" {
				openFollowup()
			} else {
				model.onGenerate = func(string) { openFollowup() }
			}
			model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(id2)}}}
			if err := s.processNext(ctx); err != nil {
				t.Fatal(err)
			}
			status, _ := s.Status(ctx)
			if status.Processing.Status != "failed" {
				t.Fatalf("missed newly opened followup: %+v", status)
			}
			if timing == "before" && len(model.prompts) != beforeCalls {
				t.Fatal("sent stale question context to model")
			}
			pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if len(pending.Items) != 1 || pending.Items[0].ID != followupID {
				t.Fatal("created a duplicate action")
			}
			raw, _ := s.ListRaw(ctx, "pending", 0)
			if len(raw.Items) != 1 || raw.Items[0].ID != id2 {
				t.Fatal("consumed notification based on stale context")
			}
			model.onGenerate = nil
			retried := process(t, s, model, BatchDecision{DiscardIDs: []int64{id2}})
			if retried.Status != "completed" {
				t.Fatal(retried.Error)
			}
			pending, _ = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if len(pending.Items) != 1 || pending.Items[0].ID != followupID {
				t.Fatal("retry did not preserve existing action")
			}
		})
	}
}
func TestUnrelatedInboxChangesDoNotInvalidateBatch(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	model.onGenerate = func(string) {
		other := event()
		other.ExternalID = "unrelated"
		other.URL = "https://github.com/acme/repo/pull/99"
		otherID := seed(t, s, source, other)
		publish(t, s, otherID)
	}
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id)}})
	if p.Status != "completed" {
		t.Fatal(p.Error)
	}
	raw, _ := s.ListRaw(ctx, "pending", 0)
	if len(raw.Items) != 1 || raw.Items[0].Event.ExternalID != "unrelated" {
		t.Fatal("unrelated arrivals were consumed")
	}
}
func TestIdenticalBatchActionsArePublishedOnce(t *testing.T) {
	for _, kind := range []string{"identical", "reordered_events", "empty_options"} {
		t.Run(kind, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			id1 := seed(t, s, source, event())
			other := event()
			other.ExternalID = "second"
			other.URL = "https://github.com/acme/repo/pull/8"
			id2 := seed(t, s, source, other)
			first := actionFor(id1, id2)
			second := first
			if kind == "reordered_events" {
				second.EventIDs = []int64{id2, id1}
			}
			if kind == "empty_options" {
				first.Options = nil
				second.Options = []string{}
			}
			p := process(t, s, model, BatchDecision{Actions: []BatchAction{first, second}})
			if p.Status != "completed" || p.ActionCount != 1 {
				t.Fatalf("duplicate actions: %+v", p)
			}
			cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if len(cards.Items) != 1 {
				t.Fatal("published duplicate cards")
			}
			related, err := s.Inbox.RelatedNotificationIDs(ctx, cards.Items[0].ID)
			if err != nil || len(related) != 2 {
				t.Fatal("deduplication lost source notifications")
			}
		})
	}
}

type unavailableSubjectProvider struct {
	*fakeProvider
	unavailable string
	failure     error
}

func (p unavailableSubjectProvider) Enrich(ctx context.Context, source Source, event Event) (Event, error) {
	if event.ExternalID == p.unavailable {
		return Event{Context: "Unverified old content", SubjectState: "merged"}, p.failure
	}
	return p.fakeProvider.Enrich(ctx, source, event)
}
func TestUnavailableSubjectDoesNotBlockOtherNotifications(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	missing := event()
	missing.ExternalID = "missing"
	missing.URL = "https://github.com/acme/repo/pull/8"
	id1 := seed(t, s, source, missing)
	id2 := seed(t, s, source, event())
	s.Providers["github"] = unavailableSubjectProvider{fakeProvider: &fakeProvider{}, unavailable: "missing", failure: fmt.Errorf("comment disappeared: %w", ErrContextUnavailable)}
	inspect := actionFor(id1)
	inspect.Question = "Inspect this notification with unavailable context?"
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{inspect, actionFor(id2)}})
	if p.Status != "completed" || p.ActionCount != 2 {
		t.Fatalf("one missing subject blocked the batch: %+v", p)
	}
	if !strings.Contains(model.prompts[0], "context_warnings") || strings.Contains(model.prompts[0], "Unverified old content") || strings.Contains(model.prompts[0], `"subject_state":"merged"`) {
		t.Fatal("missing evidence was not clearly distinguished from verified state")
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	warned := false
	for _, q := range cards.Items {
		if q.NotificationID == id1 {
			warned = strings.Contains(q.Context, "Context availability:") && strings.Contains(q.Context, "Verify the linked notification before acting")
		}
	}
	if !warned {
		t.Fatal("card hid missing evidence from user")
	}
}
func TestHardEnrichmentFailuresRemainFatal(t *testing.T) {
	for _, cause := range []error{&GitHubHTTPError{StatusCode: 401}, &GitHubHTTPError{StatusCode: 403}, &GitHubHTTPError{StatusCode: 429}, errors.New("transport unavailable"), context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			id := seed(t, s, source, event())
			s.Providers["github"] = &fakeProvider{enrichErr: cause}
			p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id)}})
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

func TestBatchRejectsFollowupOpenedAndAnsweredDuringInference(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id1 := seed(t, s, source, event())
	initial := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id1)}})
	if initial.Status != "completed" {
		t.Fatal(initial.Error)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	original := cards.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, original.ID, original.Revision, "Explain first", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRun(ctx, original.ID, "succeeded", "Explained.", nil); err != nil {
		t.Fatal(err)
	}
	update := event()
	update.Version = "v2"
	id2 := seed(t, s, source, update)
	var followupID string
	model.onGenerate = func(string) {
		q, err := s.Inbox.ContinueNotification(ctx, original.ID)
		if err != nil {
			t.Fatal(err)
		}
		followupID = q.ID
		if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Start the review", ""); err != nil {
			t.Fatal(err)
		}
	}
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(id2)}})
	if p.Status != "failed" {
		t.Fatal("missed a follow-up that was already answered")
	}
	saved, err := s.Inbox.Get(ctx, followupID)
	if err != nil || saved.Status != "answered" || saved.Answer != "Start the review" {
		t.Fatalf("lost user's authorization: %+v %v", saved, err)
	}
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	raw, _ := s.ListRaw(ctx, "pending", 0)
	if len(pending.Items) != 0 || len(raw.Items) != 1 {
		t.Fatal("published duplicate work or consumed the update")
	}
}
func TestResolvedHistoryDoesNotHidePendingQuestionOrInvalidateStableBatch(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if _, err := s.Inbox.Dismiss(ctx, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	updated := event()
	updated.Version = "v2"
	id2 := seed(t, s, source, updated)
	p := process(t, s, model, BatchDecision{Actions: []BatchAction{actionFor(q.NotificationID, id2)}})
	if p.Status != "completed" {
		t.Fatal("unchanged resolved history invalidated batch", p.Error)
	}
	if strings.Contains(model.prompts[len(model.prompts)-1], q.ID) {
		t.Fatal("resolved history was presented as pending action")
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
	model.responses = []any{BatchDecision{DiscardIDs: []int64{id}, SupersedeIDs: []string{q.ID}}}
	if err = s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" {
		t.Fatal(status.Processing.Error)
	}
}

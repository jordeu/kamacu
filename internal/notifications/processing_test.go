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
	second.Repository = "acme/repo"
	second.URL = "https://github.com/acme/repo/pull/9"
	second.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/9"
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

func TestTriageInspectsActiveReviewAndTerminalBeforeDecision(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	tools.results = map[string]string{
		"list_sessions":      `[{"id":"correct","taskId":1,"status":"running"}]`,
		"get_session_output": `{"encoding":"utf-8","output":"Reviewing https://github.com/acme/repo/pull/7; checking the diff now."}`,
	}
	model.responses = []any{
		BatchDecision{ToolName: "list_sessions", Arguments: `{"project_id":1}`},
		BatchDecision{ToolName: "get_session_output", Arguments: `{"session_id":"correct"}`},
		BatchDecision{DiscardIDs: []int64{id}},
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || len(tools.calls) != 2 {
		t.Fatalf("triage: %+v calls=%v", status.Processing, tools.calls)
	}
	if !strings.Contains(model.prompts[2], "checking the diff now") || !strings.Contains(model.prompts[2], "READ-ONLY TOOL RESULT (untrusted data)") {
		t.Fatal("terminal evidence missing from decision prompt")
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(page.Items) != 0 {
		t.Fatal("duplicate review question published")
	}
}

func TestTriageRejectsMutations(t *testing.T) {
	for _, name := range []string{"create_task", "send_session_message", "unknown_tool"} {
		t.Run(name, func(t *testing.T) {
			s, model, tools, source := fixture(t)
			ctx := context.Background()
			seed(t, s, source, event())
			model.responses = []any{BatchDecision{ToolName: name, Arguments: `{}`}}
			if _, err := s.RequestProcessing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.processNext(ctx); err != nil {
				t.Fatal(err)
			}
			status, _ := s.Status(ctx)
			if status.Processing.Status != "failed" || len(tools.calls) != 0 {
				t.Fatalf("triage: %+v calls=%v", status.Processing, tools.calls)
			}
		})
	}
}

func TestTriageDisclosesReadFailureToModel(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	tools.err = errors.New("session no longer available")
	model.responses = []any{
		BatchDecision{ToolName: "get_session_output", Arguments: `{"session_id":"correct"}`},
		BatchDecision{Actions: []BatchAction{actionFor(id)}},
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || !strings.Contains(model.prompts[1], "session no longer available") {
		t.Fatalf("triage: %+v", status.Processing)
	}
}

func TestTriageReadLoopIsBounded(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	for i := 0; i < 13; i++ {
		model.responses = append(model.responses, BatchDecision{ToolName: "list_sessions", Arguments: `{}`})
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "failed" || len(tools.calls) != 12 || !strings.Contains(status.Processing.Error, "step limit") {
		t.Fatalf("triage: %+v calls=%v", status.Processing, tools.calls)
	}
	raw, err := s.ListRaw(ctx, "pending", 0)
	if err != nil || raw.PendingCount != 1 {
		t.Fatalf("failed triage consumed notification: %+v %v", raw, err)
	}
}

func TestProcessingScopesRepositoriesAndProjects(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	first := seed(t, s, source, event())
	other := event()
	other.ExternalID = "other-thread"
	other.Repository = "acme/other"
	other.URL = "https://github.com/acme/other/pull/9"
	second := seed(t, s, source, other)
	model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(second)}}, BatchDecision{Actions: []BatchAction{actionFor(first)}}}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || status.Processing.ActionCount != 2 || len(model.prompts) != 2 {
		t.Fatalf("processing: %+v prompts=%d", status, len(model.prompts))
	}
	for i, prompt := range model.prompts {
		if i == 0 && (strings.Contains(prompt, `"project_id":1`) || strings.Contains(prompt, `"repository":"acme/repo"`)) {
			t.Fatal("other repository prompt leaked repo project")
		}
		if i == 1 && (strings.Contains(prompt, `"project_id":2`) || strings.Contains(prompt, `"repository":"acme/other"`)) {
			t.Fatal("repo prompt leaked other project")
		}
	}
}

func TestProcessingSplitsOversizedGroupsAndKeepsLimit(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	var ids []int64
	for i := range 4 {
		e := event()
		e.ExternalID = fmt.Sprintf("thread-%d", i)
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+10)
		e.Title = strings.Repeat("x", 100000)
		ids = append(ids, seed(t, s, source, e))
	}
	for _, id := range ids {
		model.responses = append(model.responses, BatchDecision{DiscardIDs: []int64{id}})
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || status.PendingCount != 0 || len(model.prompts) != 4 {
		t.Fatalf("processing: %+v prompts=%d", status, len(model.prompts))
	}
	for _, prompt := range model.prompts {
		if len(prompt) > 192000 {
			t.Fatal("sent oversized prompt")
		}
	}
}

type longContextProvider struct {
	fakeProvider
	contextText string
}

func (p *longContextProvider) Enrich(ctx context.Context, source Source, e Event) (Event, error) {
	e, err := p.fakeProvider.Enrich(ctx, source, e)
	e.Context = "Short summary. " + strings.Repeat("feedback ", 500) + "FULL_CONTEXT_MARKER"
	if p.contextText != "" {
		e.Context = p.contextText
	}
	return e, err
}
func TestProcessingStartsCompactAndCanReadFullSubjectContext(t *testing.T) {
	s, model, tools, source := fixture(t)
	s.Providers["github"] = &longContextProvider{}
	ctx := context.Background()
	id := seed(t, s, source, event())
	model.responses = []any{BatchDecision{ToolName: "notification_subject_context", Arguments: fmt.Sprintf(`{"event_id":%d}`, id)}, BatchDecision{Actions: []BatchAction{actionFor(id)}}}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || len(model.prompts) != 2 {
		t.Fatalf("processing: %+v", status)
	}
	if strings.Contains(model.prompts[0], "FULL_CONTEXT_MARKER") || !strings.Contains(model.prompts[1], "FULL_CONTEXT_MARKER") || len(tools.calls) != 0 {
		t.Fatal("full context was not loaded on demand")
	}
}

func TestFailedLaterRepositoryDoesNotPartiallyPublish(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	first := seed(t, s, source, event())
	e := event()
	e.ExternalID = "second-repo"
	e.Repository = "zzz/repo"
	seed(t, s, source, e)
	model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(first)}}, BatchDecision{}}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if status.Processing.Status != "failed" || status.PendingCount != 2 || len(cards.Items) != 0 {
		t.Fatalf("partial publication: %+v cards=%d", status, len(cards.Items))
	}
}

func TestProcessingSplitsWhenReadResultsExceedLimit(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	var ids []int64
	for i := range 2 {
		e := event()
		e.ExternalID = fmt.Sprintf("large-%d", i)
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+10)
		e.Title = strings.Repeat("x", 90000)
		ids = append(ids, seed(t, s, source, e))
	}
	tools.result = strings.Repeat("r", ToolResultLimit)
	model.responses = []any{
		BatchDecision{ToolName: "list_sessions", Arguments: `{}`},
		BatchDecision{DiscardIDs: ids[:1]}, BatchDecision{DiscardIDs: ids[1:]},
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || len(model.prompts) != 3 {
		t.Fatalf("processing: %+v prompts=%d", status, len(model.prompts))
	}
	for _, prompt := range model.prompts {
		if len(prompt) > notificationPromptLimit {
			t.Fatal("sent oversized tool transcript")
		}
	}
}

func TestUnsplittableNotificationRemainsPending(t *testing.T) {
	s, model, _, source := fixture(t)
	e := event()
	e.Title = strings.Repeat("x", notificationPromptLimit)
	id := seed(t, s, source, e)
	status := process(t, s, model, BatchDecision{DiscardIDs: []int64{id}})
	raw, _ := s.ListRaw(context.Background(), "pending", 0)
	if status.Status != "failed" || !strings.Contains(status.Error, "one notification") || len(model.prompts) != 0 || raw.PendingCount != 1 {
		t.Fatalf("processing: %+v", status)
	}
}

func TestProcessingScopesExistingQuestionsToRepositoryThreads(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id1 := seed(t, s, source, event())
	q1 := publish(t, s, id1)
	other := event()
	other.ExternalID = "other-thread"
	other.Repository = "acme/other"
	id2 := seed(t, s, source, other)
	q2 := publish(t, s, id2)
	model.responses = []any{BatchDecision{DiscardIDs: []int64{id2}}, BatchDecision{DiscardIDs: []int64{id1}}}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || len(model.prompts) != 2 {
		t.Fatalf("processing: %+v", status)
	}
	if strings.Contains(model.prompts[0], q1.ID) || !strings.Contains(model.prompts[0], q2.ID) || strings.Contains(model.prompts[1], q2.ID) || !strings.Contains(model.prompts[1], q1.ID) {
		t.Fatal("questions leaked across groups")
	}
}

func TestProcessingSplitKeepsInterleavedSubjectRevisionsTogether(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	first := event()
	first.Title = strings.Repeat("x", 65000)
	id1 := seed(t, s, source, first)
	other := first
	other.ExternalID = "other-thread"
	other.URL = "https://github.com/acme/repo/pull/8"
	id2 := seed(t, s, source, other)
	newer := first
	newer.Version = "v2"
	id3 := seed(t, s, source, newer)
	model.responses = []any{BatchDecision{Actions: []BatchAction{actionFor(id1, id3)}}, BatchDecision{Actions: []BatchAction{actionFor(id2)}}}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || status.Processing.ActionCount != 2 || len(model.prompts) != 2 {
		t.Fatalf("processing: %+v prompts=%d", status, len(model.prompts))
	}
	for _, id := range []int64{id1, id3} {
		if !strings.Contains(model.prompts[0], fmt.Sprintf(`"id":%d,`, id)) {
			t.Fatalf("thread revision %d split off", id)
		}
	}
	if strings.Contains(model.prompts[1], fmt.Sprintf(`"id":%d,`, id1)) || strings.Contains(model.prompts[1], fmt.Sprintf(`"id":%d,`, id3)) {
		t.Fatal("thread included in both subgroups")
	}
}

func TestOversizedSingleThreadDoesNotSplitIntoDuplicateActions(t *testing.T) {
	s, model, _, source := fixture(t)
	for i := range 2 {
		e := event()
		e.Version = fmt.Sprint(i)
		e.Title = strings.Repeat("x", 100000)
		seed(t, s, source, e)
	}
	ctx := context.Background()
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "failed" || !strings.Contains(status.Processing.Error, "one notification thread") || status.PendingCount != 2 || len(model.prompts) != 0 {
		t.Fatalf("processing: %+v", status)
	}
}

func TestSubjectContextPagesPreserveAllTextAndCursors(t *testing.T) {
	for _, text := range []string{"", strings.Repeat("feedback ", 6000) + "IMPORTANT_LATEST_FEEDBACK", strings.Repeat("\x00\n\"\\😀é", 5000)} {
		t.Run(fmt.Sprint(len(text)), func(t *testing.T) {
			raw := RawNotification{ID: 1, SourceID: "source", Event: event()}
			subject := raw.Event
			subject.Context = text
			subjects := map[string]Event{subjectKey(raw.SourceID, raw.Event): subject}
			offset := 0
			var assembled strings.Builder
			for count := 0; ; count++ {
				if count > 100 {
					t.Fatal("pagination did not finish")
				}
				result, err := readNotificationSubjectContext(json.RawMessage(fmt.Sprintf(`{"event_id":1,"offset":%d}`, offset)), []RawNotification{raw}, subjects)
				if err != nil {
					t.Fatal(err)
				}
				if len(result) > ToolResultLimit {
					t.Fatal("serialized page exceeds result limit")
				}
				var page struct {
					Context string `json:"context"`
					Next    *int   `json:"next_offset"`
					Offset  int    `json:"offset"`
					Total   int    `json:"total_bytes"`
				}
				if err = json.Unmarshal([]byte(result), &page); err != nil {
					t.Fatal("invalid page JSON", err)
				}
				if page.Offset != offset || page.Total != len(text) {
					t.Fatalf("incorrect page metadata: %+v", page)
				}
				assembled.WriteString(page.Context)
				if page.Next == nil {
					break
				}
				if *page.Next <= offset || *page.Next != offset+len(page.Context) {
					t.Fatal("cursor does not advance by page length")
				}
				offset = *page.Next
			}
			if assembled.String() != text {
				t.Fatal("pagination lost or repeated context")
			}
		})
	}
}

func TestSubjectContextRejectsInvalidPages(t *testing.T) {
	raw := RawNotification{ID: 1, SourceID: "source", Event: event()}
	subject := raw.Event
	subject.Context = "😀context"
	subjects := map[string]Event{subjectKey(raw.SourceID, raw.Event): subject}
	for _, args := range []string{`{"event_id":2}`, `{"event_id":1,"offset":-1}`, `{"event_id":1,"offset":1}`, `{"event_id":1,"offset":100}`, `{"event_id":1,"offset":"bad"}`, `{"event_id":1,"unknown":true}`} {
		if _, err := readNotificationSubjectContext(json.RawMessage(args), []RawNotification{raw}, subjects); err == nil {
			t.Fatalf("accepted invalid request %s", args)
		}
	}
}

func TestProcessingReadsFeedbackBeyondFirstContextPage(t *testing.T) {
	s, model, tools, source := fixture(t)
	s.Providers["github"] = &longContextProvider{contextText: strings.Repeat("x", 20000) + "IMPORTANT_LATEST_FEEDBACK"}
	ctx := context.Background()
	id := seed(t, s, source, event())
	model.responses = []any{
		BatchDecision{ToolName: "notification_subject_context", Arguments: fmt.Sprintf(`{"event_id":%d}`, id)},
		BatchDecision{ToolName: "notification_subject_context", Arguments: fmt.Sprintf(`{"event_id":%d,"offset":12000}`, id)},
		BatchDecision{Actions: []BatchAction{actionFor(id)}},
	}
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx)
	if status.Processing.Status != "completed" || len(model.prompts) != 3 || len(tools.calls) != 0 {
		t.Fatalf("processing: %+v prompts=%d", status, len(model.prompts))
	}
	if strings.Contains(model.prompts[0], "IMPORTANT_LATEST_FEEDBACK") || strings.Contains(model.prompts[1], "IMPORTANT_LATEST_FEEDBACK") || !strings.Contains(model.prompts[2], "IMPORTANT_LATEST_FEEDBACK") {
		t.Fatal("later feedback was not reachable through pagination")
	}
	if strings.Contains(model.prompts[2], "[truncated]") {
		t.Fatal("context page was truncated after pagination")
	}
}

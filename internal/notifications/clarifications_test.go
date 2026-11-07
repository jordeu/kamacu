package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestUncertaintyCategoriesAndKnownExclusions(t *testing.T) {
	for _, category := range []string{"presentation", "grouping", "action", "filtering"} {
		t.Run(category, func(t *testing.T) {
			effect := map[string]string{"presentation": "format", "grouping": "group_project", "action": "route_review", "filtering": "suppress"}[category]
			set := RuleSet{Rules: []NotificationRule{{ID: "R1", Enabled: true, Clauses: []RuleClause{{ID: "R1.1", Category: category, Effect: effect, OnUnknown: "hold", When: RuleExpression{Op: "eq", Fact: "actor", Value: "alice"}}}}}}
			outcome, evals, _ := evaluateRuleSet(set, nil, nil, false)
			want := "admitted"
			if category == "action" || category == "filtering" {
				want = "clarification"
			}
			if outcome != want || !strings.Contains(evals[0].Reason, "actor") {
				t.Fatal(outcome, evals)
			}
			set.Rules = append(set.Rules, NotificationRule{ID: "R2", Enabled: true, Clauses: []RuleClause{{Category: "filtering", Effect: "suppress", When: RuleExpression{Op: "always"}}}})
			outcome, _, _ = evaluateRuleSet(set, nil, nil, false)
			if outcome != "suppressed" {
				t.Fatal("uncertainty bypassed exclusion", outcome)
			}
		})
	}
}

func uncertainOwnRule() NotificationRule {
	return NotificationRule{ID: "SELF", Text: "Ignore my own activity", Enabled: true, Clauses: []RuleClause{{Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "eq", Fact: "activity.own", Value: true}}}}
}

func TestClarificationsGroupMergeAndContinueWithoutStartingWork(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	for i := 7; i < 9; i++ {
		e := event()
		e.ExternalID = fmt.Sprint(i)
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i)
		seed(t, s, source, e)
	}
	first := process(t, s, model, "direct_question")
	if first.Status != "completed" || first.ActionCount != 1 {
		t.Fatal(first)
	}
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || len(cards.Items) != 1 {
		t.Fatal(cards, err)
	}
	q := cards.Items[0]
	if !strings.Contains(q.Context, "activity.own") || !strings.Contains(q.Context, "/pull/8") {
		t.Fatal(q.Context)
	}
	for _, p := range model.prompts {
		if strings.Contains(p, projectCardsMarker) {
			t.Fatal("native clarification used a writer")
		}
	}
	e := event()
	e.ExternalID = "9"
	e.URL = "https://github.com/acme/repo/pull/9"
	seed(t, s, source, e)
	next := process(t, s, model, "")
	if next.Status != "completed" || next.ActionCount != 1 {
		t.Fatal(next)
	}
	cards, err = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || len(cards.Items) != 1 {
		t.Fatal(cards, err)
	}
	if cards.Items[0].ID != q.ID || cards.Items[0].Revision <= q.Revision || !strings.Contains(cards.Items[0].Context, "/pull/9") {
		t.Fatal("group did not update", cards.Items)
	}
	q = cards.Items[0]
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, q.ID)
	if err != nil || len(ids) != 3 {
		t.Fatal(ids, err)
	}
	if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "rule_continue_once"); err != nil {
		t.Fatal(err)
	}
	priorCalls := len(model.prompts)
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 0 || len(model.prompts) != priorCalls {
		t.Fatal("one-time processing started work")
	}
	status, _ := s.Status(ctx)
	if status.PendingCount != 3 {
		t.Fatal(status)
	}
	result := process(t, s, model, "")
	if result.Status != "completed" || result.ActionCount != 3 {
		t.Fatal(result)
	}
	var snapshots int
	s.DB.QueryRow(`SELECT count(*) FROM notification_decision_snapshots WHERE json_valid(evidence)`).Scan(&snapshots)
	if snapshots < 6 {
		t.Fatal("missing frozen evidence", snapshots)
	}
	// A later update on the same subject must evaluate SELF again.
	e.Version = "2026-11-07T14:00:00Z"
	e.Evidence = []SubjectEvidence{{ID: "new-comment", Text: "A different question?", UpdatedAt: e.Version, Complete: true}}
	seed(t, s, source, e)
	result = process(t, s, model, "")
	if result.Status != "completed" || result.ActionCount != 1 {
		t.Fatal(result)
	}
	cards, _ = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	found := false
	for _, q := range cards.Items {
		found = found || strings.Contains(q.Context, "activity.own")
	}
	if !found {
		t.Fatal("one-time override leaked into a later version")
	}
}

func TestClarificationIgnoreAndDismissedReceipt(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	id := seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := cards.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	before := len(model.prompts)
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 0 || len(model.prompts) != before {
		t.Fatal("ignore triggered work")
	}
	if err := s.SetRawStatus(ctx, id, "pending"); err != nil {
		t.Fatal(err)
	}
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 0 {
		t.Fatal("handled clarification repeated", p)
	}
}

func TestNoveltyUsesDeliveredReceiptsAndContentChanges(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	e := event()
	e.Version = "2026-11-06T12:00:00Z"
	e.Evidence = []SubjectEvidence{{ID: "comment", Text: "Please check", Complete: true, UpdatedAt: e.Version}}
	seed(t, s, source, e)
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := cards.Items[0]
	if _, err := s.Inbox.Dismiss(ctx, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	subject := &triageSubject{fresh: e, events: []RawNotification{{SourceID: source.ID, Event: e}}}
	f := intentFinding{Intent: "direct_question", EvidenceID: "comment"}
	facts := map[string]RuleFact{}
	if err := s.addNoveltyFact(ctx, activityID(subject, f), facts); err != nil {
		t.Fatal(err)
	}
	if facts["activity.new"].Value != false {
		t.Fatal("dismissal lost delivery receipt", facts)
	}
	original := activityID(subject, f)
	subject.fresh.Evidence[0].Text = "Please check a different problem"
	if original == activityID(subject, f) {
		t.Fatal("content edits reused handled activity identity")
	}
}

func TestImprovementPreservesDraftAndRequiresExplicitActivation(t *testing.T) {
	for _, cause := range []string{"rule", "data"} {
		t.Run(cause, func(t *testing.T) {
			s, model, tools, _ := fixture(t)
			ctx := context.Background()
			activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
			active, _ := s.activeRuleSet(ctx)
			draft, err := s.SaveRuleDraft(ctx, active.Revision, append(active.Rules, NotificationRule{Text: "Also ignore irrelevant releases", Enabled: true}))
			if err != nil {
				t.Fatal(err)
			}
			result := RuleImprovement{Diagnosis: "A specific clarification is needed", Cause: cause, Risks: "Preserve exclusions", Changes: []RuleTextChange{}}
			if cause == "rule" {
				result.Changes = []RuleTextChange{{RuleID: "SELF", Text: "Ignore activity authored by alice; ask when the actor is unavailable", Explanation: "Clarifies authorship"}}
			}
			model.responses = []any{result}
			improvement, err := s.ImproveRules(ctx, RuleImprovementRequest{BaseRevision: draft.Revision, Kind: "unclear_rule", Instructions: "Ignore only comments authored by me; ask when the author is unknown"})
			if err != nil {
				t.Fatal(err)
			}
			if cause == "rule" && (improvement.Draft == nil || len(improvement.Draft.Rules) != 2 || improvement.Draft.Status != "draft") {
				t.Fatal(improvement)
			}
			if cause == "data" && improvement.Draft != nil {
				t.Fatal("data defect rewrote preferences")
			}
			after, _ := s.activeRuleSet(ctx)
			if after.ID != active.ID || len(tools.calls) != 0 {
				t.Fatal("diagnosis activated rules or executed work")
			}
			if _, err = s.ImproveRules(ctx, RuleImprovementRequest{BaseRevision: active.Revision, Kind: "unclear_rule", Instructions: "Try again"}); err != ErrRulesChanged {
				t.Fatal("stale draft was accepted", err)
			}
		})
	}
}

func TestFrozenHistoryReplayNeverFetchesSourcesOrMutates(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	active, _ := s.activeRuleSet(ctx)
	provider := s.Providers["github"].(*fakeProvider)
	calls := provider.enrichCalls
	modelCalls := len(model.prompts)
	results, err := s.ReplayRuleHistory(ctx, active.ID)
	if err != nil || len(results) != 1 || results[0].After != "clarification" || !results[0].Limited {
		t.Fatal(results, err)
	}
	if provider.enrichCalls != calls || len(model.prompts) != modelCalls || len(tools.calls) != 0 {
		t.Fatal("historical replay used live tools")
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&count)
	if count != 1 {
		t.Fatal("replay published cards", count)
	}
	raw, _ := json.Marshal(results)
	if !strings.Contains(string(raw), "activity.own") {
		t.Fatal("replay lost missing fact", string(raw))
	}
}

// Older migration tests intentionally rerun a historical migration. Roll back
// its newer dependent migration first, just as the real migration runner does.
func downgradeClarifications(t *testing.T, s *Service) string {
	t.Helper()
	raw, err := os.ReadFile("../store/migrations/00040_notification_clarifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	return parts[0]
}
func TestClarificationMigrationRequeuesHiddenInputsAndPreservesHistory(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	id := seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	up := downgradeClarifications(t, s)
	if _, err := s.DB.Exec(`UPDATE notification_events SET held=1,raw_status='pending' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(up); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.PendingCount != 1 {
		t.Fatal("hidden input was not returned to pending", status, err)
	}
	var outcome string
	if err = s.DB.QueryRow(`SELECT outcome FROM notification_rule_decisions LIMIT 1`).Scan(&outcome); err != nil || outcome != "clarification" {
		t.Fatal(outcome, err)
	}
	if _, err = s.DB.Exec(`SELECT held FROM notification_events`); err == nil {
		t.Fatal("hidden queue column survived migration")
	}
}

func TestRoutineBotNoopPreservesSubstantiveAndFormalReviews(t *testing.T) {
	for _, tc := range []struct {
		text, state string
		want        bool
	}{
		{"No new findings.", "COMMENTED", true},
		{"No new findings. Please check the API regression.", "COMMENTED", false},
		{"No new findings.", "APPROVED", false},
		{"No new findings.", "CHANGES_REQUESTED", false},
	} {
		e := SubjectEvidence{ID: "review", Actor: "reviewer[bot]", Text: tc.text, ReviewState: tc.state, Complete: true}
		if got := routineBotFinding(intentFinding{Intent: "review_feedback", EvidenceID: "review"}, []SubjectEvidence{e}); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
func TestClarificationContinueStillEnforcesOtherExclusions(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	other := NotificationRule{ID: "BLOCK", Text: "Ignore known self authors", Enabled: true, Clauses: []RuleClause{{Category: "filtering", Effect: "suppress", OnUnknown: "continue", When: RuleExpression{Op: "eq", Fact: "actor", Value: "bob"}}}}
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule(), other})
	e := event()
	id := seed(t, s, source, e)
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := cards.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "rule_continue_once"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	// Enrichment establishes the other exclusion before the one-time retry.
	e.Evidence = []SubjectEvidence{{ID: "comment", Actor: "bob", Text: "Please check this", Complete: true, UpdatedAt: "2026-11-07T14:00:00Z"}}
	e.Version = "2026-11-07T14:00:00Z"
	s.Providers["github"] = &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 0 {
		t.Fatal("override bypassed another exclusion", p)
	}
	decisions, err := s.EventDecisions(ctx, id)
	if err != nil || len(decisions) == 0 || decisions[0].Outcome != "suppressed" {
		t.Fatal(decisions, err)
	}
}

func TestClarificationKeepsNavigationToExistingOwner(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	if _, err := s.DB.Exec(`INSERT INTO task_pr_links(task_id,repository,number,role,source,evidence,uncertainty,observed_at,corrected) VALUES(1,'acme/repo',7,'implementation','explicit','Existing owner','','2026-11-07T12:00:00Z',0)`); err != nil {
		t.Fatal(err)
	}
	seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 1 || len(cards.Items[0].LinkedTasks) != 1 || cards.Items[0].LinkedTasks[0].TaskID != 1 {
		t.Fatal("clarification lost task navigation", cards)
	}
}

func TestImprovementDraftAndFeedbackAreAtomic(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	active, _ := s.activeRuleSet(ctx)
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_feedback BEFORE INSERT ON notification_rule_feedback BEGIN SELECT RAISE(ABORT,'simulated write failure'); END`); err != nil {
		t.Fatal(err)
	}
	model.responses = []any{RuleImprovement{Diagnosis: "Clarify authorship", Cause: "rule", Risks: "Keep exclusions", Changes: []RuleTextChange{{RuleID: "SELF", Text: "Ignore comments authored by alice", Explanation: "Explicit user preference"}}}}
	_, err := s.ImproveRules(ctx, RuleImprovementRequest{BaseRevision: active.Revision, Kind: "unclear_rule", Instructions: "Ignore only comments I authored"})
	if err == nil {
		t.Fatal("feedback failure was hidden")
	}
	var revision int64
	if err = s.DB.QueryRow(`SELECT max(revision) FROM notification_rule_sets`).Scan(&revision); err != nil || revision != active.Revision {
		t.Fatal("failed diagnosis left a partial draft", revision, err)
	}
}

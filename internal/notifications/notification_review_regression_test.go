package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"kamacu/internal/inbox"
)

func TestOneTimeOverrideStillEvaluatesOtherLLMExclusions(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	e := event()
	e.Evidence = []SubjectEvidence{{ID: "comment", Actor: "alice", Text: "Ignore this question", Complete: true, Current: true}}
	f := intentFinding{Intent: "direct_question", EvidenceID: "comment", Quote: e.Evidence[0].Text}
	subject := &triageSubject{viewer: "alice", fresh: e, findings: []intentFinding{f}}
	id := activityID(subject, f)
	subject.overrides = map[string][]string{id: {"SELF"}}
	rule := uncertainOwnRule()
	rule.Clauses[0].ID = "SELF.1"
	set := RuleSet{Rules: []NotificationRule{rule, {ID: "OTHER", Revision: 1, Enabled: true, Text: "Ignore this specific question", Mode: "llm"}}}
	model.respond = func(prompt string) (any, error) {
		_, raw, ok := strings.Cut(prompt, "CHECKS:\n")
		if !ok {
			t.Fatal("unexpected model call", prompt)
		}
		var checks []struct {
			Facts map[string]RuleFact `json:"facts"`
		}
		if err := json.Unmarshal([]byte(raw), &checks); err != nil {
			t.Fatal(err)
		}
		if len(checks) != 1 || !checks[0].Facts["activity.new"].Known {
			t.Fatal("fallback omitted delivery facts", checks)
		}
		clause := RuleClause{ID: "OTHER.1", Category: "filtering", Effect: "suppress", When: RuleExpression{Op: "semantic", Text: "OTHER.1"}, OnUnknown: "hold"}
		return map[string]string{"compiled_json": mustJSON(map[string]any{"results": []any{map[string]any{"id": id + ":OTHER", "effects": []any{map[string]any{"clause": clause, "result": "true", "evidence_id": "comment", "quote": f.Quote}}}}})}, nil
	}
	if err := s.evaluateActivities(ctx, []*triageSubject{subject}, set); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 1 || len(subject.decisions) != 1 || subject.decisions[0].Outcome != "suppressed" {
		t.Fatal("one-time exception bypassed an independent rule", subject.decisions)
	}
}

func TestPreviewIncludesNativeClarificationsWithoutPublishing(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	id := seed(t, s, source, event())
	set, _ := s.activeRuleSet(ctx)
	respondWithIntent(model, "direct_question")
	result, err := s.previewRules(ctx, set, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cards) != 1 || result.Cards[0].Plan.ClarificationKey == "" || !strings.Contains(result.Cards[0].Context, "activity.own") {
		t.Fatal("preview omitted the actual Inbox decision", result)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&count)
	status, _ := s.Status(ctx)
	if count != 0 || status.PendingCount != 1 || len(tools.calls) != 0 {
		t.Fatal("preview mutated notifications or work", count, status)
	}
}

func TestConversationFollowupIsNotAClarificationMergeDestination(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := cards.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	followup, err := s.Inbox.ContinueNotification(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := event()
	e.ExternalID = "different"
	e.URL = "https://github.com/acme/repo/pull/8"
	seed(t, s, source, e)
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal(p)
	}
	cards, _ = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 2 {
		t.Fatal("new updates merged into a free-text conversation", cards)
	}
	next, err := s.Inbox.Get(ctx, followup.ID)
	if err != nil || next.Revision != followup.Revision || next.Context != followup.Context {
		t.Fatal("conversation was rewritten", next, err)
	}
}

func TestClarificationGrantRejectsUnrelatedNotificationAtomically(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	first := seed(t, s, source, event())
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := cards.Items[0]
	other := event()
	other.ExternalID = "unrelated"
	other.URL = "https://github.com/acme/repo/pull/8"
	unrelated := seed(t, s, source, other)
	var raw string
	if err := s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, q.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plan ActionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatal(err)
	}
	plan.Uncertain[0].EventIDs = append(plan.Uncertain[0].EventIDs, unrelated)
	if _, err := s.DB.Exec(`UPDATE notification_card_plans SET plan=? WHERE question_id=?`, mustJSON(plan), q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.continueRuleClarification(ctx, q); !errors.Is(err, inbox.ErrInvalid) {
		t.Fatal("unrelated grant accepted", err)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_overrides`).Scan(&count)
	var state string
	s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, first).Scan(&state)
	if count != 0 || state != "archived" {
		t.Fatal("invalid grant partially applied", count, state)
	}
}

func TestReplyPlannerCannotInventTypedClarificationControls(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	original := ActionPlan{Workflow: "clarify", ProjectID: 1, RuleSetID: "bound"}
	injected := original
	injected.ClarificationKey = "untrusted"
	injected.Uncertain = []UncertainActivity{{ID: "foreign", RuleIDs: []string{"SELF"}, EventIDs: []int64{999}}}
	model.planResponse = map[string]any{"plan_json": mustJSON(injected), "explicit_change": false, "user_quote": "", "request_kind": "clarify"}
	got, err := s.replyActionPlan(ctx, original, inbox.Question{Answer: "Explain the problem"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClarificationKey != "" || len(got.Uncertain) > 0 {
		t.Fatal("planner manufactured application controls", got)
	}
}

func TestHistoricalReplayMatchesRuntimeUnknownPolicy(t *testing.T) {
	for _, tc := range []struct{ name, category, effect, unknown, want string }{
		{"presentation", "presentation", "format", "hold", "admitted"},
		{"grouping", "grouping", "group_project", "hold", "admitted"},
		{"best effort exclusion", "filtering", "suppress", "continue", "admitted"},
		{"blocking exclusion", "filtering", "suppress", "hold", "clarification"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := RuleSet{Rules: []NotificationRule{{ID: "R1", Enabled: true, Clauses: []RuleClause{{Category: tc.category, Effect: tc.effect, OnUnknown: tc.unknown, When: RuleExpression{Op: "eq", Fact: "actor", Value: "alice"}}}}}}
			facts := map[string]RuleFact{"activity.kind": {Known: true, Value: "direct_question"}}
			got, _, limited := replayRuleDecision(set, facts, nil)
			if got != tc.want || !limited {
				t.Fatal(got, limited)
			}
		})
	}
	opaque := RuleSet{Rules: []NotificationRule{{ID: "LLM", Enabled: true, Mode: "llm"}}}
	facts := map[string]RuleFact{"activity.kind": {Known: true, Value: "needs_context"}, "activity.work_kind": {Known: true, Value: "review"}, "pr.state": {Known: true, Value: "merged"}}
	got, _, limited := replayRuleDecision(opaque, facts, nil)
	if got != "suppressed" || !limited {
		t.Fatal("obsolete review replayed as actionable", got, limited)
	}
	delete(facts, "pr.state")
	got, _, _ = replayRuleDecision(opaque, facts, nil)
	if got != "unknown" {
		t.Fatal("opaque LLM outcome was guessed", got)
	}
	got, _, limited = replayRuleDecision(RuleSet{}, map[string]RuleFact{}, nil)
	if got != "unknown" || !limited {
		t.Fatal("unclassified history was treated as admitted", got)
	}
}

func TestDiagnosisWithoutEditsStillRejectsConcurrentDraftChanges(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	active, _ := s.activeRuleSet(ctx)
	model.onGenerate = func(string) {
		model.onGenerate = nil
		if _, err := s.SaveRuleDraft(ctx, active.Revision, active.Rules); err != nil {
			t.Fatal(err)
		}
	}
	model.responses = []any{RuleImprovement{Diagnosis: "Missing actor evidence", Cause: "data", Changes: []RuleTextChange{}}}
	_, err := s.ImproveRules(ctx, RuleImprovementRequest{BaseRevision: active.Revision, Kind: "unclear_rule", Instructions: "Explain why this actor is missing"})
	if !errors.Is(err, ErrRulesChanged) {
		t.Fatal("stale diagnosis was accepted", err)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_feedback`).Scan(&count)
	if count != 0 {
		t.Fatal("stale feedback was saved", count)
	}
}

func TestOneTimeGrantsExpireAfterTerminalProcessing(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule()})
	seed(t, s, source, event())
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
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_overrides`).Scan(&count)
	if count != 1 {
		t.Fatal("grant was not recorded", count)
	}
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal(p)
	}
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_overrides`).Scan(&count)
	if count != 0 {
		t.Fatal("one-time grant survived successful processing", count)
	}
}

func TestOneTimeGrantsSurviveFurtherClarificationWithoutPingPong(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	other := NotificationRule{ID: "OTHER", Text: "Ignore open PRs by this author", Enabled: true, Clauses: []RuleClause{{Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "all", Args: []RuleExpression{{Op: "eq", Fact: "pr.state", Value: "open"}, {Op: "eq", Fact: "pr.author", Value: "someone"}}}}}}
	activateTestRules(t, s, []NotificationRule{uncertainOwnRule(), other})
	e := event()
	e.PRFacts = &PRFacts{Available: true, State: "closed"}
	e.EvidenceComplete = true
	provider := &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
	s.Providers["github"] = provider
	seed(t, s, source, e)
	p := process(t, s, model, "direct_question")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	continueCard := func() {
		t.Helper()
		cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
		if err != nil || len(cards.Items) != 1 {
			t.Fatal(cards, err)
		}
		q := cards.Items[0]
		if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "rule_continue_once"); err != nil {
			t.Fatal(err)
		}
		if err = s.executeNext(ctx); err != nil {
			t.Fatal(err)
		}
	}
	continueCard()
	e.PRFacts = &PRFacts{Available: true, State: "open"}
	provider.fresh = e
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal(p)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_overrides`).Scan(&count)
	if count != 1 {
		t.Fatal("grant was consumed before the decision resolved", count)
	}
	continueCard()
	p = process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal("clarifications sent the user back to the first rule", p)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 1 || strings.Contains(cards.Items[0].Context, "SELF:") || strings.Contains(cards.Items[0].Context, "OTHER:") {
		t.Fatal("second confirmation did not resolve uncertainty", cards)
	}
	s.DB.QueryRow(`SELECT count(*) FROM notification_rule_overrides`).Scan(&count)
	if count != 0 {
		t.Fatal("resolved one-time grants were not consumed", count)
	}
}

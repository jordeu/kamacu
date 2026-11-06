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
	"kamacu/internal/store"
)

func TestRuleExpressionsAndLocalExceptions(t *testing.T) {
	rules := repositoryTestRules("seqeralabs/platform")
	for _, tc := range []struct {
		name, repo string
		mention    *bool
		want       string
	}{
		{"excluded without mention", "seqeralabs/platform", ptr(false), "suppressed"},
		{"missing mention evidence", "seqeralabs/platform", nil, "held"},
		{"direct mention exception", "seqeralabs/platform", ptr(true), "admitted"},
		{"other repository does not require mention", "acme/other", nil, "admitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := map[string]RuleFact{"repository": {Known: true, Value: tc.repo}}
			if tc.mention != nil {
				facts["viewer.mentioned"] = RuleFact{Known: true, Value: *tc.mention}
			}
			got, evals, _ := evaluateRuleSet(rules, facts, nil, false)
			if got != tc.want || len(evals) != 1 {
				t.Fatalf("%s %+v", got, evals)
			}
		})
	}
	rules.Rules = append(rules.Rules, NotificationRule{ID: "R002", Enabled: true, Clauses: []RuleClause{{ID: "R002.1", Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "eq", Fact: "pr.draft", Value: true}}}})
	got, _, _ := evaluateRuleSet(rules, map[string]RuleFact{"repository": {Known: true, Value: "seqeralabs/platform"}, "viewer.mentioned": {Known: true, Value: true}, "pr.draft": {Known: true, Value: true}}, nil, false)
	if got != "suppressed" {
		t.Fatal("mention exception escaped its own rule", got)
	}
}
func ptr(v bool) *bool { return &v }

func TestRuleValidationRejectsWeakeningAndConflicts(t *testing.T) {
	for _, e := range []RuleExpression{{Op: "always", Fact: "viewer.mentioned"}, {Op: "eq", Fact: "invented", Value: true}, {Op: "not"}, {Op: "eq", Fact: "pr.draft", Value: "true"}, {Op: "semantic", Text: "", Value: true}} {
		if validateExpression(e, 0) == nil {
			t.Fatalf("accepted ambiguous expression %+v", e)
		}
	}
	rule := NotificationRule{ID: "R1", Revision: 1, Text: "Do this", Enabled: true, CompilerVersion: ruleCompilerVersion, Clauses: []RuleClause{{ID: "R1.1", Category: "action", Effect: "route_review", When: RuleExpression{Op: "always"}, OnUnknown: "hold"}, {ID: "R1.2", Category: "action", Effect: "route_investigation", When: RuleExpression{Op: "always"}, OnUnknown: "hold"}}}
	if validateRuleSet(RuleSet{Rules: []NotificationRule{rule}}, true) == nil {
		t.Fatal("accepted contradictory equal-priority routing")
	}
	rule.Clauses[1].Priority = 100
	if err := validateRuleSet(RuleSet{Rules: []NotificationRule{rule}}, true); err != nil {
		t.Fatal(err)
	}
}
func TestOnlyHighestPriorityRoutingCanHold(t *testing.T) {
	rules := RuleSet{Rules: []NotificationRule{{ID: "R1", Enabled: true, Clauses: []RuleClause{
		{ID: "R1.1", Category: "action", Effect: "route_review", Priority: 0, When: RuleExpression{Op: "always"}},
		{ID: "R1.2", Category: "action", Effect: "route_investigation", Priority: 0, When: RuleExpression{Op: "always"}},
		{ID: "R1.3", Category: "action", Effect: "route_implementation", Priority: 10, When: RuleExpression{Op: "always"}},
		{ID: "R1.4", Category: "action", Effect: "route_recurring", Priority: 0, When: RuleExpression{Op: "eq", Fact: "actor", Value: "bot"}},
	}}}}
	for i := range rules.Rules[0].Clauses {
		rules.Rules[0].Clauses[i].OnUnknown = "hold"
	}
	got, _, _ := evaluateRuleSet(rules, nil, nil, false)
	if got != "admitted" {
		t.Fatal("lower priority conflict or unknown overrode a known rule", got)
	}
	rules.Rules[0].Clauses[3].Priority = 20
	got, _, _ = evaluateRuleSet(rules, nil, nil, false)
	if got != "held" {
		t.Fatal("unknown higher priority must hold", got)
	}
}

func TestRuleCompilationActivationAndCache(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	active, _ := s.activeRuleSet(ctx)
	draft, err := s.SaveRuleDraft(ctx, active.Revision, []NotificationRule{{Text: "Ignore platform unless directly mentioned", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateRules(ctx, draft.ID); err == nil {
		t.Fatal("uncompiled draft activated")
	}
	compiled := draft.Rules[0]
	compiled.CompilerVersion = ruleCompilerVersion
	compiled.Clauses = repositoryTestRules("seqeralabs/platform").Rules[0].Clauses
	compiled.Clauses[0].ID = compiled.ID + ".1"
	model.responses = []any{map[string]string{"compiled_json": mustJSON([]NotificationRule{compiled})}}
	if _, err = s.QueueRuleCompilation(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.compileNextRuleSet(ctx); err != nil {
		t.Fatal(err)
	}
	ready, err := scanRuleSet(s.DB.QueryRow(`SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, draft.ID))
	if err != nil || ready.Status != "ready" {
		t.Fatalf("%+v %v", ready, err)
	}
	if current, _ := s.activeRuleSet(ctx); current.ID != active.ID {
		t.Fatal("compilation changed active rules")
	}
	if _, err = s.ActivateRules(ctx, ready.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.SaveRuleDraft(ctx, ready.Revision, ready.Rules)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = nil
	if _, err = s.QueueRuleCompilation(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.compileNextRuleSet(ctx); err != nil {
		t.Fatal(err)
	}
	cached, _ := scanRuleSet(s.DB.QueryRow(`SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, next.ID))
	if cached.Status != "ready" {
		t.Fatal("unchanged rules were not cached", cached)
	}
	if _, err = s.SaveRuleDraft(ctx, ready.Revision, ready.Rules); !errors.Is(err, ErrRulesChanged) {
		t.Fatal("stale editor overwrote newer draft", err)
	}
}
func TestInvalidCompilerOutputNeverActivates(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	active, _ := s.activeRuleSet(ctx)
	draft, err := s.SaveRuleDraft(ctx, active.Revision, []NotificationRule{{Text: "Group related work", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	model.responses = []any{map[string]string{"compiled_json": `[{"id":"invented","revision":1,"text":"changed","enabled":true,"clauses":[],"compiler_version":"1"}]`}}
	s.QueueRuleCompilation(ctx, draft.ID)
	if err = s.compileNextRuleSet(ctx); err != nil {
		t.Fatal(err)
	}
	sets, _ := s.RuleSets(ctx)
	if sets[0].Status != "invalid" || len(sets[0].Diagnostics) == 0 {
		t.Fatal(sets)
	}
	if got, _ := s.activeRuleSet(ctx); got.ID != active.ID {
		t.Fatal("invalid compilation replaced active rules")
	}
}

type frozenRuleProvider struct {
	*fakeProvider
	fresh Event
}

func (p *frozenRuleProvider) Enrich(context.Context, Source, Event) (Event, error) {
	return p.fresh, nil
}

func TestPlatformRuleProducesNoOverrideCard(t *testing.T) {
	for _, mode := range []string{"absent", "unknown", "mentioned"} {
		t.Run(mode, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			if _, err := s.DB.Exec(`UPDATE projects SET github_repo='seqeralabs/platform' WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			rule := repositoryTestRules("seqeralabs/platform").Rules[0]
			rule.Text = "Ignore platform unless directly cited"
			activateTestRules(t, s, []NotificationRule{rule})
			e := event()
			e.Repository = "seqeralabs/platform"
			e.URL = "https://github.com/seqeralabs/platform/pull/13408"
			e.Version = "2026-11-06T01:00:00Z"
			e.Reason = "review_requested"
			body := "Routine project update"
			if mode == "mentioned" {
				body = "@alice please inspect this change"
			}
			e.Evidence = []SubjectEvidence{{ID: e.URL + "#comment", URL: e.URL + "#comment", Text: body, UpdatedAt: e.Version, Complete: true}}
			e.EvidenceComplete = mode != "unknown"
			if mode == "unknown" {
				e.ContextWarnings = []string{"Could not load comment: HTTP 404"}
			}
			e.PRFacts = &PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"team:codeowners"}}
			s.Providers["github"] = &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
			seed(t, s, source, e)
			status := process(t, s, model, "direct_question")
			if status.Status != "completed" {
				t.Fatal(status)
			}
			want := 0
			if mode == "mentioned" {
				want = 1
			}
			if status.ActionCount != want {
				t.Fatalf("unexpected cards %+v", status)
			}
			if mode != "mentioned" && len(model.prompts) != 0 {
				t.Fatal("excluded/held subjects were sent to the writer")
			}
			var held int
			if err := s.DB.QueryRow(`SELECT held FROM notification_events LIMIT 1`).Scan(&held); err != nil {
				t.Fatal(err)
			}
			if (held == 1) != (mode == "unknown") {
				t.Fatal("wrong held disposition", held)
			}
			if mode == "unknown" {
				pending, _ := s.Status(ctx)
				if pending.PendingCount != 0 || pending.HeldCount != 1 {
					t.Fatal(pending)
				}
				if _, err := s.RequestProcessing(ctx); !errors.Is(err, ErrNoPending) {
					t.Fatal("held inputs automatically retried", err)
				}
				page, err := s.ListRaw(ctx, "held", 0)
				if err != nil || len(page.Items) != 1 || len(page.Items[0].Decisions) == 0 {
					t.Fatalf("missing held explanation %+v %v", page, err)
				}
				e.EvidenceComplete = true
				e.ContextWarnings = nil
				s.Providers["github"] = &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
				if n, err := s.RetryHeld(ctx); err != nil || n != 1 {
					t.Fatal(n, err)
				}
				retry := process(t, s, model, "direct_question")
				if retry.Status != "completed" || retry.ActionCount != 0 {
					t.Fatal(retry)
				}
			}
		})
	}
}

func TestRuleChangeDuringDraftingCannotPublish(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	respondWithIntent(model, "direct_question")
	old := model.respond
	changed := false
	model.respond = func(prompt string) (any, error) {
		if !changed && strings.Contains(prompt, projectCardsMarker) {
			changed = true
			activateTestRules(t, s, nil)
		}
		return old(prompt)
	}
	status := process(t, s, model, "")
	if status.Status != "failed" || !strings.Contains(status.Error, "rules changed") {
		t.Fatal(status)
	}
	var cards, archived int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&cards)
	s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='archived'`).Scan(&archived)
	if cards != 0 || archived != 0 {
		t.Fatal("published stale rules", cards, archived)
	}
}
func TestSemanticConditionsRequireEveryCheckAndEvidence(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: "comment", Text: "Can this be merged?", Complete: true, Current: true}}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, findings: []intentFinding{{Intent: "direct_question", EvidenceID: "comment", Quote: "Can this be merged?"}}}
	set := RuleSet{Rules: []NotificationRule{{ID: "R1", Enabled: true, Clauses: []RuleClause{{ID: "R1.1", Category: "filtering", Effect: "suppress", When: RuleExpression{Op: "semantic", Text: "Is the question resolved?"}, OnUnknown: "hold"}}}}}
	for _, result := range []string{"true", "false", "unknown"} {
		t.Run(result, func(t *testing.T) {
			subject.findings = []intentFinding{{Intent: "direct_question", EvidenceID: "comment", Quote: "Can this be merged?"}}
			model.responses = []any{map[string]any{"results": []any{map[string]string{"id": "1", "result": result, "evidence_id": "comment", "quote": "Can this be merged?"}}}}
			if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"true": "suppressed", "false": "admitted", "unknown": "held"}[result]
			if len(subject.decisions[0].Semantic) != 1 || subject.decisions[0].Semantic[0].EvidenceID != "comment" {
				t.Fatal("semantic citation missing", subject.decisions)
			}
			if subject.decisions[0].Outcome != want {
				t.Fatal(subject.decisions)
			}
		})
	}
	subject.findings = []intentFinding{{Intent: "direct_question", EvidenceID: "comment", Quote: "Can this be merged?"}}
	model.responses = []any{map[string]any{"results": []any{map[string]string{"id": "1", "result": "true", "evidence_id": "invented", "quote": "not source text"}}}}
	if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err == nil {
		t.Fatal("accepted invented semantic evidence")
	}
}

func TestRulePreviewDoesNotPublishOrArchive(t *testing.T) {
	s, model, _, source := fixture(t)
	set, _ := s.activeRuleSet(context.Background())
	id := seed(t, s, source, event())
	respondWithIntent(model, "direct_question")
	p, err := s.QueueRulePreview(context.Background(), set.ID, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processRulePreview(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err = s.RulePreview(context.Background(), p.ID)
	if err != nil || p.Status != "completed" {
		t.Fatalf("%+v %v", p, err)
	}
	var result RulePreviewResult
	if err = json.Unmarshal(p.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Cards) != 1 || len(result.Decisions) != 1 {
		t.Fatal(result)
	}
	var cards, batches int
	var raw string
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&cards)
	s.DB.QueryRow(`SELECT count(*) FROM notification_batches`).Scan(&batches)
	s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&raw)
	if cards != 0 || batches != 0 || raw != "pending" {
		t.Fatalf("preview mutated work: %d %d %s", cards, batches, raw)
	}
}
func TestTypedPlanAndFollowupReferences(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	status := process(t, s, model, "direct_question")
	if status.Status != "completed" {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	q := page.Items[0]
	if strings.Contains(q.Context, "<!-- kamacu-triage-evidence:") {
		t.Fatal("routing metadata remains embedded in prose")
	}
	var original string
	if err := s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, q.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inbox.Answer(context.Background(), nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	followup, err := s.Inbox.ContinueNotification(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	var copied string
	s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, followup.ID).Scan(&copied)
	if original != copied {
		t.Fatal("follow-up changed its validated plan")
	}
}
func TestUnsupportedMutationsCannotEscapePlan(t *testing.T) {
	s, _, _, _ := fixture(t)
	plan := ActionPlan{Workflow: "route_review", ProjectID: 1, PRNumber: 7, Targets: []ReviewTarget{{1, 7}}}
	for _, name := range []string{"create_task", "delete_task", "move_task"} {
		if err := s.validatePlannedTool(context.Background(), plan, name, json.RawMessage(`{"task_id":1,"project_id":1}`)); err == nil {
			t.Fatal("allowed unplanned mutation", name)
		}
	}
	if err := s.validatePlannedTool(context.Background(), plan, "open_review", json.RawMessage(`{"project_id":1,"pr_number":7}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.validatePlannedTool(context.Background(), plan, "open_review", json.RawMessage(`{"project_id":1,"pr_number":8}`)); err == nil {
		t.Fatal("wrong PR allowed")
	}
}

func TestRulesAreRequiredAndOldSettingsRemoved(t *testing.T) {
	s, _, _, source := fixture(t)
	s.DB.Exec(`UPDATE notification_rule_state SET active_set_id=NULL WHERE id=1`)
	seed(t, s, source, event())
	if _, err := s.RequestProcessing(context.Background()); !errors.Is(err, ErrNoActiveRules) {
		t.Fatal(err)
	}
	for _, old := range []string{"notification_policy", "notification_system_prompt"} {
		var count int
		if err := s.DB.QueryRow(`SELECT count(*) FROM settings WHERE key=?`, old).Scan(&count); err != nil || count != 0 {
			t.Fatal(old, count, err)
		}
	}
}
func ExampleRuleExpression() {
	fmt.Println("Factual conditions are evaluated by code; semantic conditions require source evidence.")
	// Output: Factual conditions are evaluated by code; semantic conditions require source evidence.
}

func TestActivationRetiresPendingCardsAndRequeuesTheirSources(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	first := process(t, s, model, "direct_question")
	if first.Status != "completed" {
		t.Fatal(first)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	old := page.Items[0]
	activateTestRules(t, s, nil)
	retired, err := s.Inbox.Get(ctx, old.ID)
	if err != nil || retired.Status != "unanswered" || retired.Reason != "notification_rules_changed" {
		t.Fatalf("stale card is still actionable: %+v %v", retired, err)
	}
	status, _ := s.Status(ctx)
	if status.PendingCount != 1 {
		t.Fatal("source was not requeued", status)
	}
	if next := process(t, s, model, "direct_question"); next.Status != "completed" || next.ActionCount != 1 {
		t.Fatal("retired activity could not be reconsidered", next)
	}
}
func TestDevelopmentRuleImportIsDraftOnly(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	migration, err := os.ReadFile("../store/migrations/00037_notification_rules.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(migration), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	instructions := "Ignore platform unless directly mentioned.\n\nGroup Renovate reviews by project."
	if _, err = s.DB.Exec(`INSERT INTO settings(key,value) VALUES('notification_system_prompt',?),('notification_policy','{"ignore_drafts":true}')`, instructions); err != nil {
		t.Fatal(err)
	}
	id := seed(t, s, source, event())
	q, err := s.Inbox.PublishNotification(ctx, id, "GitHub", event().URL, ask().question())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE notification_events SET raw_status='archived' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	sets, err := s.RuleSets(ctx)
	if err != nil || len(sets) != 1 || sets[0].Status != "draft" || len(sets[0].Rules) != 3 {
		t.Fatalf("import: %+v %v", sets, err)
	}
	if _, err = s.activeRuleSet(ctx); !errors.Is(err, ErrNoActiveRules) {
		t.Fatal("import silently activated rules", err)
	}
	if sets[0].Rules[0].Text != "Ignore platform unless directly mentioned." || sets[0].Rules[1].Text != "Group Renovate reviews by project." {
		t.Fatal("import changed original instructions", sets)
	}
	old, err := s.Inbox.Get(ctx, q.ID)
	if err != nil || old.Status != "unanswered" {
		t.Fatal("retired path still has an actionable card", old, err)
	}
	var rawStatus string
	if err = s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&rawStatus); err != nil || rawStatus != "pending" {
		t.Fatal("invalidated card input was lost", rawStatus, err)
	}
	var sources, tasks int
	s.DB.QueryRow(`SELECT count(*) FROM notification_sources`).Scan(&sources)
	s.DB.QueryRow(`SELECT count(*) FROM tasks`).Scan(&tasks)
	if sources != 1 || tasks != 2 {
		t.Fatal("import lost development data")
	}
}
func TestImplementationOwnerIsSelectedBeforeGrouping(t *testing.T) {
	s, model, _, source := fixture(t)
	model.ownerRespond = true
	ctx := context.Background()
	s.DB.Exec(`UPDATE tasks SET title='Fix CRIU ghost-file fallback' WHERE id=1`)
	rule := NotificationRule{ID: "OWNER", Text: "Resume the implementation owner for feedback", Enabled: true, Clauses: []RuleClause{{Category: "action", Effect: "route_implementation", OnUnknown: "hold", When: RuleExpression{Op: "eq", Fact: "activity.kind", Value: "review_feedback"}}}}
	activateTestRules(t, s, []NotificationRule{rule})
	seed(t, s, source, event())
	respondWithIntent(model, "review_feedback")
	base := model.respond
	model.respond = func(prompt string) (any, error) {
		if _, raw, ok := strings.Cut(prompt, "ACTIVITIES:\n"); ok {
			var input struct {
				Activities []struct {
					ID string `json:"activity_id"`
				} `json:"activities"`
			}
			if err := json.Unmarshal([]byte(raw), &input); err != nil {
				return nil, err
			}
			return map[string]string{"compiled_json": mustJSON(map[string]any{"choices": []any{map[string]any{"activity_id": input.Activities[0].ID, "task_id": 1, "resolution": "owner", "title_quote": "CRIU ghost-file fallback", "reason": "The task implements this fix."}}})}, nil
		}
		return base(prompt)
	}
	p := process(t, s, model, "")
	if p.Status != "completed" {
		t.Fatal(p)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(page.Items) != 1 {
		t.Fatalf("expected owner card, got %+v", p)
	}
	var raw string
	s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, page.Items[0].ID).Scan(&raw)
	var plan ActionPlan
	json.Unmarshal([]byte(raw), &plan)
	if plan.TaskID != 1 || plan.Workflow != "route_implementation" || plan.OwnerEvidence == "" {
		t.Fatal("ownership was left to the card writer", plan)
	}
}

func TestMixedHeldActivitiesDoNotRepeatPublishedActivity(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	rule := NotificationRule{ID: "RSEM", Text: "Ignore questions that are already resolved", Enabled: true, Clauses: []RuleClause{{Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "all", Args: []RuleExpression{{Op: "eq", Fact: "activity.kind", Value: "direct_question"}, {Op: "semantic", Text: "Is this question already resolved?"}}}}}}
	activateTestRules(t, s, []NotificationRule{rule})
	e := event()
	e.Version = "2026-11-06T01:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: "feedback", Text: "I approved the change.", UpdatedAt: e.Version, Complete: true}, {ID: "question", Text: "Can you check the release?", UpdatedAt: e.Version, Complete: true}}
	s.Providers["github"] = &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
	seed(t, s, source, e)
	resolved := "unknown"
	model.respond = func(prompt string) (any, error) {
		if strings.Contains(prompt, "CHECKS:\n") {
			return map[string]any{"results": []any{map[string]string{"id": "1", "result": resolved, "evidence_id": "question", "quote": "Can you check the release?"}}}, nil
		}
		if strings.Contains(prompt, projectEvidenceMarker) {
			return intentResponse{Findings: []intentFinding{{Intent: "review_feedback", EvidenceID: "feedback", Quote: "I approved the change."}, {Intent: "direct_question", EvidenceID: "question", Quote: "Can you check the release?"}}}, nil
		}
		return fixtureProjectCards(prompt, nil)
	}
	first := process(t, s, model, "")
	if first.Status != "completed" || first.ActionCount != 1 {
		t.Fatal(first)
	}
	status, _ := s.Status(ctx)
	if status.HeldCount != 1 {
		t.Fatal("mixed input was archived", status)
	}
	raw, err := s.ListRaw(ctx, "held", 0)
	if err != nil || len(raw.Items) != 1 {
		t.Fatal(raw, err)
	}
	found := false
	for _, d := range raw.Items[0].Decisions {
		if len(d.Semantic) > 0 && d.Semantic[0].Quote == "Can you check the release?" {
			found = true
		}
	}
	if !found {
		t.Fatal("semantic audit was not persisted", raw)
	}
	resolved = "false"
	if _, err := s.RetryHeld(ctx); err != nil {
		t.Fatal(err)
	}
	next := process(t, s, model, "")
	if next.Status != "completed" || next.ActionCount != 1 {
		t.Fatal("retry repeated delivered activity", next)
	}
	status, _ = s.Status(ctx)
	if status.HeldCount != 0 {
		t.Fatal("resolved activity stayed held")
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&count)
	if count != 2 {
		t.Fatal("duplicated previously admitted activity", count)
	}
}
func TestPreviewExplainsAlreadyProcessedEventsWithoutDeduplicatingThem(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	if p := process(t, s, model, "direct_question"); p.Status != "completed" {
		t.Fatal(p)
	}
	rules, _ := s.activeRuleSet(ctx)
	result, err := s.previewRules(ctx, rules, []int64{id})
	if err != nil || len(result.Cards) != 1 || result.Decisions[0].Outcome != "admitted" {
		t.Fatalf("preview hid a rule result behind receipt deduplication: %+v %v", result, err)
	}
}
func TestProvisioningAloneCannotClaimReviewStarted(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`)
	q := publish(t, s, seed(t, s, source, event()))
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Start review", ""); err != nil {
		t.Fatal(err)
	}
	model.steps = []Step{{Action: "tool", ToolName: "open_review", Arguments: `{"project_id":1,"pr_number":7}`}, {Action: "done", Message: "Review started"}}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(ctx, q.ID)
	if got.DeliveryStatus != "uncertain" || len(tools.calls) != 1 || !strings.Contains(got.ExecutionResult, "agent was not started") {
		t.Fatal(got, tools.calls)
	}
}
func TestResumingReviewDoesNotOverwriteFollowupWithInitialSeed(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`)
	s.DB.Exec(`INSERT INTO notification_review_prompts(task_id,prompt,state) VALUES(1,'Initial review seed','sent')`)
	prompt, err := s.reviewPrompt(ctx, 1, map[string]Event{}, map[string]Source{source.ID: source})
	if err != nil || prompt != "" {
		t.Fatal("resume would replace the follow-up", prompt, err)
	}
}

func TestTeamReviewReasonCannotBecomePersonalReview(t *testing.T) {
	e := event()
	e.EvidenceComplete = true
	e.PRFacts = &PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"team:codeowners"}}
	evidence := SubjectEvidence{ID: "notification", Text: "review_requested", Complete: true, Current: true}
	subject := &triageSubject{viewer: "alice", fresh: e, events: []RawNotification{{Event: e}}}
	no := false
	normalizeProjectFindings(subject, []intentFinding{{Intent: "review_request", EvidenceID: evidence.ID, Quote: evidence.Text}}, projectSubjectData{Evidence: []SubjectEvidence{evidence}, DirectMention: &no, RetrievalComplete: true, ContextComplete: false}, RuleSet{})
	if len(subject.findings) != 1 || subject.findings[0].Intent != "routine" {
		t.Fatal("unverified request was admitted", subject.findings)
	}
	evidence.Text = "@alice please review this PR"
	yes := true
	normalizeProjectFindings(subject, []intentFinding{{Intent: "review_request", EvidenceID: evidence.ID, Quote: evidence.Text}}, projectSubjectData{Evidence: []SubjectEvidence{evidence}, DirectMention: &yes, RetrievalComplete: true, ContextComplete: true}, RuleSet{})
	if len(subject.findings) != 1 || subject.findings[0].Intent != "review_request" {
		t.Fatal("direct source request lost", subject.findings)
	}
}

func TestIndependentActivitiesWithSameIntentKeepSeparateRuleDecisions(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	e := event()
	e.Version = "2026-11-06T01:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: "own", Text: "I fixed it", Actor: "alice", UpdatedAt: e.Version, Current: true, Complete: true}, {ID: "approval", Text: "LGTM", Actor: "reviewer", UpdatedAt: e.Version, Current: true, Complete: true}}
	subject := &triageSubject{viewer: "alice", fresh: e, events: []RawNotification{{Event: e}}}
	normalizeProjectFindings(subject, []intentFinding{{Intent: "review_feedback", EvidenceID: "own", Quote: "I fixed it"}, {Intent: "review_feedback", EvidenceID: "approval", Quote: "LGTM"}}, projectSubjectData{Evidence: e.Evidence, ContextComplete: true, RetrievalComplete: true}, RuleSet{})
	set := RuleSet{Rules: []NotificationRule{{ID: "OWN", Enabled: true, Clauses: []RuleClause{{ID: "OWN.1", Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "eq", Fact: "activity.own", Value: true}}}}}}
	if err := s.evaluateActivities(ctx, []*triageSubject{subject}, set); err != nil {
		t.Fatal(err)
	}
	if len(subject.decisions) != 2 || subject.decisions[0].Outcome != "suppressed" || subject.decisions[1].Outcome != "admitted" || len(subject.findings) != 1 || subject.findings[0].EvidenceID != "approval" {
		t.Fatalf("one activity hid independent feedback: %+v %+v", subject.decisions, subject.findings)
	}
}

func TestActionOverridesClearOwnerAndSupportExplicitRecurringReviews(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	for _, tc := range []struct{ intent, effect, workflow string }{
		{"review_feedback", "route_investigation", "route_implementation"},
		{"review_request", "route_recurring", "route_recurring"},
	} {
		t.Run(tc.effect, func(t *testing.T) {
			taskID := int64(1)
			taskName := ""
			if tc.effect == "route_recurring" {
				taskID = 1
				taskName = "Daily"
				if _, err := s.DB.Exec(`UPDATE tasks SET recurrent=1,title='Daily' WHERE id=1`); err != nil {
					t.Fatal(err)
				}
			}
			f := intentFinding{Intent: tc.intent, EvidenceID: "comment", Quote: "Please check"}
			subject := &triageSubject{projectID: 1, fresh: event()}
			subject.fresh.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}}
			subject.matched = map[string][]RuleClause{activityID(subject, f): {{ID: "R.1", Category: "action", Effect: tc.effect, Argument: map[bool]string{true: "Daily"}[tc.effect == "route_recurring"]}}}
			plan, err := s.activityActionPlan(ctx, subject, f, RuleSet{})
			if err != nil || plan.Workflow != tc.workflow || plan.TaskID != taskID || plan.TaskName != taskName {
				t.Fatal(plan, err)
			}
		})
	}
}

func TestFollowupCopiesReplannedExecutionTargets(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	process(t, s, model, "direct_question")
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	plan := ActionPlan{Workflow: "route_implementation", ProjectID: 1, TaskID: 2}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET plan=? WHERE id=?`, mustJSON(plan), q.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.Inbox.ContinueNotification(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, next.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var copied ActionPlan
	if err = json.Unmarshal([]byte(raw), &copied); err != nil || copied.TaskID != 2 {
		t.Fatal(raw, err)
	}
}

func TestFollowupOptionIsReplannedInsteadOfBlindlyConfirmingOriginal(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	process(t, s, model, "direct_question")
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := s.Inbox.PublishNotificationFollowup(ctx, q.ID, ask().question())
	if err != nil {
		t.Fatal(err)
	}
	next.Options = []inbox.Option{{Label: "Use task 1"}}
	next.Answer = "Use task 1"
	original := ActionPlan{Workflow: "route_investigation", ProjectID: 1}
	selected := original
	selected.Workflow = "route_implementation"
	selected.TaskID = 1
	model.planResponse = map[string]any{"plan_json": mustJSON(selected), "explicit_change": true, "user_quote": "Use task 1", "request_kind": "followup"}
	got, err := s.replyActionPlan(ctx, original, next, "")
	if err != nil || got.TaskID != 1 {
		t.Fatal(got, err)
	}
}

func TestMergedPRRetiresOnlyReviewCards(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			s, _, _, source := fixture(t)
			ctx := context.Background()
			id := seed(t, s, source, event())
			q := publish(t, s, id)
			plan := ActionPlan{Workflow: "route_review", Evidence: []triageEvidence{{Intent: "review_request"}}}
			if mixed {
				plan.Evidence = append(plan.Evidence, triageEvidence{Intent: "direct_question"})
			}
			if _, err := s.DB.Exec(`UPDATE notification_card_plans SET plan=? WHERE question_id=?`, mustJSON(plan), q.ID); err != nil {
				t.Fatal(err)
			}
			e := event()
			e.PRFacts = &PRFacts{Available: true, State: "merged"}
			subject := &triageSubject{fresh: e, events: []RawNotification{{ID: id, SourceID: source.ID, Event: e}}}
			decision, err := s.renderTriage(ctx, []*triageSubject{subject}, []inbox.Question{q}, RuleSet{})
			if err != nil {
				t.Fatal(err)
			}
			if (!mixed && len(decision.SupersedeIDs) != 1) || (mixed && len(decision.SupersedeIDs) != 0) {
				t.Fatal(decision)
			}
		})
	}
}

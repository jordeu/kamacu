package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCompilerKeepsOtherRulesWhenOneNeedsLLM(t *testing.T) {
	s, model, _, _ := fixture(t)
	active, _ := s.activeRuleSet(context.Background())
	draft, err := s.SaveRuleDraft(context.Background(), active.Revision, []NotificationRule{{Text: "Ignore draft PRs", Enabled: true}, {Text: "Group Renovate reviews in my preferred workflow", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	compiled := append([]NotificationRule(nil), draft.Rules...)
	compiled[0].CompilerVersion = ruleCompilerVersion
	compiled[0].Mode = "compiled"
	compiled[0].Clauses = []RuleClause{{ID: compiled[0].ID + ".1", Category: "filtering", Effect: "suppress", When: RuleExpression{Op: "eq", Fact: "pr.draft", Value: true}, OnUnknown: "hold"}}
	compiled[1].CompilerVersion = ruleCompilerVersion
	compiled[1].Mode = "llm"
	compiled[1].Diagnostic = "The preferred workflow is not specified."
	compiled[1].Alternatives = []string{"Group review requests on PRs authored by seqeralabs-renovate by project and use the recurring task named seqeralabs-renovate."}
	model.responses = []any{map[string]string{"compiled_json": mustJSON(compiled)}}
	if _, err = s.QueueRuleCompilation(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.compileNextRuleSet(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready, err := s.RuleSet(context.Background(), draft.ID)
	if err != nil || ready.Status != "ready" || ready.Rules[0].Mode != "compiled" || ready.Rules[1].Mode != "llm" || len(ready.Rules[1].Alternatives) != 1 {
		t.Fatal(ready, err)
	}
	if _, err = s.ActivateRules(context.Background(), ready.ID); err != nil {
		t.Fatal(err)
	}
	ready.Rules[1].Enabled = false
	next, err := s.SaveRuleDraft(context.Background(), ready.Revision, ready.Rules)
	if err != nil || next.Rules[1].Diagnostic == "" || len(next.Rules[1].Alternatives) == 0 {
		t.Fatal("toggling lost the explanation", next, err)
	}
}

func TestLLMFallbackEvaluatesCompoundRuleAndHoldsUnknown(t *testing.T) {
	for _, result := range []string{"true", "unknown"} {
		t.Run(result, func(t *testing.T) {
			s, model, _, _ := fixture(t)
			e := event()
			e.EvidenceComplete = true
			e.Evidence = []SubjectEvidence{{ID: "comment", Text: "Please review these Renovate changes", Complete: true, Current: true}}
			f := intentFinding{Intent: "review_request", EvidenceID: "comment", Quote: e.Evidence[0].Text}
			subject := &triageSubject{projectID: 1, fresh: e, events: []RawNotification{{Event: e}}, findings: []intentFinding{f}}
			rule := NotificationRule{ID: "R1", Revision: 1, Text: "Group Renovate reviews by project and run recurring task Renovate", Enabled: true, Mode: "llm"}
			effects := []any{}
			for i, c := range []RuleClause{{Category: "grouping", Effect: "group_project"}, {Category: "action", Effect: "route_recurring", Argument: "Renovate"}} {
				c.ID = rule.ID + "." + string(rune('1'+i))
				c.When = RuleExpression{Op: "semantic", Text: c.ID}
				c.OnUnknown = "hold"
				c.Priority = 100
				effects = append(effects, map[string]any{"clause": c, "result": result, "evidence_id": "comment", "quote": f.Quote})
			}
			model.responses = []any{map[string]string{"compiled_json": mustJSON(map[string]any{"results": []any{map[string]any{"id": activityID(subject, f) + ":R1", "effects": effects}}})}}
			set := RuleSet{Rules: []NotificationRule{rule}}
			if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err != nil {
				t.Fatal(err)
			}
			want := "admitted"
			if result == "unknown" {
				want = "held"
			}
			if len(subject.decisions) != 1 || subject.decisions[0].Outcome != want || len(subject.decisions[0].Semantic) != 2 {
				t.Fatal(subject.decisions)
			}
			if result == "true" {
				if _, err := s.DB.Exec(`UPDATE tasks SET recurrent=1,title='Renovate' WHERE id=1`); err != nil {
					t.Fatal(err)
				}
				plan, err := s.activityActionPlan(context.Background(), subject, f, set)
				if err != nil || plan.Workflow != "route_recurring" || plan.TaskID != 1 {
					t.Fatal(plan, err)
				}
			}
		})
	}
}

func TestCompiledExclusionSkipsLLMFallback(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	f := intentFinding{Intent: "direct_question", EvidenceID: "context", Quote: "Question"}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, findings: []intentFinding{f}}
	set := RuleSet{Rules: []NotificationRule{{ID: "R1", Enabled: true, Clauses: []RuleClause{{ID: "R1.1", Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "always"}}}}, {ID: "R2", Revision: 1, Text: "Interpret this fully", Enabled: true, Mode: "llm"}}}
	if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != 0 || subject.decisions[0].Outcome != "suppressed" {
		t.Fatal(model.prompts, subject.decisions)
	}
}

func TestLLMFallbackRejectsMissingOrInventedEvidence(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	e.Evidence = []SubjectEvidence{{ID: "comment", Text: "Question", Complete: true}}
	f := intentFinding{Intent: "direct_question", EvidenceID: "comment", Quote: "Question"}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, findings: []intentFinding{f}}
	set := RuleSet{Rules: []NotificationRule{{ID: "R1", Revision: 1, Text: "Interpret", Enabled: true, Mode: "llm"}}}
	for _, response := range []string{`{"results":[]}`, `{"results":[{"id":"ID","effects":[{"clause":{"id":"R1.1","category":"filtering","when":{"op":"semantic","text":"R1.1"},"effect":"suppress","argument":"","priority":0,"on_unknown":"hold"},"result":"true","evidence_id":"invented","quote":"invented"}]}]}`} {
		response = strings.ReplaceAll(response, "ID", activityID(subject, f)+":R1")
		model.responses = []any{map[string]string{"compiled_json": response}}
		if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err == nil {
			t.Fatal("accepted invalid fallback", response)
		}
	}
}

func TestExplicitLLMRuleDoesNotRequireCompilationModel(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	active, _ := s.activeRuleSet(ctx)
	draft, err := s.SaveRuleDraft(ctx, active.Revision, []NotificationRule{{Text: "Interpret my workflow", Enabled: true, Mode: "llm", Clauses: []RuleClause{{Effect: "suppress"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s.Model = nil
	ready, err := s.compileRuleSet(ctx, draft)
	if err != nil || len(ready.Rules[0].Clauses) != 0 {
		t.Fatal(ready, err)
	}
	raw, _ := json.Marshal(ready)
	if !strings.Contains(string(raw), `"mode":"llm"`) {
		t.Fatal(string(raw))
	}
}

package notifications

import (
	"context"
	"encoding/json"
	"testing"
)

func activateTestRules(t *testing.T, s *Service, rules []NotificationRule) RuleSet {
	t.Helper()
	var base int64
	if err := s.DB.QueryRow(`SELECT COALESCE(max(revision),0) FROM notification_rule_sets`).Scan(&base); err != nil {
		t.Fatal(err)
	}
	for i := range rules {
		rules[i].CompilerVersion = ruleCompilerVersion
		if rules[i].Revision == 0 {
			rules[i].Revision = 1
		}
		for j := range rules[i].Clauses {
			rules[i].Clauses[j].ID = rules[i].ID + "." + string(rune('1'+j))
		}
	}
	saved := mustJSON(rules)
	set, err := s.SaveRuleDraft(context.Background(), base, rules)
	if err != nil {
		t.Fatal(err)
	}
	// Supply a frozen compiler result directly; integration compiler tests exercise the real validation path separately.
	if err = json.Unmarshal([]byte(saved), &rules); err != nil {
		t.Fatal(err)
	}
	set.Rules = rules
	if err = validateRuleSet(set, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE notification_rule_sets SET rules_json=?,status='ready' WHERE id=?`, mustJSON(rules), set.ID); err != nil {
		t.Fatal(err)
	}
	set, err = s.ActivateRules(context.Background(), set.ID)
	if err != nil {
		t.Fatal(err)
	}
	return set
}
func repositoryTestRules(repo string) RuleSet {
	return RuleSet{Rules: []NotificationRule{{ID: "R001", Enabled: true, Clauses: []RuleClause{{ID: "R001.1", Category: "filtering", Effect: "suppress", OnUnknown: "hold", When: RuleExpression{Op: "all", Args: []RuleExpression{{Op: "eq", Fact: "repository", Value: repo}, {Op: "not", Args: []RuleExpression{{Op: "eq", Fact: "viewer.mentioned", Value: true}}}}}}}}}}
}
func groupTestRule() NotificationRule {
	return NotificationRule{ID: "RG", Text: "Group project reviews and run renovate", Enabled: true, Clauses: []RuleClause{{Category: "grouping", Effect: "group_project", OnUnknown: "hold", When: RuleExpression{Op: "always"}}, {Category: "action", Effect: "route_recurring", Argument: "renovate", OnUnknown: "hold", When: RuleExpression{Op: "always"}}}}
}

func attachTestPlan(t *testing.T, s *Service, id string) {
	t.Helper()
	set, err := s.activeRuleSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan := ActionPlan{RuleSetID: set.ID, Workflow: "route_implementation", ProjectID: 1, TaskID: 1, PRNumber: 7, Targets: []ReviewTarget{{1, 7}}, Evidence: []triageEvidence{{Intent: "review_feedback"}}}
	var source string
	if err = s.DB.QueryRow(`SELECT source FROM tasks WHERE id=1`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source == "github_pr" {
		plan.Workflow = "route_review"
		plan.Evidence = []triageEvidence{{Intent: "review_request"}}
	}
	var context string
	if err = s.DB.QueryRow(`SELECT context FROM inbox_questions WHERE id=?`, id).Scan(&context); err != nil {
		t.Fatal(err)
	}
	if context == "Review this PR" {
		plan.Workflow = "route_review"
		plan.Evidence = []triageEvidence{{Intent: "review_request"}}
	}

	if _, err = s.DB.Exec(`INSERT INTO notification_card_plans(question_id,rule_set_id,plan) VALUES(?,?,?) ON CONFLICT(question_id) DO UPDATE SET rule_set_id=excluded.rule_set_id,plan=excluded.plan`, id, set.ID, mustJSON(plan)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE inbox_questions SET notification_group=id WHERE id=? AND notification_group=''`, id); err != nil {
		t.Fatal(err)
	}
}

package notifications

import (
	"context"
	"testing"
)

func configureTestModel(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.DB.Exec(`UPDATE notification_settings SET profile_id=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
}
func attachTestPlan(t *testing.T, s *Service, id string) {
	t.Helper()
	set, err := s.activeRuleSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan := ActionPlan{RuleSetID: set.ID, Workflow: "route_implementation", ProjectID: 1, TaskID: 1, PRNumber: 7, Targets: []ReviewTarget{{1, 7}}, Evidence: []ActionEvidence{{Intent: "review_feedback"}}}
	var source, body string
	s.DB.QueryRow(`SELECT source FROM tasks WHERE id=1`).Scan(&source)
	s.DB.QueryRow(`SELECT context FROM inbox_questions WHERE id=?`, id).Scan(&body)
	if source == "github_pr" || body == "Review this PR" {
		plan.Workflow = "route_review"
		plan.Evidence = []ActionEvidence{{Intent: "review_request"}}
	}
	if _, err = s.DB.Exec(`INSERT INTO notification_card_plans(question_id,rule_set_id,plan) VALUES(?,?,?) ON CONFLICT(question_id) DO UPDATE SET rule_set_id=excluded.rule_set_id,plan=excluded.plan`, id, set.ID, mustJSON(plan)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE inbox_questions SET notification_group=id WHERE id=? AND notification_group=''`, id); err != nil {
		t.Fatal(err)
	}
}

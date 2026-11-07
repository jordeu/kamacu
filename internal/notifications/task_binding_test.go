package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/store"
)

func TestStoppedOwnerIsResumedAndLinkedEvenWithNewSessionRule(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			if _, err := s.DB.Exec(`UPDATE tasks SET status='done' WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "implementation", Source: "explicit", Evidence: "This task implements the PR", ObservedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			s.TaskHasSession = func(id int64) bool { return active && id == 1 }
			s.TaskSession = func(id int64) string {
				if active && id == 1 {
					return "correct"
				}
				return ""
			}
			rule := NotificationRule{ID: "R005", Text: "If work needs a new session, create a task", Enabled: true, Clauses: []RuleClause{{Category: "action", Effect: "route_investigation", When: RuleExpression{Op: "always"}, OnUnknown: "hold"}}}
			activateTestRules(t, s, []NotificationRule{rule})
			seed(t, s, source, event())
			p := process(t, s, model, "direct_question")
			if p.Status != "completed" || p.ActionCount != 1 {
				t.Fatal(p)
			}
			page, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if err != nil || len(page.Items) != 1 {
				t.Fatal(page, err)
			}
			q := page.Items[0]
			if q.TaskID != 0 || len(q.LinkedTasks) != 1 || q.LinkedTasks[0].TaskID != 1 || q.LinkedTasks[0].Status != "done" {
				t.Fatal("navigation link lost or terminal ownership changed", q)
			}
			var raw string
			s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, q.ID).Scan(&raw)
			var plan ActionPlan
			if err = json.Unmarshal([]byte(raw), &plan); err != nil {
				t.Fatal(err)
			}
			mode := "resume"
			if active {
				mode = "continue"
			}
			if plan.Workflow != "route_implementation" || plan.TaskID != 1 || plan.SessionMode != mode {
				t.Fatal(plan)
			}
			if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
				t.Fatal(err)
			}
			if err = s.executeNext(ctx); err != nil {
				t.Fatal(err)
			}
			next, err := s.Inbox.ContinueNotification(ctx, q.ID)
			if err != nil || len(next.LinkedTasks) != 1 || next.LinkedTasks[0].TaskID != 1 {
				t.Fatal("follow-up lost its task", next, err)
			}
		})
	}
}

func TestTeamReviewOpportunityUsesReviewWorkspaceWithoutPersonalAssignment(t *testing.T) {
	s, model, tools, source := fixture(t)
	e := event()
	e.EvidenceComplete = true
	e.Version = "2026-11-06T01:00:00Z"
	e.Evidence = []SubjectEvidence{{ID: "comment", Actor: "author", Text: "Ready for review @acme/reviewers", Complete: true, Current: true, UpdatedAt: e.Version}}
	e.PRFacts = &PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"team:reviewers"}}
	s.Providers["github"] = &frozenRuleProvider{fakeProvider: &fakeProvider{}, fresh: e}
	rule := NotificationRule{ID: "R005", Text: "New session means new task", Enabled: true, Clauses: []RuleClause{{Category: "action", Effect: "route_investigation", When: RuleExpression{Op: "always"}, OnUnknown: "hold"}}}
	activateTestRules(t, s, []NotificationRule{rule})
	seed(t, s, source, e)
	model.respond = func(prompt string) (any, error) {
		if strings.Contains(prompt, projectEvidenceMarker) {
			return intentResponse{Findings: []intentFinding{{Intent: "needs_context", WorkKind: "review", Audience: "team", Uncertainty: "Only the team was mentioned", EvidenceID: "comment", Quote: e.Evidence[0].Text}}}, nil
		}
		return fixtureProjectCards(prompt, nil)
	}
	p := process(t, s, model, "")
	if p.Status != "completed" || p.ActionCount != 1 {
		t.Fatal(p)
	}
	var raw string
	if err := s.DB.QueryRow(`SELECT plan FROM notification_card_plans`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plan ActionPlan
	json.Unmarshal([]byte(raw), &plan)
	if plan.Workflow != "route_review" || plan.WorkKind != "review" || plan.TaskID != 0 {
		t.Fatal(plan)
	}
	if len(tools.calls) != 0 {
		t.Fatal("review workspace created without acceptance")
	}
}

func TestExistingReviewWorkspaceAndAmbiguousOwners(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	f := intentFinding{Intent: "needs_context", WorkKind: "review", Audience: "team"}
	subject := &triageSubject{projectID: 1, fresh: event()}
	plan, err := s.activityActionPlan(ctx, subject, f, RuleSet{})
	if err != nil || plan.Workflow != "route_review" || plan.TaskID != 1 || len(plan.Bindings) != 1 {
		t.Fatal(plan, err)
	}
	if _, err = s.DB.Exec(`UPDATE tasks SET source='manual',pr_number=NULL,project_id=1 WHERE id IN (1,2)`); err != nil {
		t.Fatal(err)
	}
	subject.fresh.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}, {Link: store.PRLink{TaskID: 2, Role: "implementation"}}}
	f = intentFinding{Intent: "needs_context", WorkKind: "implementation"}
	if err = s.prepareActivityPlans(ctx, []*triageSubject{subject}, RuleSet{}); err != nil {
		t.Fatal(err)
	}
	plan, err = s.activityActionPlan(ctx, subject, f, RuleSet{})
	if err != nil || plan.Workflow != "clarify" || plan.TaskID != 0 || len(plan.Bindings) != 2 {
		t.Fatal(plan, err)
	}
}

func TestSessionPRLinkCanIdentifyOwnerWithoutTaskPRLink(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`INSERT INTO session_pr_links(session_id,repository,number,role,source,evidence,uncertainty,observed_at,task_id) VALUES('past','acme/repo',7,'implementation','terminal','Implemented this PR','','2026-11-06T01:00:00Z',1)`); err != nil {
		t.Fatal(err)
	}
	subject := &triageSubject{projectID: 1, fresh: event()}
	plan, err := s.activityActionPlan(ctx, subject, intentFinding{Intent: "direct_question"}, RuleSet{})
	if err != nil || plan.TaskID != 1 || len(plan.Bindings) != 1 {
		t.Fatal(plan, err)
	}
	if err = store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "incidental", Source: "user", Evidence: "This task does not own this PR", Corrected: true, ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	plan, err = s.activityActionPlan(ctx, subject, intentFinding{Intent: "direct_question"}, RuleSet{})
	if err != nil || plan.TaskID != 0 || len(plan.Bindings) != 0 {
		t.Fatal("explicit rejection was ignored", plan, err)
	}
}

func TestOwnershipLookupRunsWithoutSpecificRuleAndKeepsAmbiguity(t *testing.T) {
	for _, resolution := range []string{"owner", "none", "ambiguous"} {
		t.Run(resolution, func(t *testing.T) {
			s, model, _, _ := fixture(t)
			model.ownerRespond = true
			ctx := context.Background()
			if _, err := s.DB.Exec(`UPDATE tasks SET title='Implement output sharing',status='done' WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			f := intentFinding{Intent: "needs_context", WorkKind: "implementation", EvidenceID: "comment", Quote: "How does output sharing work?"}
			subject := &triageSubject{projectID: 1, fresh: event(), findings: []intentFinding{f}}
			model.respond = func(prompt string) (any, error) {
				var requests struct {
					Activities []struct {
						ID string `json:"activity_id"`
					} `json:"activities"`
				}
				_, raw, _ := strings.Cut(prompt, "ACTIVITIES:\n")
				if err := json.Unmarshal([]byte(raw), &requests); err != nil {
					return nil, err
				}
				taskID := int64(0)
				title := ""
				if resolution == "owner" {
					taskID = 1
					title = "output sharing"
				}
				return map[string]string{"compiled_json": mustJSON(map[string]any{"choices": []any{map[string]any{"activity_id": requests.Activities[0].ID, "task_id": taskID, "resolution": resolution, "title_quote": title, "reason": "Supplied task evidence"}}})}, nil
			}
			if err := s.prepareActivityPlans(ctx, []*triageSubject{subject}, RuleSet{}); err != nil {
				t.Fatal(err)
			}
			plan := subject.plans[activityID(subject, f)]
			want := map[string]string{"owner": "route_implementation", "none": "route_investigation", "ambiguous": "clarify"}[resolution]
			if plan.Workflow != want {
				t.Fatal(plan)
			}
			if resolution == "owner" && (plan.TaskID != 1 || len(plan.Bindings) != 1 || plan.Bindings[0].Status != "done") {
				t.Fatal(plan)
			}
		})
	}
}

func TestTaskLinkMigrationPreservesHistoryAndRequeuesStaleCards(t *testing.T) {
	s, _, _, source := fixture(t)
	downgradeClarifications(t, s)
	ctx := context.Background()
	raw, err := os.ReadFile("../store/migrations/00038_notification_task_links.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	questionID := q.ID
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	set, _ := s.activeRuleSet(ctx)
	plan := ActionPlan{ProjectID: 1, Targets: []ReviewTarget{{1, 7}}, Workflow: "route_investigation"}
	if _, err = s.DB.Exec(`UPDATE notification_card_plans SET rule_set_id=?,plan=? WHERE question_id=?`, set.ID, mustJSON(plan), questionID); err != nil {
		t.Fatal(err)
	}
	if err = store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "implementation", Source: "user", Evidence: "Existing owner", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE notification_events SET raw_status='archived' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	got, err := s.Inbox.Get(ctx, questionID)
	if err != nil || got.Status != "unanswered" || len(got.LinkedTasks) != 1 || got.LinkedTasks[0].TaskID != 1 {
		t.Fatal(got, err)
	}
	var status string
	if err = s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM tasks WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	got, err = s.Inbox.Get(ctx, questionID)
	if err != nil || len(got.LinkedTasks) != 0 {
		t.Fatal("deleted task left a broken link", got, err)
	}
}

func TestNavigationLinksDoNotResolveNotification(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if err := s.Inbox.LinkNotificationTasks(ctx, q.ID, []inbox.TaskLink{{TaskID: 1, ProjectID: 1, Role: "implementation", Source: "user", Evidence: "Verified owner"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := s.Inbox.Get(ctx, q.ID)
		if err != nil || got.Status != "pending" || got.Answer != "" || len(got.LinkedTasks) != 1 {
			t.Fatal(got, err)
		}
	}
}

func TestProvisionedReviewTaskIsLinkedBeforeAgentStarts(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	q := publish(t, s, seed(t, s, source, event()))
	var raw string
	s.DB.QueryRow(`SELECT plan FROM notification_card_plans WHERE question_id=?`, q.ID).Scan(&raw)
	var plan ActionPlan
	json.Unmarshal([]byte(raw), &plan)
	plan.Workflow = "route_review"
	plan.TaskID = 0
	plan.Bindings = nil
	if _, err := s.DB.Exec(`UPDATE notification_card_plans SET plan=? WHERE question_id=?`, mustJSON(plan), q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, q.Options[0].Label, ""); err != nil {
		t.Fatal(err)
	}
	model.steps = []Step{{Action: "tool", ToolName: "open_review", Arguments: `{"project_id":1,"pr_number":7}`}, {Action: "done", Message: "Ready"}}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Inbox.Get(ctx, q.ID)
	if err != nil || len(got.LinkedTasks) != 1 || got.LinkedTasks[0].TaskID != 1 || got.LinkedTasks[0].Role != "review" {
		t.Fatal("provisioned review is not navigable", got, err)
	}
}

func TestWriterCannotSuggestOrdinaryTaskForReviewPlan(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	f := intentFinding{Intent: "needs_context", WorkKind: "review", Audience: "team", EvidenceID: "comment", Quote: "Ready for review"}
	subject := &triageSubject{projectID: 1, fresh: event(), events: []RawNotification{{ID: 1, SourceID: source.ID, Event: event()}}, findings: []intentFinding{f}}
	decision, err := s.renderTriage(ctx, []*triageSubject{subject}, nil, RuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	model.respond = func(prompt string) (any, error) {
		return fixtureProjectCards(prompt, &triageDraft{Summary: "The team was asked to review", Context: "A team review opportunity", Question: "Review it?", Options: []string{"Create an ordinary task"}})
	}
	if err = s.draftProjectCards(ctx, &decision, []*triageSubject{subject}, RuleSet{}, ""); err != nil {
		t.Fatal(err)
	}
	if len(decision.Actions) != 1 || decision.Actions[0].Options[0] != "Start review" {
		t.Fatal(decision)
	}
}

func TestIncompleteTaskCatalogDoesNotProposeNewTask(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	for i := 0; i < 61; i++ {
		if _, err := s.DB.Exec(`INSERT INTO tasks(project_id,title,status,source,position,created_at,updated_at) VALUES(1,?,'done','manual',0,0,?)`, fmt.Sprint("Older task ", i), i); err != nil {
			t.Fatal(err)
		}
	}
	f := intentFinding{Intent: "direct_question", EvidenceID: "comment", Quote: "Please check"}
	subject := &triageSubject{projectID: 1, fresh: event(), findings: []intentFinding{f}}
	if err := s.prepareActivityPlans(ctx, []*triageSubject{subject}, RuleSet{}); err != nil {
		t.Fatal(err)
	}
	plan := subject.plans[activityID(subject, f)]
	if plan.Workflow != "clarify" || plan.TaskID != 0 {
		t.Fatal("incomplete lookup was treated as proof of no owner", plan)
	}
	if len(model.prompts) != 1 || !strings.Contains(model.prompts[0], `"candidates_complete":false`) {
		t.Fatal(model.prompts)
	}
}

func TestLinkedInvestigationCannotCreateAnotherTask(t *testing.T) {
	s, _, _, _ := fixture(t)
	plan := ActionPlan{Workflow: "route_investigation", ProjectID: 1, TaskID: 1}
	if err := s.validatePlannedTool(context.Background(), plan, "create_task", json.RawMessage(`{"project_id":1}`)); err == nil {
		t.Fatal("existing investigation could create duplicate work")
	}
	if label := notificationActionLabel(plan); label != "Resume task 1" {
		t.Fatal(label)
	}
}

func TestTeamReviewKindSurvivesEligibilityNormalization(t *testing.T) {
	no := false
	e := event()
	e.EvidenceComplete = true
	evidence := SubjectEvidence{ID: "comment", Text: "Ready for review @acme/reviewers", Complete: true, Current: true}
	e.Evidence = []SubjectEvidence{evidence}
	e.PRFacts = &PRFacts{Available: true, Viewer: "alice", State: "open"}
	subject := &triageSubject{viewer: "alice", fresh: e, events: []RawNotification{{Event: e}}}
	normalizeProjectFindings(subject, []intentFinding{{Intent: "review_request", WorkKind: "review", Audience: "team", EvidenceID: "comment", Quote: evidence.Text}}, projectSubjectData{Evidence: e.Evidence, DirectMention: &no, RetrievalComplete: true, ContextComplete: true}, RuleSet{})
	if len(subject.findings) != 1 || subject.findings[0].Intent != "needs_context" || !isReviewWork(subject.findings[0]) || subject.findings[0].Audience != "team" {
		t.Fatal(subject.findings)
	}
}

func TestMergedPRRetiresOptionalReviewCard(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	plan := ActionPlan{WorkKind: "review", Workflow: "route_review", Evidence: []triageEvidence{{Intent: "needs_context"}}}
	if _, err := s.DB.Exec(`UPDATE notification_card_plans SET plan=? WHERE question_id=?`, mustJSON(plan), q.ID); err != nil {
		t.Fatal(err)
	}
	e := event()
	e.PRFacts = &PRFacts{Available: true, State: "merged"}
	subject := &triageSubject{fresh: e, events: []RawNotification{{ID: id, SourceID: source.ID, Event: e}}}
	decision, err := s.renderTriage(ctx, []*triageSubject{subject}, []inbox.Question{q}, RuleSet{})
	if err != nil || len(decision.SupersedeIDs) != 1 || decision.SupersedeIDs[0] != q.ID {
		t.Fatal(decision, err)
	}
}

func TestReviewActionKeepsSourceImplementationLink(t *testing.T) {
	s, _, _, _ := fixture(t)
	f := intentFinding{Intent: "needs_context", WorkKind: "review", Audience: "team"}
	subject := &triageSubject{projectID: 1, fresh: event()}
	subject.fresh.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}}
	plan, err := s.activityActionPlan(context.Background(), subject, f, RuleSet{})
	if err != nil || plan.Workflow != "route_review" || plan.TaskID != 0 || len(plan.Bindings) != 1 || plan.Bindings[0].TaskID != 1 {
		t.Fatal(plan, err)
	}
}

func TestMigrationBackfillsSessionOwnersAndReviewWorkspaces(t *testing.T) {
	s, _, _, source := fixture(t)
	downgradeClarifications(t, s)
	ctx := context.Background()
	if _, err := s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE tasks SET project_id=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	q := publish(t, s, seed(t, s, source, event()))
	plan := ActionPlan{Workflow: "route_review", WorkKind: "review", ProjectID: 1, Targets: []ReviewTarget{{1, 7}}}
	if _, err := s.DB.Exec(`UPDATE notification_card_plans SET plan=? WHERE question_id=?`, mustJSON(plan), q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO session_pr_links(session_id,repository,number,role,source,evidence,uncertainty,observed_at,task_id) VALUES('past','acme/repo',7,'implementation','terminal','Implemented it','','2026-11-06T01:00:00Z',2)`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../store/migrations/00038_notification_task_links.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	got, err := s.Inbox.Get(ctx, q.ID)
	if err != nil || len(got.LinkedTasks) != 2 {
		t.Fatal(got, err)
	}
	roles := map[int64]string{}
	for _, link := range got.LinkedTasks {
		roles[link.TaskID] = link.Role
	}
	if roles[1] != "review" || roles[2] != "implementation" {
		t.Fatal(roles)
	}
}

func TestOptionalReviewSessionFactUsesReviewRole(t *testing.T) {
	yes, no := true, false
	e := event()
	e.SessionRouting = &RuleFact{Known: true, Value: yes}
	e.ReviewSessionRouting = &RuleFact{Known: true, Value: no}
	finding := intentFinding{Intent: "needs_context", WorkKind: "review", Audience: "team"}
	fact := subjectRuleFacts(&triageSubject{fresh: e}, &finding)["workflow.needs_session"]
	if !fact.Known || fact.Value != false {
		t.Fatal("optional review used implementation session state", fact)
	}
}

func TestRejectedAssociationCannotBeReselectedByTitleMatching(t *testing.T) {
	s, model, _, _ := fixture(t)
	model.ownerRespond = true
	ctx := context.Background()
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "incidental", Source: "user", Evidence: "Not the owner", Corrected: true, ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f := intentFinding{Intent: "needs_context", WorkKind: "implementation", Quote: "Please check"}
	subject := &triageSubject{projectID: 1, fresh: event(), findings: []intentFinding{f}}
	// Simulate an older retrieved association too: the latest correction wins.
	subject.fresh.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}}
	model.respond = func(prompt string) (any, error) {
		var input struct {
			Activities []struct {
				ID       string  `json:"activity_id"`
				Excluded []int64 `json:"excluded_task_ids"`
			} `json:"activities"`
		}
		_, raw, _ := strings.Cut(prompt, "ACTIVITIES:\n")
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, err
		}
		if len(input.Activities) != 1 || len(input.Activities[0].Excluded) != 1 || input.Activities[0].Excluded[0] != 1 {
			t.Fatal("correction missing from ownership context", input)
		}
		return map[string]string{"compiled_json": mustJSON(map[string]any{"choices": []any{map[string]any{"activity_id": input.Activities[0].ID, "task_id": 1, "resolution": "owner", "title_quote": "Implement", "reason": "Same title"}}})}, nil
	}
	if err := s.prepareActivityPlans(ctx, []*triageSubject{subject}, RuleSet{}); err == nil {
		t.Fatal("model overrode an explicit rejected association")
	}
}

func TestOwnershipCorrectionBlocksStaleCardButAllowsExplicitRetarget(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	plan := ActionPlan{Workflow: "route_implementation", ProjectID: 1, TaskID: 1, Targets: []ReviewTarget{{1, 7}}, Bindings: []inbox.TaskLink{{TaskID: 1, ProjectID: 1, Role: "implementation", Source: "task_match"}}}
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "incidental", Source: "user", Evidence: "Not the owner", Corrected: true, ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"task_id":1}`)
	if err := s.validatePlannedTool(ctx, plan, "start_task_agent", args); err == nil {
		t.Fatal("stale card could start rejected task")
	}
	// A new explicit user target is an action instruction, not an automatic
	// reassignment of the PR's durable ownership record.
	plan.Bindings[0].Source = "user_reply"
	if err := s.validatePlannedTool(ctx, plan, "start_task_agent", args); err != nil {
		t.Fatal(err)
	}
}

func TestExistingBindingIsRevalidatedBeforeReuse(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	plan := ActionPlan{ProjectID: 1, Bindings: []inbox.TaskLink{{TaskID: 1, ProjectID: 1, Role: "implementation", Source: "pr_link"}}}
	if err := s.addPlanBinding(ctx, &plan, 1, "recurring", "rule", "Recurring target"); err == nil {
		t.Fatal("cached binding bypassed recurring-task validation")
	}
	if _, err := s.DB.Exec(`UPDATE tasks SET project_id=2 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := s.addPlanBinding(ctx, &plan, 1, "implementation", "confirmed_plan", "Continue"); err == nil {
		t.Fatal("cached binding bypassed project validation")
	}
}

func TestExplicitReviewRuleKeepsSourceOwnerButSelectsReviewWorkspace(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`UPDATE tasks SET project_id=1,source='github_pr',pr_number=7 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	f := intentFinding{Intent: "direct_question", WorkKind: "implementation", EvidenceID: "comment"}
	subject := &triageSubject{projectID: 1, fresh: event()}
	subject.fresh.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}}
	subject.matched = map[string][]RuleClause{activityID(subject, f): {{ID: "RREVIEW.1", Category: "action", Effect: "route_review", Priority: 100}}}
	plan, err := s.activityActionPlan(ctx, subject, f, RuleSet{})
	if err != nil || plan.Workflow != "route_review" || plan.TaskID != 2 || len(plan.Bindings) != 2 {
		t.Fatal(plan, err)
	}
}

func TestInvalidTaskLinkDoesNotSucceedSilently(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	err := s.Inbox.LinkNotificationTasks(ctx, q.ID, []inbox.TaskLink{{TaskID: 1, ProjectID: 1, Role: "invalid", Source: "user"}})
	if err == nil {
		t.Fatal("invalid link role was silently discarded")
	}
}

func TestVerifiedCrossProjectOwnerIsResumedAndNavigable(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	// Task 2 belongs to project 2, but explicitly owns a PR in project 1's repo.
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 2, Repository: "acme/repo", Number: 7, Role: "implementation", Source: "user", Evidence: "This task owns the cross-repository work", Corrected: true, ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	subject := &triageSubject{projectID: 1, fresh: event()}
	f := intentFinding{Intent: "direct_question", WorkKind: "implementation"}
	// Fetch from current DB associations even when retrieval predates the link.
	plan, err := s.activityActionPlan(ctx, subject, f, RuleSet{})
	if err != nil || plan.Workflow != "route_implementation" || plan.TaskID != 2 || plan.ProjectID != 2 || len(plan.Bindings) != 1 || plan.Bindings[0].ProjectID != 2 {
		t.Fatal(plan, err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].ProjectID != 1 {
		t.Fatal("source PR identity was overwritten", plan)
	}
	if err = s.validatePlannedTool(ctx, plan, "start_task_agent", json.RawMessage(`{"task_id":2}`)); err != nil {
		t.Fatal(err)
	}
	// An explicit recurring rule still names the source project's recurring task.
	if _, err = s.DB.Exec(`UPDATE tasks SET recurrent=1,title='Batch reviews' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	subject.matched = map[string][]RuleClause{activityID(subject, f): {{ID: "R.1", Category: "action", Effect: "route_recurring", Argument: "Batch reviews"}}}
	plan, err = s.activityActionPlan(ctx, subject, f, RuleSet{})
	if err != nil || plan.Workflow != "route_recurring" || plan.TaskID != 1 || plan.ProjectID != 1 || len(plan.Bindings) != 2 {
		t.Fatal(plan, err)
	}
}

func TestGenericReplyDoesNotEraseOwnershipProvenance(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	plan := ActionPlan{Workflow: "route_implementation", ProjectID: 1, TaskID: 1, Targets: []ReviewTarget{{1, 7}}, Bindings: []inbox.TaskLink{{TaskID: 1, ProjectID: 1, Role: "implementation", Source: "pr_link", Evidence: "Original owner"}}}
	q.Answer = "Yes, go ahead"
	model.planResponse = map[string]any{"plan_json": mustJSON(plan), "explicit_change": false, "user_quote": "", "request_kind": "followup"}
	got, err := s.replyActionPlan(ctx, plan, q, "")
	if err != nil || len(got.Bindings) != 1 || got.Bindings[0].Source != "pr_link" {
		t.Fatal(got, err)
	}
}

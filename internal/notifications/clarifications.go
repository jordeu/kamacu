package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"kamacu/internal/inbox"
	"kamacu/internal/store"
)

type UncertainActivity struct {
	ID       string   `json:"id"`
	RuleIDs  []string `json:"rule_ids"`
	EventIDs []int64  `json:"event_ids"`
}

func missingRuleEvidence(e RuleExpression, facts map[string]RuleFact, semantic map[string]string) string {
	missing := []string{}
	var visit func(RuleExpression)
	visit = func(x RuleExpression) {
		if evalExpression(x, facts, semantic) != "unknown" {
			return
		}
		if x.Fact != "" {
			f := facts[x.Fact]
			reason := f.Reason
			if reason == "" {
				reason = "not supplied by the source"
			}
			missing = append(missing, x.Fact+": "+reason)
		}
		if x.Op == "semantic" {
			missing = append(missing, "Cannot establish: "+x.Text)
		}
		for _, child := range x.Args {
			visit(child)
		}
	}
	visit(e)
	if len(missing) == 0 {
		return "Conflicting or unavailable routing evidence"
	}
	slices.Sort(missing)
	return strings.Join(slices.Compact(missing), "; ")
}

// A subject override applies only to the precise notification versions the user
// saw. A later notification on the same PR has a different key.
func subjectFilterID(subject *triageSubject) string {
	versions := []string{subjectScope(subject)}
	for _, e := range subject.events {
		versions = append(versions, fmt.Sprintf("%d:%s", e.ID, e.Event.Version))
	}
	slices.Sort(versions)
	return evidenceFingerprint(strings.Join(versions, "\x00") + ":subject-filter")
}
func effectiveSubjectRules(set RuleSet, subject *triageSubject, id string) RuleSet {
	if len(subject.overrides) == 0 {
		return set
	}
	result := set
	result.Rules = append([]NotificationRule(nil), set.Rules...)
	for i, r := range result.Rules {
		if slices.Contains(subject.overrides[id], r.ID) || slices.Contains(subject.overrides[subjectFilterID(subject)], r.ID) {
			result.Rules[i].Enabled = false
		}
	}
	return result
}
func (s *Service) loadRuleOverrides(ctx context.Context, subject *triageSubject, set RuleSet) error {
	if ctx.Value(rulePreviewKey{}) == true {
		return nil
	}
	ids := []int64{}
	for _, e := range subject.events {
		ids = append(ids, e.ID)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT activity_id,rule_id FROM notification_rule_overrides WHERE rule_set_id=? AND question_id IN (SELECT question_id FROM notification_question_events WHERE event_id IN (SELECT value FROM json_each(?)))`, set.ID, mustJSON(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	subject.overrides = map[string][]string{}
	for rows.Next() {
		var id, rule string
		if err = rows.Scan(&id, &rule); err != nil {
			return err
		}
		subject.overrides[id] = append(subject.overrides[id], rule)
	}
	return rows.Err()
}

// These cards are application generated: uncertainty never asks a model to
// invent an investigation or select a task.
func (s *Service) clarificationActions(ctx context.Context, plans []notificationPlan) ([]inbox.NotificationBatchAction, error) {
	grouped := map[string]*inbox.NotificationBatchAction{}
	order := []string{}
	for _, p := range plans {
		for _, group := range p.groups {
			for _, subject := range group.subjects {
				for _, d := range subject.decisions {
					if d.Outcome != "clarification" {
						continue
					}
					rules, reasons := []string{}, []string{}
					for _, e := range d.Evaluations {
						if e.Blocking && e.Result == "unknown" {
							rules = append(rules, e.RuleID)
							reasons = append(reasons, e.RuleID+": "+e.Reason)
						}
					}
					if len(rules) == 0 {
						for _, e := range d.Evaluations {
							if e.Blocking && e.Result == "true" {
								rules = append(rules, e.RuleID)
							}
						}
						reasons = append(reasons, "Applicable routing rules conflict ("+strings.Join(rules, ", ")+"). Choose a rule change before starting work.")
					}
					slices.Sort(rules)
					rules = slices.Compact(rules)
					slices.Sort(reasons)
					reasons = slices.Compact(reasons)
					key := evidenceFingerprint(group.policy.ID + "\x00" + strings.Join(reasons, "\n"))
					a := grouped[key]
					if a == nil {
						a = &inbox.NotificationBatchAction{SourceName: subject.events[0].SourceName, SourceURL: subject.fresh.URL, Request: inbox.Create{Abstract: "Notification rules need clarification", Summary: "A filtering or routing decision needs your input. No task has been started.", Context: strings.Join(reasons, "\n") + "\n\nIgnore these updates, or process them once without the uncertain rules. Other rules still apply; starting work requires a separate Inbox decision. Improve rules to propose a permanent change.", Question: "How should these updates be handled?", Options: []inbox.Option{{ID: "triage_noop", Label: "Ignore these updates"}, {ID: "rule_continue_once", Label: "Process once without uncertain rules"}}}, Plan: mustJSON(ActionPlan{RuleSetID: group.policy.ID, Workflow: "clarify", ClarificationKey: key, RuleIDs: rules})}
						grouped[key] = a
						order = append(order, key)
					}
					var plan ActionPlan
					_ = json.Unmarshal([]byte(a.Plan), &plan)
					links, err := s.clarificationBindings(ctx, subject)
					if err != nil {
						return nil, err
					}
					for _, link := range links {
						if !slices.ContainsFunc(plan.Bindings, func(b inbox.TaskLink) bool { return b.TaskID == link.TaskID }) {
							plan.Bindings = append(plan.Bindings, link)
							a.Links = append(a.Links, link)
						}
					}
					plan.Activities = append(plan.Activities, d.ID)
					plan.Uncertain = append(plan.Uncertain, UncertainActivity{ID: d.ID, RuleIDs: rules, EventIDs: d.EventIDs})
					a.Plan = mustJSON(plan)
					for _, id := range d.EventIDs {
						if !slices.Contains(a.EventIDs, id) {
							a.EventIDs = append(a.EventIDs, id)
						}
					}
					link := "\n" + subject.fresh.URL
					if !strings.Contains(a.Request.Context, link) && len(a.Request.Context)+len(link) < 20000 {
						a.Request.Context += link
					}
				}
			}
		}
	}
	result := []inbox.NotificationBatchAction{}
	for _, key := range order {
		a := grouped[key]
		var plan ActionPlan
		_ = json.Unmarshal([]byte(a.Plan), &plan)
		a.Request.Summary = fmt.Sprintf("%d updates need a filtering or routing decision. No task has been started.", len(plan.Activities))
		result = append(result, *a)
	}
	return result, nil
}

func (s *Service) continueRuleClarification(ctx context.Context, q inbox.Question) error {
	var raw string
	if err := s.DB.QueryRowContext(ctx, `SELECT plan FROM notification_card_plans WHERE question_id=?`, q.ID).Scan(&raw); err != nil {
		return err
	}
	var plan ActionPlan
	if err := strictRuleJSON([]byte(raw), &plan); err != nil {
		return err
	}
	if plan.ClarificationKey == "" || len(plan.Uncertain) == 0 {
		return inbox.ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active string
	if err = tx.QueryRowContext(ctx, `SELECT active_set_id FROM notification_rule_state WHERE id=1`).Scan(&active); err != nil {
		return err
	}
	if active != plan.RuleSetID {
		return ErrRulesChanged
	}
	// Requeue only events represented by this saved message. Validate all
	// grants before writing anything so a malformed plan cannot affect another
	// notification or persist a partial override.
	for _, a := range plan.Uncertain {
		if !slices.Contains(plan.Activities, a.ID) || len(a.EventIDs) == 0 || len(a.RuleIDs) == 0 {
			return inbox.ErrInvalid
		}
		for _, rule := range a.RuleIDs {
			if !slices.Contains(plan.RuleIDs, rule) {
				return inbox.ErrInvalid
			}
		}
		for _, eventID := range a.EventIDs {
			var member int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_question_events WHERE question_id=? AND event_id=?`, q.ID, eventID).Scan(&member); err != nil {
				return err
			}
			if member != 1 {
				return inbox.ErrInvalid
			}
		}
	}
	for _, a := range plan.Uncertain {
		for _, rule := range a.RuleIDs {
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_rule_overrides(rule_set_id,activity_id,rule_id,question_id) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, active, a.ID, rule, q.ID); err != nil {
				return err
			}
		}
		for _, id := range a.EventIDs {
			if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET raw_status='pending',raw_revision=raw_revision+1 WHERE id=?`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func isNoveltyCondition(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "the review feedback is new since the user was last notified about this subject")
}
func (s *Service) addNoveltyFact(ctx context.Context, id string, facts map[string]RuleFact) error {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_card_plans p JOIN inbox_questions q ON q.id=p.question_id,json_each(p.plan,'$.activities') a WHERE a.value=? AND q.status IN ('pending','answered','dismissed') AND q.reason!='rule_continue_once'`, id).Scan(&count)
	if err != nil {
		return err
	}
	facts["activity.new"] = RuleFact{Known: true, Value: count == 0, Reason: "Exact activity version compared with delivered Inbox receipts, including dismissed messages"}
	return nil
}
func decisionSnapshot(subject *triageSubject) string {
	evidence, complete := classifierEvidence(subject.events, subject.fresh)
	raw := mustJSON(map[string]any{"evidence": evidence, "complete": complete, "raw_findings": subject.rawFindings, "normalized_findings": subject.classifiedFindings, "viewer": subject.viewer, "pr_facts": compactTriageFacts(subject.fresh.PRFacts)})
	if len(raw) > 96000 {
		return `{"omitted":"snapshot exceeds evidence budget"}`
	}
	return raw
}

// Navigation uses existing PR associations only. Clarification never invokes
// the model to invent a task owner or create a review workspace.
func (s *Service) clarificationBindings(ctx context.Context, subject *triageSubject) ([]inbox.TaskLink, error) {
	_, pr, err := store.PRIdentity(subject.fresh.URL, 0)
	if err != nil {
		return nil, nil
	}
	owners, err := s.implementationOwners(ctx, subject, pr)
	if err != nil {
		return nil, err
	}
	reviews, err := s.reviewTaskIDs(ctx, subject.projectID, pr)
	if err != nil {
		return nil, err
	}
	result := []inbox.TaskLink{}
	for _, role := range []string{"implementation", "review"} {
		ids := owners
		if role == "review" {
			ids = reviews
		}
		for _, id := range ids {
			var project int64
			if err = s.DB.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id=?`, id).Scan(&project); errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return nil, err
			}
			plan := ActionPlan{ProjectID: project}
			if err = s.addPlanBinding(ctx, &plan, id, role, "pr_context", "Existing task associated with "+subject.fresh.URL); err != nil {
				return nil, err
			}
			result = append(result, plan.Bindings...)
		}
	}
	return result, nil
}

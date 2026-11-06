package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kamacu/internal/monitor"
)

const ruleCompilerPrompt = `Compile user-authored notification rules into declarative clauses. Do not execute anything. Preserve the exact meaning of each rule, including scope, exceptions and uncertainty. Return {"compiled_json":"..."}, containing a JSON array of NotificationRule objects with exactly the supplied rule IDs, text, revision and enabled flags, compiler_version:"1", and clauses.
Clause fields: id (RULE_ID.1, RULE_ID.2...), category (filtering/grouping/action/presentation), when (expression), effect, argument (string), priority (integer 0–1000), on_unknown:"hold" (or "continue" only when the user explicitly permits proceeding without evidence).
Allowed category/effect pairs: filtering/suppress; grouping/group_project; action/route_review, route_implementation, route_recurring, route_investigation; presentation/format. argument is the exact recurring task name for route_recurring and the formatting instruction for format; otherwise empty. Only grouping by project is supported; diagnose other grouping dimensions instead of pretending group_project represents them. Use priority 100 for specific action/grouping instructions and 0 for generic ones. Split compound instructions into linked clauses. Do not add universal exceptions: "unless directly mentioned" applies only to its own rule. Express suppress-unless as all(scope, not(exception)). Rule text suggesting new work never authorizes execution. A generic create-task instruction means route_investigation. Review requests default to review workspaces. An explicit rule routing reviews to an existing named recurring task is supported; preserve that exception. Never route a review request to a new ordinary task.
Expressions: {"op":"always"}; {"op":"all"|"any","args":[expressions]}; {"op":"not","args":[one expression]}; {"op":"eq"|"in","fact":"...","value":scalar or array}; {"op":"semantic","text":"a precise condition to evaluate against source evidence"}.
Facts and types: workflow.needs_session (bool, computed from the selected linked implementation owner and known session lifecycle; use this fact for new-session versus continue-session conditions, never a semantic leaf), repository (lowercase owner/repo string), project_id (number), actor (string), subject.type (PullRequest/Issue/Release), subject.url (string), subject.title (string), activity.kind (review_request/review_feedback/direct_question/routine/needs_context), activity.work_kind (review/implementation/investigation), activity.audience (individual/team/unspecified), activity.uncertain (bool). Work kind is independent of uncertainty and audience. Use activity.work_kind=review for rules about review work generally; use activity.kind=review_request only when the rule specifically refers to personal review assignment. A new-session routing instruction never discards an already verified task owner, activity.own (bool), pr.state (open/closed/merged), pr.draft (bool), pr.author (string), review.state (APPROVED/CHANGES_REQUESTED/COMMENTED/DISMISSED), viewer.mentioned (bool), viewer.review_requested (verified individual assignment bool), viewer.approved (bool). Use pr.author for the author of a PR (including bot-authored review requests); actor refers to the cited activity author and may be unavailable for an assignment. Use factual conditions whenever possible. Use semantic leaves only for meaning unavailable from these facts, not for repository matching or mention detection. Unknown evidence must not be treated as false. Default to on_unknown hold; use continue only for an explicit best-effort instruction. Unsupported unknown dispositions must produce a diagnostic. Category determines the stage. Ignore PRs already approved means suppress review_request activities with viewer.approved=true, unless the user explicitly says to suppress all activities. Personal review assignment is not a direct mention. Preserve exact usernames and task names. "My" refers to the bound viewer facts, not an invented username.
If a rule cannot be represented faithfully, return that rule with mode:"llm", diagnostic explaining why, alternatives:[one to three self-contained rule rewrites that avoid the ambiguity or unsupported behavior], clauses:[], and compiler_version:"1". Continue compiling the other rules; never reject the whole set because one instruction needs LLM interpretation. For supported rules use mode:"compiled". A recurring review task explicitly requested by a rule is a supported override of the default review workspace.`

var ruleJSONSchema = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"compiled_json"}, "properties": map[string]any{"compiled_json": map[string]string{"type": "string"}}}

func (s *Service) QueueRuleCompilation(ctx context.Context, id string) (RuleSet, error) {
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_rule_sets SET status='compiling',diagnostics='[]',updated_at=? WHERE id=? AND status IN ('draft','invalid')`, time.Now().UnixMilli(), id)
	if err != nil {
		return RuleSet{}, err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return RuleSet{}, errors.New("only draft or invalid rules can be compiled")
	}
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, id))
}
func (s *Service) compileNextRuleSet(ctx context.Context) error {
	set, err := scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE status='compiling' ORDER BY revision LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	result, compileErr := s.compileRuleSet(ctx, set)
	if compileErr != nil {
		result = set
		result.Status = "invalid"
		result.Diagnostics = []string{compileErr.Error()}
	} else {
		result.Status = "ready"
		result.Diagnostics = []string{}
	}
	raw, _ := json.Marshal(result.Rules)
	diagnostics, _ := json.Marshal(result.Diagnostics)
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = s.DB.ExecContext(recordCtx, `UPDATE notification_rule_sets SET status=?,rules_json=?,diagnostics=?,updated_at=? WHERE id=? AND status='compiling'`, result.Status, string(raw), string(diagnostics), time.Now().UnixMilli(), set.ID)
	return err
}
func (s *Service) compileRuleSet(ctx context.Context, set RuleSet) (RuleSet, error) {
	changed := []NotificationRule{}
	for _, r := range set.Rules {
		if r.Enabled && r.Mode != "llm" && (r.CompilerVersion != ruleCompilerVersion || len(r.Clauses) == 0) {
			changed = append(changed, r)
		}
	}
	if len(changed) == 0 {
		return set, validateRuleSet(set, true)
	}
	if s.Model == nil {
		return set, errors.New("configure a model before compiling notification rules")
	}
	input, _ := json.Marshal(changed)
	if len(input) > projectPromptLimit-10000 {
		return set, errors.New("changed rules exceed compilation budget; split or shorten the draft")
	}
	profile, err := monitor.Resolve(ctx, s.DB, 0)
	if err != nil {
		return set, err
	}
	var compiled []NotificationRule
	check := func(raw []byte) error {
		var response struct {
			JSON string `json:"compiled_json"`
		}
		if err := strictRuleJSON(raw, &response); err != nil {
			return err
		}
		if len(response.JSON) > 128000 {
			return errors.New("compiled rules exceed budget")
		}
		var diagnostic struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(response.JSON), &diagnostic) == nil && diagnostic.Error != "" {
			compiled = make([]NotificationRule, len(changed))
			copy(compiled, changed)
			for i := range compiled {
				compiled[i].Mode = "llm"
				compiled[i].Diagnostic = diagnostic.Error
				compiled[i].Alternatives = []string{"Split this instruction into separate rules for filtering, grouping, and routing, with explicit repository or PR-author names."}
				compiled[i].Clauses = nil
				compiled[i].CompilerVersion = ruleCompilerVersion
			}
			return validateRuleSet(RuleSet{Rules: compiled}, true)
		}
		if err := strictRuleJSON([]byte(response.JSON), &compiled); err != nil {
			return err
		}
		if len(compiled) != len(changed) {
			return errors.New("compiler must preserve every rule")
		}
		seen := map[string]bool{}
		for _, r := range compiled {
			var original *NotificationRule
			for i := range changed {
				if changed[i].ID == r.ID {
					original = &changed[i]
					break
				}
			}
			if original == nil || seen[r.ID] || r.Text != original.Text || r.Revision != original.Revision || r.Enabled != original.Enabled {
				return errors.New("compiler changed rule identity or text")
			}
			if r.Mode == "llm" && (r.Diagnostic == "" || len(r.Alternatives) == 0) {
				return errors.New("LLM fallback requires a rule explanation and suggested alternatives")
			}
			seen[r.ID] = true
		}
		return validateRuleSet(RuleSet{Rules: compiled}, true)
	}
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err = notificationStage(ctx, "rule_compilation", func() error {
		_, err := s.Model.GenerateValidatedJSON(callCtx, profile, ruleCompilerPrompt+"\nRULES (trusted configuration):\n"+string(input), ruleJSONSchema, check)
		return err
	}); err != nil {
		return set, err
	}
	for i, r := range set.Rules {
		for _, c := range compiled {
			if c.ID == r.ID {
				set.Rules[i] = c
			}
		}
	}
	if err = validateRuleSet(set, true); err != nil {
		return set, fmt.Errorf("rule conflicts: %w", err)
	}
	return set, nil
}

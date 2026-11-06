package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ruleCompilerVersion = "1"

var ErrRulesChanged = errors.New("notification rules changed; process notifications again")
var ErrNoActiveRules = errors.New("activate a compiled notification rule set before processing")

type RuleSet struct {
	ID          string             `json:"id"`
	Revision    int64              `json:"revision"`
	Status      string             `json:"status"`
	Rules       []NotificationRule `json:"rules"`
	Diagnostics []string           `json:"diagnostics"`
}
type NotificationRule struct {
	Mode            string       `json:"mode,omitempty"`
	Diagnostic      string       `json:"diagnostic,omitempty"`
	Alternatives    []string     `json:"alternatives,omitempty"`
	ID              string       `json:"id"`
	Revision        int64        `json:"revision"`
	Text            string       `json:"text"`
	Enabled         bool         `json:"enabled"`
	Clauses         []RuleClause `json:"clauses"`
	CompilerVersion string       `json:"compiler_version,omitempty"`
}
type RuleClause struct {
	ID        string         `json:"id"`
	Category  string         `json:"category"`
	When      RuleExpression `json:"when"`
	Effect    string         `json:"effect"`
	Argument  string         `json:"argument"`
	Priority  int            `json:"priority"`
	OnUnknown string         `json:"on_unknown"`
}
type RuleExpression struct {
	Op    string           `json:"op"`
	Fact  string           `json:"fact,omitempty"`
	Value any              `json:"value,omitempty"`
	Args  []RuleExpression `json:"args,omitempty"`
	Text  string           `json:"text,omitempty"`
}
type RuleFact struct {
	ObservedAt string   `json:"observed_at,omitempty"`
	Known      bool     `json:"known"`
	Value      any      `json:"value"`
	Evidence   []string `json:"evidence"`
	Reason     string   `json:"reason,omitempty"`
}
type RuleEvaluation struct {
	OnUnknown string   `json:"on_unknown"`
	RuleID    string   `json:"rule_id"`
	ClauseID  string   `json:"clause_id"`
	Result    string   `json:"result"`
	Effect    string   `json:"effect"`
	Evidence  []string `json:"evidence"`
	Reason    string   `json:"reason,omitempty"`
}
type SemanticJudgment struct {
	Condition  string `json:"condition"`
	Result     string `json:"result"`
	EvidenceID string `json:"evidence_id,omitempty"`
	Quote      string `json:"quote,omitempty"`
}
type ActivityDecision struct {
	Semantic        []SemanticJudgment  `json:"semantic,omitempty"`
	RuleSetID       string              `json:"rule_set_id"`
	RuleSetRevision int64               `json:"rule_set_revision"`
	ID              string              `json:"activity_id"`
	EventIDs        []int64             `json:"event_ids"`
	Outcome         string              `json:"outcome"`
	Evaluations     []RuleEvaluation    `json:"evaluations"`
	Facts           map[string]RuleFact `json:"facts"`
}
type ReviewTarget struct {
	ProjectID int64 `json:"project_id"`
	PRNumber  int64 `json:"pr_number"`
}
type ActionPlan struct {
	OwnerEvidence string           `json:"owner_evidence,omitempty"`
	Targets       []ReviewTarget   `json:"targets"`
	Presentation  []string         `json:"presentation"`
	Evidence      []triageEvidence `json:"evidence"`
	RuleSetID     string           `json:"rule_set_id"`
	Activities    []string         `json:"activities"`
	Workflow      string           `json:"workflow"`
	ProjectID     int64            `json:"project_id"`
	TaskID        int64            `json:"task_id"`
	PRNumber      int64            `json:"pr_number"`
	TaskName      string           `json:"task_name"`
	RuleIDs       []string         `json:"rule_ids"`
}

var ruleFacts = map[string]string{
	"workflow.needs_session": "bool", "repository": "string", "project_id": "number", "actor": "string", "subject.type": "string", "subject.url": "string", "subject.title": "string", "activity.kind": "string", "activity.own": "bool", "pr.state": "string", "pr.draft": "bool", "pr.author": "string", "review.state": "string", "viewer.mentioned": "bool", "viewer.review_requested": "bool", "viewer.approved": "bool",
}

func strictRuleJSON(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("multiple or invalid JSON values")
	}
	return nil
}
func validateExpression(e RuleExpression, depth int) error {
	if depth > 8 {
		return errors.New("rule expression exceeds depth limit")
	}
	if e.Op == "eq" || e.Op == "in" {
		if len(e.Args) > 0 || e.Text != "" {
			return errors.New("comparison has unused fields")
		}
	} else if e.Op == "semantic" {
		if e.Fact != "" || e.Value != nil || len(e.Args) > 0 {
			return errors.New("semantic condition has unused fields")
		}
	} else {
		if e.Fact != "" || e.Value != nil || e.Text != "" {
			return errors.New("logical condition has unused fields")
		}
		if e.Op == "always" && len(e.Args) > 0 {
			return errors.New("always has unused conditions")
		}
	}
	switch e.Op {
	case "all", "any":
		if len(e.Args) == 0 || len(e.Args) > 20 {
			return errors.New("all/any requires 1–20 conditions")
		}
		for _, a := range e.Args {
			if err := validateExpression(a, depth+1); err != nil {
				return err
			}
		}
	case "not":
		if len(e.Args) != 1 {
			return errors.New("not requires one condition")
		}
		return validateExpression(e.Args[0], depth+1)
	case "always":
	case "semantic":
		if strings.TrimSpace(e.Text) == "" || len(e.Text) > 2000 {
			return errors.New("semantic condition requires a bounded description")
		}
	case "eq", "in":
		typ, ok := ruleFacts[e.Fact]
		if !ok {
			return fmt.Errorf("unsupported fact %q", e.Fact)
		}
		values := []any{e.Value}
		if e.Op == "in" {
			var ok bool
			values, ok = e.Value.([]any)
			if !ok || len(values) == 0 || len(values) > 100 {
				return errors.New("in requires 1–100 values")
			}
		}
		for _, v := range values {
			switch typ {
			case "string":
				if _, ok := v.(string); !ok {
					return errors.New("string fact requires string values")
				}
			case "bool":
				if _, ok := v.(bool); !ok {
					return errors.New("boolean fact requires true/false")
				}
			case "number":
				if _, ok := v.(float64); !ok {
					return errors.New("numeric fact requires number")
				}
			}
		}
	default:
		return fmt.Errorf("unsupported operator %q", e.Op)
	}
	return nil
}
func validateRuleSet(set RuleSet, compiled bool) error {
	if len(set.Rules) > 100 {
		return errors.New("at most 100 notification rules")
	}
	ids := map[string]bool{}
	for _, r := range set.Rules {
		if r.Revision < 1 || r.ID == "" || ids[r.ID] || len(r.Text) > 8000 || strings.TrimSpace(r.Text) == "" {
			return errors.New("rules require unique IDs and bounded nonempty text")
		}
		if len(r.Diagnostic) > 8000 || len(r.Alternatives) > 3 {
			return errors.New("rule explanation exceeds limits")
		}
		for _, a := range r.Alternatives {
			if strings.TrimSpace(a) == "" || len(a) > 8000 {
				return errors.New("invalid alternative rule")
			}
		}
		ids[r.ID] = true
		if r.Mode != "" && r.Mode != "compiled" && r.Mode != "llm" {
			return errors.New("unsupported rule execution mode")
		}
		if !compiled || !r.Enabled {
			continue
		}
		if r.Mode == "llm" {
			if len(r.Clauses) != 0 {
				return errors.New("LLM rules must not contain stale compiled clauses")
			}
			continue
		}
		if r.CompilerVersion != ruleCompilerVersion || len(r.Clauses) == 0 || len(r.Clauses) > 20 {
			return fmt.Errorf("rule %s is not compiled", r.ID)
		}
		for i, c := range r.Clauses {
			if c.ID != fmt.Sprintf("%s.%d", r.ID, i+1) {
				return errors.New("invalid clause ID")
			}
			categories := map[string]string{"suppress": "filtering", "group_project": "grouping", "route_review": "action", "route_implementation": "action", "route_recurring": "action", "route_investigation": "action", "format": "presentation"}
			if categories[c.Effect] != c.Category || c.Category == "" || !slices.Contains([]string{"hold", "continue"}, c.OnUnknown) || c.Priority < 0 || c.Priority > 1000 || len(c.Argument) > 2000 {
				return fmt.Errorf("invalid clause on %s", r.ID)
			}
			if c.Effect != "route_recurring" && c.Effect != "format" && c.Argument != "" {
				return errors.New("clause argument is unsupported for this effect")
			}
			if (c.Effect == "route_recurring" || c.Effect == "format") && strings.TrimSpace(c.Argument) == "" {
				return errors.New("recurring/presentation clauses require an argument")
			}
			if err := validateExpression(c.When, 0); err != nil {
				return fmt.Errorf("%s: %w", c.ID, err)
			}
		}
	}
	// Exact overlapping predicates with contradictory effects are rejected now;
	// semantic/runtime overlaps are detected by the evaluator before publication.
	for _, a := range set.Rules {
		if !a.Enabled {
			continue
		}
		for _, x := range a.Clauses {
			for _, b := range set.Rules {
				if !b.Enabled {
					continue
				}
				for _, y := range b.Clauses {
					if x.ID >= y.ID || x.Category != y.Category || x.Category == "filtering" || x.Priority != y.Priority {
						continue
					}
					j, _ := json.Marshal(x.When)
					k, _ := json.Marshal(y.When)
					if bytes.Equal(j, k) && (x.Effect != y.Effect || x.Argument != y.Argument) {
						return fmt.Errorf("conflicting clauses %s and %s at equal priority", x.ID, y.ID)
					}
				}
			}
		}
	}
	return nil
}

func scanRuleSet(row interface{ Scan(...any) error }) (RuleSet, error) {
	var set RuleSet
	var rules, diagnostics string
	err := row.Scan(&set.ID, &set.Revision, &set.Status, &rules, &diagnostics)
	if err != nil {
		return set, err
	}
	if err = json.Unmarshal([]byte(rules), &set.Rules); err != nil {
		return set, err
	}
	err = json.Unmarshal([]byte(diagnostics), &set.Diagnostics)
	return set, err
}

const ruleSetColumns = "id,revision,status,rules_json,diagnostics"

func (s *Service) RuleSets(ctx context.Context) ([]RuleSet, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE status='active' OR id IN (SELECT id FROM notification_rule_sets ORDER BY revision DESC LIMIT 30) ORDER BY revision DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuleSet{}
	for rows.Next() {
		set, err := scanRuleSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, set)
	}
	return out, rows.Err()
}
func (s *Service) activeRuleSet(ctx context.Context) (RuleSet, error) {
	set, err := scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=(SELECT active_set_id FROM notification_rule_state WHERE id=1)`))
	if errors.Is(err, sql.ErrNoRows) {
		return set, ErrNoActiveRules
	}
	return set, err
}
func (s *Service) batchRuleSet(ctx context.Context, batch string) (RuleSet, error) {
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=(SELECT rule_set_id FROM notification_batches WHERE id=?)`, batch))
}
func (s *Service) SaveRuleDraft(ctx context.Context, base int64, rules []NotificationRule) (RuleSet, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RuleSet{}, err
	}
	defer tx.Rollback()
	var latest int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(revision),0) FROM notification_rule_sets`).Scan(&latest); err != nil {
		return RuleSet{}, err
	}
	if latest != base {
		return RuleSet{}, ErrRulesChanged
	}
	previous := RuleSet{}
	if latest > 0 {
		previous, err = scanRuleSet(tx.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE revision=?`, latest))
		if err != nil {
			return RuleSet{}, err
		}
	}
	for i := range rules {
		r := &rules[i]
		if r.ID == "" {
			r.ID = "R-" + uuid.NewString()
		}
		old := NotificationRule{}
		for _, o := range previous.Rules {
			if o.ID == r.ID {
				old = o
				break
			}
		}
		requestedMode := r.Mode
		r.Diagnostic = ""
		r.Alternatives = nil
		r.Mode = ""
		r.Revision = old.Revision + 1
		r.Clauses = nil
		r.CompilerVersion = ""
		if requestedMode == "llm" {
			r.Mode = "llm"
			if old.Mode == "llm" && old.Text == r.Text {
				if old.Enabled == r.Enabled {
					r.Revision = old.Revision
				}
				r.Diagnostic = old.Diagnostic
				r.Alternatives = old.Alternatives
			}
		} else if old.Text == r.Text && old.Mode != "llm" && old.CompilerVersion == ruleCompilerVersion {
			r.Clauses = old.Clauses
			r.CompilerVersion = old.CompilerVersion
			if r.Enabled == old.Enabled {
				r.Revision = old.Revision
			}
		}
	}
	set := RuleSet{ID: uuid.NewString(), Revision: latest + 1, Status: "draft", Rules: rules, Diagnostics: []string{}}
	if err = validateRuleSet(set, false); err != nil {
		return set, err
	}
	raw, _ := json.Marshal(rules)
	now := time.Now().UnixMilli()
	_, err = tx.ExecContext(ctx, `INSERT INTO notification_rule_sets(id,revision,status,rules_json,created_at,updated_at) VALUES(?,?,'draft',?,?,?)`, set.ID, set.Revision, string(raw), now, now)
	if err != nil {
		return set, err
	}
	return set, tx.Commit()
}
func (s *Service) ActivateRules(ctx context.Context, id string) (RuleSet, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RuleSet{}, err
	}
	defer tx.Rollback()
	set, err := scanRuleSet(tx.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, id))
	if err != nil {
		return set, err
	}
	if set.Status != "ready" {
		return set, errors.New("only a ready rule set can be activated")
	}
	if err = validateRuleSet(set, true); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_rule_sets SET status='retired' WHERE status='active'`); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_rule_sets SET status='active' WHERE id=?`, id); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_rule_state SET active_set_id=? WHERE id=1`, id); err != nil {
		return set, err
	}
	// Pending cards from older rule sets must not remain executable dead ends.
	// Requeue their sources and retire the cards atomically with activation.
	if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET raw_status='pending',held=0,raw_revision=raw_revision+1 WHERE id IN (SELECT links.event_id FROM notification_question_events links JOIN inbox_questions q ON q.id=links.question_id JOIN notification_card_plans p ON p.question_id=q.id WHERE q.status='pending' AND p.rule_set_id!=? UNION SELECT q.notification_id FROM inbox_questions q JOIN notification_card_plans p ON p.question_id=q.id WHERE q.status='pending' AND p.rule_set_id!=?)`, id, id); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET status='unanswered',reason='notification_rules_changed',revision=revision+1,updated_at=? WHERE status='pending' AND id IN (SELECT question_id FROM notification_card_plans WHERE rule_set_id!=?)`, time.Now().UnixMilli(), id); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET held=0,raw_revision=raw_revision+1 WHERE raw_status='pending' AND held=1`); err != nil {
		return set, err
	}
	set.Status = "active"
	return set, tx.Commit()
}
func (s *Service) checkRuleSet(ctx context.Context, set RuleSet) error {
	var id sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT active_set_id FROM notification_rule_state WHERE id=1`).Scan(&id); err != nil {
		return err
	}
	if !id.Valid || id.String != set.ID {
		return ErrRulesChanged
	}
	return nil
}

func evalExpression(e RuleExpression, facts map[string]RuleFact, semantic map[string]string) string {
	switch e.Op {
	case "always":
		return "true"
	case "semantic":
		if e.Text == sessionRoutingCondition {
			return evalExpression(RuleExpression{Op: "eq", Fact: "workflow.needs_session", Value: true}, facts, nil)
		}
		if v, ok := semantic[e.Text]; ok {
			return v
		}
		return "unknown"
	case "not":
		switch evalExpression(e.Args[0], facts, semantic) {
		case "true":
			return "false"
		case "false":
			return "true"
		default:
			return "unknown"
		}
	case "all", "any":
		unknown := false
		for _, a := range e.Args {
			v := evalExpression(a, facts, semantic)
			if e.Op == "all" && v == "false" {
				return "false"
			}
			if e.Op == "any" && v == "true" {
				return "true"
			}
			unknown = unknown || v == "unknown"
		}
		if unknown {
			return "unknown"
		}
		if e.Op == "all" {
			return "true"
		}
		return "false"
	default:
		fact := facts[e.Fact]
		if !fact.Known {
			return "unknown"
		}
		j, _ := json.Marshal(fact.Value)
		values := []any{e.Value}
		if e.Op == "in" {
			values, _ = e.Value.([]any)
		}
		for _, v := range values {
			k, _ := json.Marshal(v)
			if slices.Contains([]string{"repository", "actor", "pr.author"}, e.Fact) {
				a, ok := fact.Value.(string)
				b, bok := v.(string)
				if ok && bok && strings.EqualFold(a, b) {
					return "true"
				}
			}
			if bytes.Equal(j, k) {
				return "true"
			}
		}
		return "false"
	}
}
func expressionActivity(e RuleExpression) bool {
	if e.Op == "semantic" || slices.Contains([]string{"actor", "activity.kind", "activity.own", "review.state"}, e.Fact) {
		return true
	}
	for _, a := range e.Args {
		if expressionActivity(a) {
			return true
		}
	}
	return false
}
func subjectRuleFacts(subject *triageSubject, finding *intentFinding) map[string]RuleFact {
	e := subject.fresh
	facts := map[string]RuleFact{}
	observed := ""
	if !e.EvidenceObservedAt.IsZero() {
		observed = e.EvidenceObservedAt.Format(time.RFC3339Nano)
	}
	known := func(key string, v any, ids ...string) {
		facts[key] = RuleFact{Known: true, Value: v, Evidence: ids, ObservedAt: observed}
	}
	known("repository", strings.ToLower(e.Repository))
	facts["workflow.needs_session"] = sessionRoutingFact(e)
	if finding != nil && finding.Intent == "review_request" {
		if e.ReviewSessionRouting != nil {
			facts["workflow.needs_session"] = *e.ReviewSessionRouting
		} else {
			facts["workflow.needs_session"] = (&Service{}).routingForRole(e, "review")
		}
	}
	if subject.projectID > 0 {
		known("project_id", float64(subject.projectID))
	}
	known("subject.type", e.SubjectType)
	known("subject.url", e.URL)
	known("subject.title", e.Title)
	if mention := mentionStatus(subject.events, e, subject.viewer); mention != nil {
		known("viewer.mentioned", *mention)
	} else {
		facts["viewer.mentioned"] = RuleFact{Reason: "mention coverage incomplete or recency unknown: " + strings.Join(e.ContextWarnings, "; ")}
	}
	if f := e.PRFacts; f != nil && f.Available && !f.Stale {
		known("pr.state", f.State)
		if f.Draft != nil {
			known("pr.draft", *f.Draft)
		}
		if f.ViewerEverApproved != nil {
			known("viewer.approved", *f.ViewerEverApproved)
		}
		if strings.EqualFold(f.Viewer, subject.viewer) && subject.viewer != "" {
			known("viewer.review_requested", slices.ContainsFunc(f.RequestedReviewers, func(v string) bool { return strings.EqualFold(v, subject.viewer) }))
		}
	}
	for _, item := range e.Evidence {
		if item.URL == e.URL && item.Actor != "" {
			known("pr.author", item.Actor, item.ID)
		}
	}
	if finding != nil {
		known("activity.kind", finding.Intent)
		for _, item := range e.Evidence {
			if item.ID == finding.EvidenceID {
				if item.Actor != "" {
					known("actor", item.Actor, item.ID)
				}
				if item.Actor != "" && !strings.EqualFold(item.Actor, subject.viewer) {
					known("activity.own", false, item.ID)
				} else if item.Current && item.Actor != "" {
					known("activity.own", true, item.ID)
				}
				if item.ReviewState != "" {
					known("review.state", item.ReviewState, item.ID)
				}
			}
		}
	}
	return facts
}
func evaluateRuleSet(set RuleSet, facts map[string]RuleFact, semantic map[string]string, preflight bool) (string, []RuleEvaluation, []RuleClause) {
	outcome := "admitted"
	evaluations := []RuleEvaluation{}
	matched := []RuleClause{}
	for _, rule := range set.Rules {
		if !rule.Enabled {
			continue
		}
		for _, c := range rule.Clauses {
			if preflight && (c.Category != "filtering" || expressionActivity(c.When)) {
				continue
			}
			result := evalExpression(c.When, facts, semantic)
			evaluation := RuleEvaluation{RuleID: rule.ID, ClauseID: c.ID, Result: result, Effect: c.Effect, OnUnknown: c.OnUnknown, Evidence: []string{}}
			for _, f := range facts {
				evaluation.Evidence = append(evaluation.Evidence, f.Evidence...)
			}
			if result == "unknown" {
				evaluation.Reason = "required fact or semantic evidence is unavailable"
				if outcome != "suppressed" && c.OnUnknown == "hold" && c.Category != "action" && c.Category != "grouping" {
					outcome = "held"
				}
			}
			if result == "true" {
				matched = append(matched, c)
				if c.Effect == "suppress" {
					outcome = "suppressed"
				}
			}
			evaluations = append(evaluations, evaluation)
		}
	}
	if preflight {
		return outcome, evaluations, matched
	}
	// Only top-priority applicable clauses compete. Lower-priority conflicts
	// and unknowns cannot overrule a known, more specific routing decision.
	for _, category := range []string{"grouping", "action"} {
		var winner *RuleClause
		for i := range matched {
			c := &matched[i]
			if c.Category == category && (winner == nil || c.Priority > winner.Priority) {
				winner = c
			}
		}
		for _, rule := range set.Rules {
			if !rule.Enabled {
				continue
			}
			for _, c := range rule.Clauses {
				if c.Category != category {
					continue
				}
				v := evalExpression(c.When, facts, semantic)
				conflict := v == "unknown" && c.OnUnknown == "hold" && (winner == nil || c.Priority >= winner.Priority)
				conflict = conflict || (v == "true" && winner != nil && c.Priority == winner.Priority && (c.Effect != winner.Effect || c.Argument != winner.Argument))
				if conflict && outcome != "suppressed" {
					outcome = "held"
				}
			}
		}
	}

	return outcome, evaluations, matched
}

func mustJSON(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func (s *Service) RuleSet(ctx context.Context, id string) (RuleSet, error) {
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, id))
}

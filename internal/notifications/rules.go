package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"kamacu/internal/inbox"
	"strings"
	"time"
)

var ErrRulesChanged = errors.New("notification settings or rules changed; refresh and try again")
var ErrNoActiveRules = errors.New("notification rules are unavailable")

type RuleSet struct {
	ID       string             `json:"id"`
	Revision int64              `json:"revision"`
	Status   string             `json:"status"`
	Rules    []NotificationRule `json:"rules"`
}
type NotificationRule struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Text     string `json:"text"`
	Enabled  bool   `json:"enabled"`
}
type RuleFact struct {
	ObservedAt string   `json:"observed_at,omitempty"`
	Known      bool     `json:"known"`
	Value      any      `json:"value"`
	Evidence   []string `json:"evidence"`
	Reason     string   `json:"reason,omitempty"`
}
type RuleEvaluation struct {
	Blocking  bool     `json:"blocking,omitempty"`
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
	BatchID         string              `json:"batch_id"`
	Reason          string              `json:"reason"`
	RuleIDs         []string            `json:"rule_ids"`
	EvidenceRefs    []string            `json:"evidence_refs"`
	Snapshot        string              `json:"-"`
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
	Bindings      []inbox.TaskLink `json:"bindings"`
	WorkKind      string           `json:"work_kind,omitempty"`
	SessionMode   string           `json:"session_mode,omitempty"`
	SessionID     string           `json:"session_id,omitempty"`
	OwnerEvidence string           `json:"owner_evidence,omitempty"`
	Targets       []ReviewTarget   `json:"targets"`
	Presentation  []string         `json:"presentation"`
	Evidence      []ActionEvidence `json:"evidence"`
	RuleSetID     string           `json:"rule_set_id"`
	Activities    []string         `json:"activities"`
	Workflow      string           `json:"workflow"`
	ProjectID     int64            `json:"project_id"`
	TaskID        int64            `json:"task_id"`
	PRNumber      int64            `json:"pr_number"`
	TaskName      string           `json:"task_name"`
	RuleIDs       []string         `json:"rule_ids"`
}

func strictRuleJSON(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid trailing JSON")
	}
	return nil
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

const ruleSetColumns = "id,revision,status,rules_json"

func scanRuleSet(row interface{ Scan(...any) error }) (RuleSet, error) {
	var s RuleSet
	var raw string
	err := row.Scan(&s.ID, &s.Revision, &s.Status, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &s.Rules)
	}
	if s.Rules == nil {
		s.Rules = []NotificationRule{}
	}
	return s, err
}
func (s *Service) RuleSets(ctx context.Context) ([]RuleSet, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets ORDER BY revision DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuleSet{}
	for rows.Next() {
		r, err := scanRuleSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) RuleSet(ctx context.Context, id string) (RuleSet, error) {
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, id))
}
func (s *Service) activeRuleSet(ctx context.Context) (RuleSet, error) {
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=(SELECT active_set_id FROM notification_rule_state WHERE id=1)`))
}
func (s *Service) batchRuleSet(ctx context.Context, id string) (RuleSet, error) {
	return scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=(SELECT rule_set_id FROM notification_batches WHERE id=?)`, id))
}
func (s *Service) checkRuleSet(ctx context.Context, set RuleSet) error {
	var id string
	if err := s.DB.QueryRowContext(ctx, `SELECT active_set_id FROM notification_rule_state WHERE id=1`).Scan(&id); err != nil {
		return err
	}
	if id != set.ID {
		return ErrRulesChanged
	}
	return nil
}
func (s *Service) SaveRules(ctx context.Context, base int64, rules []NotificationRule) (RuleSet, error) {
	if len(rules) > 100 {
		return RuleSet{}, errors.New("at most 100 notification rules")
	}
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
	seen := map[string]bool{}
	for i := range rules {
		r := &rules[i]
		if r.ID == "" {
			r.ID = "R-" + uuid.NewString()
		}
		if seen[r.ID] || len(r.ID) > 100 || strings.TrimSpace(r.Text) == "" || len(r.Text) > 8000 {
			return RuleSet{}, errors.New("rules require unique IDs and nonempty text up to 8000 bytes")
		}
		seen[r.ID] = true
		r.Revision = latest + 1
	}
	if rules == nil {
		rules = []NotificationRule{}
	}
	set := RuleSet{ID: uuid.NewString(), Revision: latest + 1, Status: "active", Rules: rules}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `UPDATE notification_rule_sets SET status='retired' WHERE status='active'`); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notification_rule_sets(id,revision,status,rules_json,created_at,updated_at) VALUES(?,?,'active',?,?,?)`, set.ID, set.Revision, mustJSON(rules), now, now); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_rule_state SET active_set_id=? WHERE id=1`, set.ID); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET raw_status='pending',raw_revision=raw_revision+1 WHERE id IN (SELECT e.event_id FROM notification_question_events e JOIN inbox_questions q ON q.id=e.question_id WHERE q.origin='notification' AND (q.status='pending' OR q.delivery_status='queued') UNION SELECT notification_id FROM inbox_questions WHERE origin='notification' AND (status='pending' OR delivery_status='queued') AND notification_id IS NOT NULL)`); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET status='unanswered',reason='notification_rules_changed',revision=revision+1,updated_at=? WHERE origin='notification' AND status='pending'`, now); err != nil {
		return set, err
	}
	// An answered card may still be queued for execution. Its old plan cannot
	// run under the new rules; return its inputs for fresh consideration rather
	// than silently leaving them archived. Running executions keep their
	// existing uncertainty safeguards and must never be automatically resent.
	if _, err = tx.ExecContext(ctx, `UPDATE notification_runs SET state='failed',updated_at=? WHERE state='queued'`, now); err != nil {
		return set, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET delivery_status='failed',execution_result='Notification rules changed before execution. Process pending notifications again.',updated_at=? WHERE origin='notification' AND delivery_status='queued'`, now); err != nil {
		return set, err
	}
	return set, tx.Commit()
}

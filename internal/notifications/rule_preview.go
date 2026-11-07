package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type rulePreviewKey struct{}
type RulePreview struct {
	ID        string          `json:"id"`
	RuleSetID string          `json:"rule_set_id"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
}
type RulePreviewResult struct {
	Decisions []ActivityDecision `json:"decisions"`
	Cards     []BatchAction      `json:"cards"`
}

func (s *Service) QueueRulePreview(ctx context.Context, setID string, ids []int64) (RulePreview, error) {
	set, err := scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, setID))
	if err != nil {
		return RulePreview{}, err
	}
	if set.Status != "ready" && set.Status != "active" {
		return RulePreview{}, errors.New("compile rules before previewing")
	}
	if err = validateRuleSet(set, true); err != nil {
		return RulePreview{}, err
	}
	if len(ids) == 0 {
		rows, err := s.DB.QueryContext(ctx, `SELECT id FROM notification_events ORDER BY id DESC LIMIT 20`)
		if err != nil {
			return RulePreview{}, err
		}
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return RulePreview{}, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return RulePreview{}, err
		}
	}
	if len(ids) == 0 || len(ids) > 50 {
		return RulePreview{}, errors.New("preview requires 1–50 notification IDs")
	}
	p := RulePreview{ID: uuid.NewString(), RuleSetID: setID, Status: "queued", Result: json.RawMessage(`{}`)}
	now := time.Now().UnixMilli()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO notification_rule_previews(id,rule_set_id,event_ids,status,created_at,updated_at) VALUES(?,?,?,'queued',?,?)`, p.ID, setID, mustJSON(ids), now, now)
	return p, err
}
func (s *Service) RulePreview(ctx context.Context, id string) (RulePreview, error) {
	var p RulePreview
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT id,rule_set_id,status,result,error FROM notification_rule_previews WHERE id=?`, id).Scan(&p.ID, &p.RuleSetID, &p.Status, &raw, &p.Error)
	p.Result = json.RawMessage(raw)
	return p, err
}
func (s *Service) processRulePreview(ctx context.Context) error {
	var id, setID, raw string
	err := s.DB.QueryRowContext(ctx, `SELECT id,rule_set_id,event_ids FROM notification_rule_previews WHERE status='queued' ORDER BY created_at LIMIT 1`).Scan(&id, &setID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_rule_previews SET status='running' WHERE id=? AND status='queued'`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return nil
	}
	set, runErr := scanRuleSet(s.DB.QueryRowContext(ctx, `SELECT `+ruleSetColumns+` FROM notification_rule_sets WHERE id=?`, setID))
	var result RulePreviewResult
	if runErr == nil {
		var ids []int64
		runErr = json.Unmarshal([]byte(raw), &ids)
		if runErr == nil {
			result, runErr = s.previewRules(ctx, set, ids)
		}
	}
	state, detail := "completed", ""
	if runErr != nil {
		state, detail = "failed", runErr.Error()
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = s.DB.ExecContext(recordCtx, `UPDATE notification_rule_previews SET status=?,result=?,error=?,updated_at=? WHERE id=?`, state, mustJSON(result), detail, time.Now().UnixMilli(), id)
	return err
}
func (s *Service) previewRules(ctx context.Context, set RuleSet, ids []int64) (RulePreviewResult, error) {
	result := RulePreviewResult{Decisions: []ActivityDecision{}, Cards: []BatchAction{}}
	events := []RawNotification{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return result, errors.New("invalid or duplicate preview ID")
		}
		seen[id] = true
		var e RawNotification
		var payload string
		if err := s.DB.QueryRowContext(ctx, `SELECT e.id,e.source_id,s.name,e.payload FROM notification_events e JOIN notification_sources s ON s.id=e.source_id WHERE e.id=?`, id).Scan(&e.ID, &e.SourceID, &e.SourceName, &payload); err != nil {
			return result, err
		}
		if err := json.Unmarshal([]byte(payload), &e.Event); err != nil {
			return result, err
		}
		events = append(events, e)
	}
	groups, err := s.notificationProjectGroups(ctx, events)
	if err != nil {
		return result, err
	}
	ctx = withNotificationRun(ctx, "preview:"+set.ID)
	ctx = context.WithValue(ctx, rulePreviewKey{}, true)
	plans := make([]notificationPlan, len(groups))
	cards := make([][]BatchAction, len(groups))
	err = parallelNotifications(ctx, len(groups), notificationProjectConcurrency, func(ctx context.Context, i int) error {
		if err := s.prepareRuleGroup(ctx, groups[i], set, &plans[i]); err != nil {
			return err
		}
		for _, g := range plans[i].groups {
			if err := s.evaluateActivities(ctx, g.subjects, set); err != nil {
				return err
			}
			if err := s.prepareActivityPlans(ctx, g.subjects, set); err != nil {
				return err
			}
			d, err := s.renderTriage(ctx, g.subjects, nil, set)
			if err != nil {
				return err
			}
			if err = s.draftProjectCards(ctx, &d, g.subjects, set, ""); err != nil {
				return err
			}
			cards[i] = append(cards[i], d.Actions...)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("preview: %w", err)
	}
	for i, p := range plans {
		result.Cards = append(result.Cards, cards[i]...)
		for _, g := range p.groups {
			for _, subject := range g.subjects {
				result.Decisions = append(result.Decisions, subject.decisions...)
			}
		}
	}
	clarifications, err := s.clarificationActions(ctx, plans)
	if err != nil {
		return result, err
	}
	for _, a := range clarifications {
		var plan ActionPlan
		if err = strictRuleJSON([]byte(a.Plan), &plan); err != nil {
			return result, err
		}
		options := []string{}
		for _, o := range a.Request.Options {
			options = append(options, o.Label)
		}
		result.Cards = append(result.Cards, BatchAction{Plan: plan, EventIDs: a.EventIDs, Summary: a.Request.Summary, Context: a.Request.Context, Question: a.Request.Question, Options: options})
	}
	return result, nil
}

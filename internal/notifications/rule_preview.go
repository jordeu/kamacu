package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"kamacu/internal/diagnostics"
	"time"
)

type RulePreview struct {
	ID        string          `json:"id"`
	RuleSetID string          `json:"rule_set_id"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
}

func (s *Service) QueueRulePreview(ctx context.Context, setID string, ids []int64) (RulePreview, error) {
	if _, err := s.RuleSet(ctx, setID); err != nil {
		return RulePreview{}, err
	}
	if _, err := s.notificationProfile(ctx); err != nil {
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
	if len(ids) == 0 || len(ids) > 100 {
		return RulePreview{}, errors.New("preview requires 1–100 notification IDs")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return RulePreview{}, errors.New("invalid preview notification IDs")
		}
		seen[id] = true
	}
	p := RulePreview{ID: uuid.NewString(), RuleSetID: setID, Status: "queued", Result: json.RawMessage(`{}`)}
	now := time.Now().UnixMilli()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO notification_rule_previews(id,rule_set_id,event_ids,status,created_at,updated_at) VALUES(?,?,?,'queued',?,?)`, p.ID, setID, mustJSON(ids), now, now)
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
	var id, setID, idsJSON string
	err := s.DB.QueryRowContext(ctx, `SELECT id,rule_set_id,event_ids FROM notification_rule_previews WHERE status='queued' ORDER BY created_at LIMIT 1`).Scan(&id, &setID, &idsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE notification_rule_previews SET status='running',updated_at=? WHERE id=?`, time.Now().UnixMilli(), id); err != nil {
		return err
	}
	ctx = diagnostics.WithGroup(ctx, diagnostics.Group{Kind: "preview", ID: id, StartedAt: time.Now()})
	ctx = withNotificationRun(ctx, "preview:"+id)
	result, runErr := func() (AgentTurn, error) {
		run, cancel, config, err := s.modelContext(ctx)
		if err != nil {
			return AgentTurn{}, err
		}
		defer cancel()
		rules, err := s.RuleSet(run, setID)
		if err != nil {
			return AgentTurn{}, err
		}
		var ids []int64
		if err = json.Unmarshal([]byte(idsJSON), &ids); err != nil {
			return AgentTurn{}, err
		}
		events := []RawNotification{}
		for _, id := range ids {
			var e RawNotification
			var payload string
			if err = s.DB.QueryRowContext(run, `SELECT e.id,e.source_id,s.name,e.payload FROM notification_events e JOIN notification_sources s ON s.id=e.source_id WHERE e.id=?`, id).Scan(&e.ID, &e.SourceID, &e.SourceName, &payload); err != nil {
				return AgentTurn{}, err
			}
			if err = json.Unmarshal([]byte(payload), &e.Event); err != nil {
				return AgentTurn{}, err
			}
			events = append(events, e)
		}
		snapshot, err := s.agentSnapshot(context.WithValue(run, activityReplayKey{}, true), events, rules)
		if err != nil {
			return AgentTurn{}, err
		}
		turn, _, err := s.runNotificationAgent(run, snapshot, config)
		if err != nil {
			return turn, err
		}
		_, _, _, err = s.agentActions(run, snapshot, turn)
		return turn, err
	}()
	status, message := "completed", ""
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
	}
	record, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = s.DB.ExecContext(record, `UPDATE notification_rule_previews SET status=?,result=?,error=?,updated_at=? WHERE id=?`, status, mustJSON(result), message, time.Now().UnixMilli(), id)
	return err
}

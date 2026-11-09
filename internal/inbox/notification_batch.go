package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type NotificationBatchAction struct {
	Links      []TaskLink
	EventIDs   []int64
	SourceName string
	SourceURL  string
	Request    Create
	Plan       string
}
type NotificationQuestionRef struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type NotificationDecision struct {
	Reason       string
	RuleIDs      string
	EvidenceRefs string
	Snapshot     string
	Semantic     string
	ActivityID   string
	EventIDs     string
	Outcome      string
	Evaluations  string
	Facts        string
}

// ApplyNotificationBatch publishes one independent card per action, records its
// full provenance, supersedes only unchanged questions, and archives the exact
// raw snapshot in the same transaction as marking the run complete.
func (s *Service) ApplyNotificationBatch(ctx context.Context, batchID string, actions []NotificationBatchAction, supersede []NotificationQuestionRef, decisions ...NotificationDecision) error {
	for i := range actions {
		actions[i].Request.Key = fmt.Sprintf("batch:%s:%d", batchID, i)
		if err := validate(actions[i].Request); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM notification_batches WHERE id=?`, batchID).Scan(&status); err != nil {
		return err
	}
	if status == "completed" {
		return nil
	}
	if status != "running" {
		return ErrConflict
	}
	var stale int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_batch_events b JOIN notification_events e ON e.id=b.event_id WHERE b.batch_id=? AND (e.raw_status!='pending' OR e.raw_revision!=b.revision)`, batchID).Scan(&stale); err != nil {
		return err
	}
	if stale > 0 {
		return ErrConflict
	}
	if err = checkNotificationBatchQuestions(ctx, tx, batchID); err != nil {
		return err
	}
	var captured, active sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT rule_set_id,(SELECT active_set_id FROM notification_rule_state WHERE id=1) FROM notification_batches WHERE id=?`, batchID).Scan(&captured, &active); err != nil {
		return err
	}
	if !captured.Valid || !active.Valid || captured.String != active.String {
		return ErrConflict
	}
	var configJSON string
	if err = tx.QueryRowContext(ctx, `SELECT processor_config FROM notification_batches WHERE id=?`, batchID).Scan(&configJSON); err != nil {
		return err
	}
	if configJSON != "{}" {
		var frozen struct {
			Settings struct {
				Revision  int64  `json:"revision"`
				ProfileID *int64 `json:"profile_id"`
			} `json:"settings"`
			Profile struct {
				ID       int64 `json:"id"`
				Revision int64 `json:"revision"`
			} `json:"profile"`
		}
		if err = json.Unmarshal([]byte(configJSON), &frozen); err != nil {
			return err
		}
		var matches int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_settings c JOIN monitor_profiles p ON p.id=c.profile_id WHERE c.id=1 AND c.revision=? AND p.id=? AND p.revision=?`, frozen.Settings.Revision, frozen.Profile.ID, frozen.Profile.Revision).Scan(&matches); err != nil {
			return err
		}
		if matches != 1 {
			return ErrConflict
		}
	}
	var disabled int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_batch_events b JOIN notification_events e ON e.id=b.event_id JOIN notification_sources s ON s.id=e.source_id WHERE b.batch_id=? AND (s.enabled=0 OR s.retired=1 OR (s.provider='github' AND COALESCE((SELECT value FROM settings WHERE key='github_integration'),'on')!='on'))`, batchID).Scan(&disabled); err != nil {
		return err
	}
	if disabled > 0 {
		return ErrConflict
	}
	for _, d := range decisions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO notification_rule_decisions(batch_id,activity_id,event_ids,outcome,evaluations,facts,semantic,explanation,rule_ids,evidence_refs) VALUES(?,?,?,?,?,?,?,?,?,?)`, batchID, d.ActivityID, d.EventIDs, d.Outcome, d.Evaluations, d.Facts, d.Semantic, d.Reason, jsonArray(d.RuleIDs), jsonArray(d.EvidenceRefs)); err != nil {
			return err
		}
		if d.Snapshot != "" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_decision_snapshots(batch_id,activity_id,evidence) VALUES(?,?,?)`, batchID, d.ActivityID, d.Snapshot); err != nil {
				return err
			}
		}
	}
	now := s.now().UnixMilli()
	for _, ref := range supersede {
		var revision int64
		var qstatus, origin string
		if err = tx.QueryRowContext(ctx, `SELECT q.revision,q.status,q.origin FROM notification_batch_questions b JOIN inbox_questions q ON q.id=b.question_id WHERE b.batch_id=? AND q.id=? AND b.revision=?`, batchID, ref.ID, ref.Revision).Scan(&revision, &qstatus, &origin); err != nil {
			return err
		}
		if qstatus != "pending" {
			continue
		}
		if revision != ref.Revision || origin != "notification" {
			return ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET status='unanswered',reason='notification_superseded',revision=revision+1,updated_at=? WHERE id=?`, now, ref.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_events(question_id,kind,session_id,created_at) VALUES(?,'notification_superseded','',?)`, ref.ID, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET raw_status='archived',state='ignored',raw_revision=raw_revision+1,last_error='' WHERE id IN (SELECT event_id FROM notification_batch_events WHERE batch_id=?)`, batchID); err != nil {
		return err
	}

	for _, a := range actions {
		if len(a.EventIDs) == 0 {
			return ErrInvalid
		}
		id := uuid.NewString()
		c := a.Request
		if c.Options == nil {
			c.Options = []Option{}
		}
		opts, _ := json.Marshal(c.Options)
		if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,abstract,summary,context,question,options,status,kind,origin,delivery_mode,created_at,updated_at,heartbeat_at,notification_id,source_name,source_url,notification_group) VALUES(?,NULL,'',?,?,?,?,?,?,?,'pending','question','notification','notification',?,?,?,?,?,?,?)`, id, "notification:"+id, c.Key, c.Abstract, c.Summary, c.Context, c.Question, string(opts), now, now, now, a.EventIDs[0], a.SourceName, a.SourceURL, id); err != nil {
			return err
		}
		if a.Plan == "" {
			return ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO notification_card_plans(question_id,rule_set_id,plan) VALUES(?,?,?)`, id, captured.String, a.Plan); err != nil {
			return err
		}
		for _, link := range a.Links {
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_task_links(question_id,task_id,role,source,evidence) VALUES(?,?,?,?,?) ON CONFLICT(question_id,task_id) DO NOTHING`, id, link.TaskID, link.Role, link.Source, link.Evidence); err != nil {
				return err
			}
		}
		for _, eventID := range a.EventIDs {
			var member int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_batch_events WHERE batch_id=? AND event_id=?`, batchID, eventID).Scan(&member); err != nil {
				return err
			}
			if member != 1 {
				return ErrInvalid
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_question_events(question_id,event_id) VALUES(?,?) ON CONFLICT DO NOTHING`, id, eventID); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET state='published' WHERE id=?`, eventID); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_events(question_id,kind,session_id,created_at) VALUES(?,'notification_batch_created','',?)`, id, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE notification_batches SET status='completed',action_count=?,updated_at=?,error='' WHERE id=?`, len(actions), now, batchID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.signal()
	return nil
}

// RelatedNotificationIDs includes every notification behind a grouped action,
// falling back to the originating event for pre-batch inbox history.
func (s *Service) RelatedNotificationIDs(ctx context.Context, questionID string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id FROM notification_question_events WHERE question_id=? UNION SELECT notification_id FROM inbox_questions WHERE id=? AND notification_id IS NOT NULL ORDER BY 1`, questionID, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (s *Service) PublishNotificationFollowup(ctx context.Context, questionID string, c Create) (Question, error) {
	c.Key = uuid.NewString()
	if err := validate(c); err != nil {
		return Question{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.get(ctx, questionID)
	if err != nil {
		return Question{}, err
	}
	if previous.Origin != "notification" {
		return Question{}, ErrInvalid
	}
	return s.publishFollowupLocked(ctx, previous, c)
}
func (s *Service) publishFollowupLocked(ctx context.Context, previous Question, c Create, attentionRun ...string) (Question, error) {
	var group string
	if err := s.db.QueryRowContext(ctx, `SELECT notification_group FROM inbox_questions WHERE id=?`, previous.ID).Scan(&group); err != nil {
		return Question{}, err
	}
	attentionID := ""
	if len(attentionRun) > 0 {
		attentionID = attentionRun[0]
		group = "action-attention:" + attentionID
	}
	if group == "" {
		return s.publishNotificationLocked(ctx, previous.NotificationID, previous.SourceName, previous.SourceURL, c, false)
	}
	q, err := scan(s.db.QueryRowContext(ctx, selectQuestion+`WHERE q.notification_group=? AND q.status='pending'`, group))
	if err == nil {
		if attentionID != "" {
			if _, err = s.db.ExecContext(ctx, `UPDATE notification_runs SET attention_pending=0,attention_question_id=? WHERE id=?`, q.ID, attentionID); err != nil {
				return Question{}, err
			}
		}
		return q, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Question{}, err
	}
	id := uuid.NewString()
	now := s.now().UnixMilli()
	if c.Options == nil {
		c.Options = []Option{}
	}
	opts, _ := json.Marshal(c.Options)
	before := func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO notification_question_events(question_id,event_id) SELECT ?,event_id FROM notification_question_events WHERE question_id=?`, id, previous.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO notification_card_plans(question_id,rule_set_id,plan) SELECT ?,rule_set_id,COALESCE(NULLIF((SELECT plan FROM notification_runs WHERE id=p.question_id),'{}'),p.plan) FROM notification_card_plans p WHERE question_id=?`, id, previous.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO notification_task_links(question_id,task_id,role,source,evidence) SELECT ?,task_id,role,source,evidence FROM notification_task_links WHERE question_id=?`, id, previous.ID)
		return err
	}
	// Provenance references the newly inserted question, so copy it after insert.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Question{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,abstract,summary,context,question,options,status,kind,origin,delivery_mode,created_at,updated_at,heartbeat_at,notification_id,source_name,source_url,notification_group) VALUES(?,NULL,'',?,?,?,?,?,?,?,'pending','question','notification','notification',?,?,?,?,?,?,?)`, id, "notification:"+id, id, c.Abstract, c.Summary, c.Context, c.Question, string(opts), now, now, now, previous.NotificationID, previous.SourceName, previous.SourceURL, group); err != nil {
		return Question{}, err
	}
	if c.Informational {
		if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET reason='operational_failure' WHERE id=?`, id); err != nil {
			return Question{}, err
		}
	}

	if err = before(tx); err != nil {
		return Question{}, err
	}
	if attentionID != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE notification_runs SET attention_pending=0,attention_question_id=? WHERE id=?`, id, attentionID); err != nil {
			return Question{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_events(question_id,kind,session_id,created_at) VALUES(?,'notification_followup','',?)`, id, now); err != nil {
		return Question{}, err
	}
	if err = tx.Commit(); err != nil {
		return Question{}, err
	}
	s.signal()
	return s.get(ctx, id)
}

func jsonArray(raw string) string {
	if raw == "" {
		return "[]"
	}
	return raw
}

package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	Snapshot    string
	Semantic    string
	ActivityID  string
	EventIDs    string
	Outcome     string
	Evaluations string
	Facts       string
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
	for _, d := range decisions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO notification_rule_decisions(batch_id,activity_id,event_ids,outcome,evaluations,facts,semantic) VALUES(?,?,?,?,?,?,?)`, batchID, d.ActivityID, d.EventIDs, d.Outcome, d.Evaluations, d.Facts, d.Semantic); err != nil {
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
		merged := false
		var incoming struct {
			Key string `json:"clarification_key"`
		}
		if err = json.Unmarshal([]byte(a.Plan), &incoming); err != nil {
			return err
		}
		if incoming.Key != "" {
			var existing, previous, previousContext string
			lookupErr := tx.QueryRowContext(ctx, `SELECT q.id,p.plan,q.context FROM inbox_questions q JOIN notification_card_plans p ON p.question_id=q.id JOIN notification_batch_questions b ON b.question_id=q.id AND b.batch_id=? WHERE q.status='pending' AND `+nativeClarificationOption+` AND p.rule_set_id=? AND json_extract(p.plan,'$.clarification_key')=? ORDER BY q.created_at LIMIT 1`, batchID, captured.String, incoming.Key).Scan(&existing, &previous, &previousContext)
			if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
				return lookupErr
			}
			if lookupErr == nil {
				id = existing
				merged = true
				a.Plan, err = mergeClarificationPlans(previous, a.Plan)
				if err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, `UPDATE notification_card_plans SET plan=? WHERE question_id=?`, a.Plan, id); err != nil {
					return err
				}
				var mergedPlan struct {
					Activities []string `json:"activities"`
				}
				if err = json.Unmarshal([]byte(a.Plan), &mergedPlan); err != nil {
					return err
				}
				summary := fmt.Sprintf("%d updates need a filtering or routing decision. No task has been started.", len(mergedPlan.Activities))
				if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET summary=?,context=?,revision=revision+1,updated_at=? WHERE id=?`, summary, mergeClarificationContext(previousContext, a.Request.Context), now, id); err != nil {
					return err
				}
			}
		}
		c := a.Request
		if c.Options == nil {
			c.Options = []Option{}
		}
		opts, _ := json.Marshal(c.Options)
		if !merged {
			if _, err = tx.ExecContext(ctx, `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,abstract,summary,context,question,options,status,kind,origin,delivery_mode,created_at,updated_at,heartbeat_at,notification_id,source_name,source_url,notification_group) VALUES(?,NULL,'',?,?,?,?,?,?,?,'pending','question','notification','notification',?,?,?,?,?,?,?)`, id, "notification:"+id, c.Key, c.Abstract, c.Summary, c.Context, c.Question, string(opts), now, now, now, a.EventIDs[0], a.SourceName, a.SourceURL, id); err != nil {
				return err
			}
			if a.Plan == "" {
				return ErrInvalid
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO notification_card_plans(question_id,rule_set_id,plan) VALUES(?,?,?)`, id, captured.String, a.Plan); err != nil {
				return err
			}
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
	// A one-time exception is consumed only after its exact inputs reach a
	// terminal decision. Keep it across another clarification, otherwise two
	// uncertain rules can send the user back and forth indefinitely.
	if _, err = tx.ExecContext(ctx, `DELETE FROM notification_rule_overrides AS o WHERE o.rule_set_id=? AND EXISTS (
  SELECT 1 FROM notification_card_plans p,json_each(p.plan,'$.uncertain') u
  WHERE p.question_id=o.question_id AND json_extract(u.value,'$.id')=o.activity_id
  AND EXISTS(SELECT 1 FROM json_each(u.value,'$.event_ids') j JOIN notification_batch_events b ON b.event_id=j.value WHERE b.batch_id=?)
  AND NOT EXISTS(SELECT 1 FROM json_each(u.value,'$.event_ids') j JOIN notification_events e ON e.id=j.value WHERE e.raw_status='pending')
  AND NOT EXISTS(SELECT 1 FROM notification_rule_decisions d,json_each(d.event_ids) j WHERE d.batch_id=? AND d.outcome='clarification' AND j.value IN (SELECT value FROM json_each(u.value,'$.event_ids')))
 )`, captured.String, batchID, batchID); err != nil {
		return err
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
func (s *Service) publishFollowupLocked(ctx context.Context, previous Question, c Create) (Question, error) {
	var group string
	if err := s.db.QueryRowContext(ctx, `SELECT notification_group FROM inbox_questions WHERE id=?`, previous.ID).Scan(&group); err != nil {
		return Question{}, err
	}
	if group == "" {
		return s.publishNotificationLocked(ctx, previous.NotificationID, previous.SourceName, previous.SourceURL, c, false)
	}
	q, err := scan(s.db.QueryRowContext(ctx, selectQuestion+`WHERE q.notification_group=? AND q.status='pending'`, group))
	if err == nil {
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
	if err = before(tx); err != nil {
		return Question{}, err
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

// Merge the bounded plan lists while preserving the original decision and
// options. All changes occur under the batch Inbox snapshot transaction.
func mergeClarificationPlans(previous, incoming string) (string, error) {
	old, next := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(previous), &old); err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(incoming), &next); err != nil {
		return "", err
	}
	for _, key := range []string{"activities", "uncertain", "rule_ids", "bindings"} {
		var a, b []json.RawMessage
		if raw := old[key]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
		}
		if raw := next[key]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &b); err != nil {
				return "", err
			}
		}
		seen := map[string]bool{}
		all := []json.RawMessage{}
		for _, v := range append(a, b...) {
			identity := string(v)
			if key == "bindings" {
				var b TaskLink
				if err := json.Unmarshal(v, &b); err != nil {
					return "", err
				}
				identity = fmt.Sprint(b.TaskID)
			}
			if !seen[identity] {
				all = append(all, v)
				seen[identity] = true
			}
		}
		if len(all) > 1000 {
			return "", errors.New("clarification group exceeds capacity")
		}
		old[key], _ = json.Marshal(all)
	}
	raw, err := json.Marshal(old)
	return string(raw), err
}

func mergeClarificationContext(previous, incoming string) string {
	for _, line := range strings.Split(incoming, "\n") {
		if strings.HasPrefix(line, "https://") && !strings.Contains(previous, line) && len(previous)+len(line) < 23000 {
			previous += "\n" + line
		}
	}
	return previous
}

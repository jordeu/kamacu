package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
)

// PublishNotificationAttention atomically acknowledges a durable failure notice
// with its card. A separate stable group prevents another pending clarification
// from hiding the failure. Repeated delivery returns the same card, even after
// the user has answered or dismissed it.
func (s *Service) PublishNotificationAttention(ctx context.Context, runID string, c Create) (Question, error) {
	c.Key = "action-attention:" + runID
	if err := validate(c); err != nil {
		return Question{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var pending bool
	var existing string
	if err := s.db.QueryRowContext(ctx, `SELECT attention_pending,attention_question_id FROM notification_runs WHERE id=?`, runID).Scan(&pending, &existing); err != nil {
		return Question{}, err
	}

	if existing != "" {
		q, err := s.get(ctx, existing)
		if err != nil {
			return Question{}, err
		}
		if pending && q.Status == "unanswered" && q.Reason == "automatic_recovery" {
			reason := ""
			if c.Informational {
				reason = "operational_failure"
			}
			opts, _ := json.Marshal(c.Options)
			err = s.transitionWith(ctx, func(tx *sql.Tx) error {
				if _, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_task_links(question_id,task_id,role,source,evidence) SELECT ?,task_id,role,source,evidence FROM notification_task_links WHERE question_id=?`, existing, runID); e != nil {
					return e
				}
				_, e := tx.ExecContext(ctx, `UPDATE notification_runs SET attention_pending=0 WHERE id=?`, runID)
				return e
			}, existing, "notification_recovery_stopped", "", `UPDATE inbox_questions SET status='pending',reason=?,abstract=?,summary=?,context=?,question=?,options=?,revision=revision+1,updated_at=? WHERE id=?`, reason, c.Abstract, c.Summary, c.Context, c.Question, string(opts), s.now().UnixMilli(), existing)
			if err != nil {
				return Question{}, err
			}
			return s.get(ctx, existing)
		}
		if pending {
			if _, err := s.db.ExecContext(ctx, `UPDATE notification_runs SET attention_pending=0 WHERE id=?`, runID); err != nil {
				return Question{}, err
			}
		}
		return q, nil
	}

	previous, err := s.get(ctx, runID)
	if err != nil {
		return Question{}, err
	}
	if previous.Origin != "notification" {
		return Question{}, ErrInvalid
	}
	if !pending {
		return previous, nil
	}
	return s.publishFollowupLocked(ctx, previous, c, runID)
}

// ReconcileNotificationAttention replaces only an unanswered notice. Answers and
// their execution records are immutable; reconciliation never reopens them.
func (s *Service) ReconcileNotificationAttention(ctx context.Context, id string, revision int64, c Create, outcome string) error {
	c.Key = "handoff:" + id
	if err := validate(c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, reason := "pending", "handoff_attention"
	if outcome == "resolved" {
		status, reason = "unanswered", "handoff_resolved"
	}
	if outcome == "being_handled" {
		status, reason = "unanswered", "handoff_tracking"
	}
	opts, err := json.Marshal(c.Options)
	if err != nil {
		return err
	}
	return s.transitionWith(ctx, func(tx *sql.Tx) error {
		var current int64
		var status, reason string
		if err := tx.QueryRowContext(ctx, `SELECT revision,status,reason FROM inbox_questions WHERE id=?`, id).Scan(&current, &status, &reason); err != nil {
			return err
		}
		if current != revision || (status != "pending" && !(status == "unanswered" && (reason == "handoff_resolved" || reason == "handoff_tracking"))) {
			return ErrConflict
		}
		return nil
	}, id, "notification_handoff_reconciled", "", `UPDATE inbox_questions SET status=?,reason=?,abstract=?,summary=?,context=?,question=?,options=?,revision=revision+1,updated_at=? WHERE id=? AND (status='pending' OR (status='unanswered' AND reason IN ('handoff_resolved','handoff_tracking'))) AND revision=?`, status, reason, c.Abstract, c.Summary, c.Context, c.Question, string(opts), s.now().UnixMilli(), id, revision)
}

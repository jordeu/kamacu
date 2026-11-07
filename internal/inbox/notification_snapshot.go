package inbox

import (
	"context"
	"database/sql"
	"fmt"
)

// Use the same relation when capturing and validating dependencies. Grouped
// questions can be related through any contributing notification, not just the
// primary link displayed on the card. Keep resolved history in the dependency
// set as well, so a follow-up opened and answered during inference is detected.
// Only application-generated cards have this typed option. Conversation
// follow-ups may retain provenance but must not become merge destinations.
const nativeClarificationOption = `EXISTS(SELECT 1 FROM json_each(q.options) o WHERE json_extract(o.value,'$.id')='rule_continue_once')`

const relatedNotificationQuestions = `SELECT q.id,q.revision,q.status FROM inbox_questions q WHERE q.origin='notification' AND ((q.status='pending' AND ` + nativeClarificationOption + ` AND EXISTS(SELECT 1 FROM notification_card_plans p WHERE p.question_id=q.id AND COALESCE(json_extract(p.plan,'$.clarification_key'),'')!='')) OR EXISTS (SELECT 1 FROM notification_events current JOIN notification_batch_events b ON b.event_id=current.id JOIN notification_events old ON old.source_id=current.source_id AND old.external_id=current.external_id WHERE b.batch_id=? AND (q.notification_id=old.id OR EXISTS(SELECT 1 FROM notification_question_events links WHERE links.question_id=q.id AND links.event_id=old.id))))`

// CaptureNotificationBatchQuestions runs in the caller's snapshot transaction.
// It deliberately acquires no inbox mutex while that transaction is held.
func CaptureNotificationBatchQuestions(ctx context.Context, tx *sql.Tx, batchID string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notification_batch_questions(batch_id,question_id,revision,status) SELECT ?,related.id,related.revision,related.status FROM (`+relatedNotificationQuestions+`) related`, batchID, batchID)
	return err
}
func checkNotificationBatchQuestions(ctx context.Context, tx *sql.Tx, batchID string) error {
	var changed int
	err := tx.QueryRowContext(ctx, `WITH current_questions AS (`+relatedNotificationQuestions+`),captured_questions AS (SELECT question_id AS id,revision,status FROM notification_batch_questions WHERE batch_id=?) SELECT (SELECT count(*) FROM captured_questions c LEFT JOIN current_questions n ON n.id=c.id AND n.revision=c.revision AND n.status=c.status WHERE n.id IS NULL)+(SELECT count(*) FROM current_questions n LEFT JOIN captured_questions c ON c.id=n.id AND c.revision=n.revision AND c.status=n.status WHERE c.id IS NULL)`, batchID, batchID).Scan(&changed)
	if err != nil {
		return err
	}
	if changed > 0 {
		return fmt.Errorf("related inbox questions changed during processing; notifications remain pending, process them again: %w", ErrConflict)
	}
	return nil
}

// NotificationBatchQuestions returns exactly the still-current decision context
// captured by the request. A dismissal, answer, edit or new follow-up invalidates
// the plan both here and again inside the final publication transaction.
func (s *Service) NotificationBatchQuestions(ctx context.Context, batchID string) ([]Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = checkNotificationBatchQuestions(ctx, tx, batchID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, selectQuestion+`JOIN notification_batch_questions b ON b.question_id=q.id WHERE b.batch_id=? AND q.status='pending' ORDER BY q.id`, batchID)
	if err != nil {
		return nil, err
	}
	out := []Question{}
	for rows.Next() {
		q, e := scan(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, q)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

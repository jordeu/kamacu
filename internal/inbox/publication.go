package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// SetBeforePublish installs the last processing step. It runs outside inbox,
// presence and database locks. Messages stay invisible until this step finishes.
func (s *Service) SetBeforePublish(fn func(context.Context, Question) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beforePublish = fn
}
func (s *Service) FinalizePublication(ctx context.Context, id string) error {
	readCtx, cancelRead := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelRead()
	s.mu.Lock()
	if s.finalizing[id] {
		s.mu.Unlock()
		return nil
	}
	var staged bool
	err := s.db.QueryRowContext(readCtx, `SELECT publishing FROM inbox_questions WHERE id=?`, id).Scan(&staged)
	hook := s.beforePublish
	if err == nil && staged {
		if s.finalizing == nil {
			s.finalizing = map[string]bool{}
		}
		s.finalizing[id] = true
	}
	s.mu.Unlock()
	if err != nil || !staged {
		return err
	}
	defer func() { s.mu.Lock(); delete(s.finalizing, id); s.mu.Unlock() }()
	q, err := s.Get(readCtx, id)
	if err != nil {
		return err
	}
	var failure error
	if hook != nil && q.Status == "pending" {
		failure = hook(ctx, q)
	}
	record, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	latest, err := s.get(record, id)
	if err != nil {
		return err
	}
	// A newer monitoring revision needs its own assessment. Leave it staged;
	// the monitor's next publication pass will finalize that revision.
	if latest.Revision != q.Revision && latest.Status == "pending" && latest.Automation == nil {
		return nil
	}
	if failure != nil && latest.Revision == q.Revision && latest.Status == "pending" && latest.Automation == nil {
		_, err = s.db.ExecContext(record, `UPDATE inbox_questions SET automation=? WHERE id=? AND revision=?`, mustAutomation(Automation{Status: "failed", Error: failure.Error()}), id, q.Revision)
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(record, `UPDATE inbox_questions SET publishing=0 WHERE id=? AND revision=?`, id, latest.Revision)
	if err == nil {
		s.signal()
	}
	return err
}
func mustAutomation(a Automation) string { raw, _ := json.Marshal(a); return string(raw) }

// RecordAutomationFailure keeps the original message and attempted response in
// Pending. Uncertain terminal delivery stays guarded against a second send.
func (s *Service) RecordAutomationFailure(ctx context.Context, id string, revision int64, a Automation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if q.Revision != revision || (q.Status != "pending" && !(q.Status == "answered" && q.AnswerSource == "inbox_agent")) {
		return nil
	}
	return s.transitionWith(ctx, nil, id, "automatic_reply_failed", "", `UPDATE inbox_questions SET status='pending',delivery_status=CASE WHEN answer_source='inbox_agent' AND origin='observer' AND delivery_status='attempting' THEN 'uncertain' ELSE delivery_status END,automation=?,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, mustAutomation(a), s.now().UnixMilli(), id, revision)
}

// SaveAutomation is called in the same transaction as the answer and its action.
func SaveAutomation(ctx context.Context, tx *sql.Tx, id string, a Automation) error {
	_, err := tx.ExecContext(ctx, `UPDATE inbox_questions SET automation=? WHERE id=?`, mustAutomation(a), id)
	return err
}

// RecoverPublications publishes interrupted decisions without rerunning them.
func (s *Service) RecoverPublications(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A confirmed handoff/terminal submission is a successful response even if
	// the process stopped before the final visibility/audit update.
	_, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET publishing=0 WHERE publishing=1 AND status='answered' AND answer_source='inbox_agent' AND (origin='notification' OR delivery_status='submitted' OR reason IN ('exit','move_to_review','move_to_done'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET publishing=0,status=CASE WHEN answer_source='inbox_agent' AND status='answered' THEN 'pending' ELSE status END,delivery_status=CASE WHEN answer_source='inbox_agent' AND origin='observer' AND delivery_status IN ('','attempting') THEN 'uncertain' ELSE delivery_status END,automation=json_set(automation,'$.status','failed','$.error','Inbox assessment or reply was interrupted. Inspect the saved response and delivery status; no automatic resend was attempted.') WHERE publishing=1 OR (origin='observer' AND answer_source='inbox_agent' AND status IN ('pending','answered') AND reason NOT IN ('exit','move_to_review','move_to_done') AND delivery_status IN ('','attempting','uncertain'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE inbox_agent_decisions SET outcome=CASE WHEN EXISTS(SELECT 1 FROM inbox_questions q WHERE q.id=question_id AND q.status='answered' AND q.answer_source='inbox_agent') THEN 'answered' ELSE 'interrupted' END,error=COALESCE((SELECT json_extract(q.automation,'$.error') FROM inbox_questions q WHERE q.id=question_id),'') WHERE outcome IN ('assessing','authorized')`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

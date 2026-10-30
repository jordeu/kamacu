package inbox

import (
	"context"
	"errors"
)

// Observe keeps a stable message identity for an attention episode. Archived
// episodes are never reopened by a repaint, reconnect or another observation.
func (s *Service) Observe(ctx context.Context, who Identity, c Create, kind, mode string, revision, input uint64) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.origin, c.deliveryMode, c.evidenceRevision, c.inputGeneration = "observer", mode, revision, input
	out, err := s.createLocked(ctx, who, c, kind)
	if err != nil {
		return out, err
	}
	q := out.Question
	if out.Outcome == "existing_pending" || q.Status != "pending" {
		return out, nil
	}
	if q.Origin == "observer" && q.Question == c.Question && q.DeliveryMode == mode && q.EvidenceRevision == revision && q.InputGeneration == input {
		return out, nil
	}
	bump := 0
	if q.Question != c.Question || q.DeliveryMode != mode || q.EvidenceRevision != revision || q.InputGeneration != input {
		bump = 1
	}
	_, err = s.db.ExecContext(ctx, `UPDATE inbox_questions SET revision=revision+?,origin='observer',delivery_mode=?,evidence_revision=?,input_generation=?,summary=?,abstract=?,context=?,question=?,updated_at=? WHERE id=?`, bump, mode, revision, input, c.Summary, c.Abstract, c.Context, c.Question, s.now().UnixMilli(), q.ID)
	if err != nil {
		return out, err
	}
	s.signal()
	q, err = s.get(ctx, q.ID)
	return Result{out.Outcome, q}, err
}
func (s *Service) HideObserved(ctx context.Context, session string, hide bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.db.ExecContext(ctx, `UPDATE inbox_questions SET hidden=? WHERE session_id=? AND status='pending' AND origin='observer' AND hidden!=?`, hide, session, hide)
	if e == nil {
		n, _ := r.RowsAffected()
		if n > 0 {
			s.signal()
		}
	}
	return e
}
func (s *Service) SupersedeObserved(ctx context.Context, session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.db.ExecContext(ctx, `UPDATE inbox_questions SET status='unanswered',reason='superseded',hidden=0,revision=revision+1,updated_at=? WHERE session_id=? AND status='pending' AND origin='observer'`, s.now().UnixMilli(), session)
	if e == nil {
		n, _ := r.RowsAffected()
		if n > 0 {
			s.signal()
		}
	}
	return e
}
func (s *Service) RecordDelivery(ctx context.Context, id, status string) error {
	if status != "submitted" && status != "uncertain" && status != "attempting" {
		return errors.New("invalid delivery status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e := s.db.ExecContext(ctx, `UPDATE inbox_questions SET delivery_status=? WHERE id=? AND origin='observer'`, status, id)
	if e == nil {
		s.signal()
	}
	return e
}

// UseTerminalMonitoring disables creation through legacy agent feedback tools.
// History/acknowledgement remains available for recovering previous answers.
func (s *Service) UseTerminalMonitoring() { s.mu.Lock(); s.terminalManaged = true; s.mu.Unlock() }

// RecoverDeliveries runs once at server startup. A crash after saving an answer
// or writing to the PTY cannot establish whether the agent received it.
func (s *Service) RecoverDeliveries(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.db.ExecContext(ctx, `UPDATE inbox_questions SET delivery_status='uncertain' WHERE origin='observer' AND status='answered' AND reason!='exit' AND delivery_status IN ('','attempting')`)
	if err == nil {
		if n, _ := r.RowsAffected(); n > 0 {
			s.signal()
		}
	}
	return err
}

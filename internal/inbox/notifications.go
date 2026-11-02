package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// PublishNotification creates session-independent questions, coalescing updates
// to a source thread while it is pending. Revisions invalidate older buttons.
func (s *Service) PublishNotification(ctx context.Context, eventID int64, source, url string, c Create) (Question, error) {
	c.Key = fmt.Sprintf("notification:%d", eventID)
	if err := validate(c); err != nil {
		return Question{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishNotificationLocked(ctx, eventID, source, url, c, true)
}
func (s *Service) publishNotificationLocked(ctx context.Context, eventID int64, source, url string, c Create, update bool) (Question, error) {
	q, err := s.pendingNotificationLocked(ctx, eventID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Question{}, err
	}
	if err == nil && !update {
		return q, nil
	}
	if c.Options == nil {
		c.Options = []Option{}
	}
	opts, _ := json.Marshal(c.Options)
	now := s.now().UnixMilli()
	markPublished := func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE notification_events SET state='published',last_error='' WHERE id=?`, eventID)
		return e
	}
	if err == nil {
		err = s.transitionWith(ctx, markPublished, q.ID, "notification_updated", "", `UPDATE inbox_questions SET notification_id=?,source_name=?,source_url=?,abstract=?,summary=?,context=?,question=?,options=?,revision=revision+1,updated_at=? WHERE id=?`, eventID, source, url, c.Abstract, c.Summary, c.Context, c.Question, string(opts), now, q.ID)
	} else {
		q.ID = uuid.NewString()
		err = s.transitionWith(ctx, markPublished, q.ID, "notification_created", "", `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,abstract,summary,context,question,options,status,kind,origin,delivery_mode,created_at,updated_at,heartbeat_at,notification_id,source_name,source_url) VALUES(?,NULL,'',?,?,?,?,?,?,?,'pending','question','notification','notification',?,?,?,?,?,?)`, q.ID, "notification:"+q.ID, q.ID, c.Abstract, c.Summary, c.Context, c.Question, string(opts), now, now, now, eventID, source, url)
	}
	if err != nil {
		return Question{}, err
	}
	return s.get(ctx, q.ID)
}

// FinishNotification records execution separately from the user's saved answer.
func (s *Service) FinishNotification(ctx context.Context, id, state, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(ctx, id, "execution_"+state, "", `UPDATE inbox_questions SET delivery_status=?,execution_result=?,updated_at=? WHERE id=? AND origin='notification'`, state, result, s.now().UnixMilli(), id)
}

// ContinueNotification creates a fresh reply opportunity without rewriting the
// user's earlier answer or replacing a newer pending question for this thread.
func (s *Service) ContinueNotification(ctx context.Context, id string) (Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.get(ctx, id)
	if err != nil {
		return q, err
	}
	if q.Origin != "notification" || q.Status == "pending" || q.DeliveryStatus == "queued" || q.DeliveryStatus == "running" {
		return q, ErrConflict
	}
	c := Create{Abstract: q.Abstract, Summary: q.Summary, Context: q.Context, Question: "What would you like to do next with this notification?"}
	if q.ExecutionResult != "" {
		c.Context += "\n\nPrevious outcome:\n" + q.ExecutionResult
	}
	if len(c.Context) > 32000 {
		c.Context = q.ExecutionResult
	}
	return s.publishNotificationLocked(ctx, q.NotificationID, q.SourceName, q.SourceURL, c, false)
}

// PendingNotification snapshots the attention decision for a source thread.
func (s *Service) PendingNotification(ctx context.Context, eventID int64) (Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingNotificationLocked(ctx, eventID)
}
func (s *Service) pendingNotificationLocked(ctx context.Context, eventID int64) (Question, error) {
	return scan(s.db.QueryRowContext(ctx, selectQuestion+`JOIN notification_events e ON e.id=q.notification_id WHERE q.origin='notification' AND q.status='pending' AND (e.source_id,e.external_id)=(SELECT source_id,external_id FROM notification_events WHERE id=?)`, eventID))
}

// SupersedeNotification archives only the exact pending decision classified by
// triage. A changed card is reconsidered; an already answered card stays intact.
func (s *Service) SupersedeNotification(ctx context.Context, eventID int64, expectedID string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.pendingNotificationLocked(ctx, eventID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	markIgnored := func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE notification_events SET state='ignored',last_error='' WHERE id=?`, eventID)
		return e
	}
	if errors.Is(err, ErrNotFound) {
		_, err = s.db.ExecContext(ctx, `UPDATE notification_events SET state='ignored',last_error='' WHERE id=?`, eventID)
		return err
	}
	if q.ID != expectedID || q.Revision != revision {
		return ErrConflict
	}
	return s.transitionWith(ctx, markIgnored, q.ID, "notification_superseded", "", `UPDATE inbox_questions SET status='unanswered',reason='notification_superseded',revision=revision+1,updated_at=? WHERE id=?`, s.now().UnixMilli(), q.ID)
}

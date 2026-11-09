package inbox

import "context"

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
		if pending {
			if _, err := s.db.ExecContext(ctx, `UPDATE notification_runs SET attention_pending=0 WHERE id=?`, runID); err != nil {
				return Question{}, err
			}
		}
		return s.get(ctx, existing)
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

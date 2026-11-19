package notifications

import (
	"context"
	"errors"
	"time"

	"kamacu/internal/settings"
)

// scheduleAutomaticProcessing runs under workMu. A changed interval starts a
// fresh wait; disabling cancels future runs without interrupting active work.
// The preference survives restarts; the wait starts again when the service does.
func (s *Service) scheduleAutomaticProcessing(ctx context.Context, now time.Time) error {
	value, err := settings.Get(s.DB, settings.KeyNotificationAutoProcess)
	if err != nil {
		return err
	}
	var interval time.Duration
	if value != "off" {
		if err := settings.Validate(settings.KeyNotificationAutoProcess, value); err != nil {
			return err
		}
		interval, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	}
	if interval != s.automaticInterval {
		s.automaticInterval = interval
		s.nextAutomaticProcessing = now.Add(interval)
	}
	if interval == 0 || now.Before(s.nextAutomaticProcessing) {
		return nil
	}
	// Empty queues and failures also wait a full interval before trying again.
	s.nextAutomaticProcessing = now.Add(interval)
	status, err := s.Status(ctx)
	if err != nil {
		return err
	}
	if status.PendingCount == 0 || !status.ModelConfigured {
		return nil
	}
	_, err = s.RequestProcessing(ctx)
	if errors.Is(err, ErrNoPending) || errors.Is(err, ErrNoActiveRules) {
		return nil
	}
	return err
}

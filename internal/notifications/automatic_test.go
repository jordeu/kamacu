package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"kamacu/internal/settings"
)

func setAutomaticInterval(t *testing.T, s *Service, value string) {
	t.Helper()
	if err := settings.Set(s.DB, settings.KeyNotificationAutoProcess, value); err != nil {
		t.Fatal(err)
	}
}

func scheduleAt(t *testing.T, s *Service, now time.Time) {
	t.Helper()
	if err := s.scheduleAutomaticProcessing(context.Background(), now); err != nil {
		t.Fatal(err)
	}
}

func batchCount(t *testing.T, s *Service) int {
	t.Helper()
	var count int
	if err := s.DB.QueryRow(`SELECT count(*) FROM notification_batches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAutomaticProcessingIntervals(t *testing.T) {
	for _, value := range []string{"5m", "10m", "20m", "40m"} {
		t.Run(value, func(t *testing.T) {
			s, _, _, source := fixture(t)
			seed(t, s, source, event())
			setAutomaticInterval(t, s, value)
			now := time.Now()
			interval, _ := time.ParseDuration(value)
			scheduleAt(t, s, now)
			scheduleAt(t, s, now.Add(interval-time.Nanosecond))
			if batchCount(t, s) != 0 {
				t.Fatal("scheduled before interval elapsed")
			}
			scheduleAt(t, s, now.Add(interval))
			if batchCount(t, s) != 1 {
				t.Fatal("did not schedule at interval boundary")
			}
			// RequestProcessing must reuse the active snapshot on later ticks.
			scheduleAt(t, s, now.Add(2*interval))
			if batchCount(t, s) != 1 {
				t.Fatal("duplicated an active batch")
			}
		})
	}
}

func TestAutomaticProcessingOptInChangeAndDisable(t *testing.T) {
	s, _, _, source := fixture(t)
	seed(t, s, source, event())
	now := time.Now()
	scheduleAt(t, s, now)
	scheduleAt(t, s, now.Add(time.Hour))
	if batchCount(t, s) != 0 {
		t.Fatal("automatic processing must be opt-in")
	}
	setAutomaticInterval(t, s, "20m")
	scheduleAt(t, s, now)
	setAutomaticInterval(t, s, "40m")
	scheduleAt(t, s, now.Add(19*time.Minute))
	scheduleAt(t, s, now.Add(40*time.Minute))
	if batchCount(t, s) != 0 {
		t.Fatal("changing the interval did not restart the wait")
	}
	setAutomaticInterval(t, s, "off")
	scheduleAt(t, s, now.Add(41*time.Minute))
	scheduleAt(t, s, now.Add(2*time.Hour))
	if batchCount(t, s) != 0 {
		t.Fatal("disabling did not cancel scheduling")
	}
	setAutomaticInterval(t, s, "10m")
	scheduleAt(t, s, now.Add(2*time.Hour))
	scheduleAt(t, s, now.Add(2*time.Hour+10*time.Minute))
	if batchCount(t, s) != 1 {
		t.Fatal("could not re-enable processing")
	}
}

func TestAutomaticProcessingSkipsEmptyQueueAndMissingModel(t *testing.T) {
	s, _, _, source := fixture(t)
	setAutomaticInterval(t, s, "20m")
	now := time.Now()
	scheduleAt(t, s, now)
	scheduleAt(t, s, now.Add(20*time.Minute))
	if batchCount(t, s) != 0 {
		t.Fatal("created an empty batch")
	}
	seed(t, s, source, event())
	if _, err := s.DB.Exec(`UPDATE notification_settings SET profile_id=NULL`); err != nil {
		t.Fatal(err)
	}
	scheduleAt(t, s, now.Add(40*time.Minute))
	if batchCount(t, s) != 0 {
		t.Fatal("scheduled without a notification model")
	}
	configureTestModel(t, s)
	scheduleAt(t, s, now.Add(41*time.Minute))
	if batchCount(t, s) != 0 {
		t.Fatal("retried before the next interval")
	}
	scheduleAt(t, s, now.Add(60*time.Minute))
	if batchCount(t, s) != 1 {
		t.Fatal("did not process new pending notifications")
	}
}

func TestTickAutomaticallyProcessesPendingNotifications(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) {
		return finalResponse(inputSnapshot(t, prompt), false), nil
	}
	setAutomaticInterval(t, s, "20m")
	scheduleAt(t, s, time.Now().Add(-21*time.Minute))
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingCount != 0 || status.Processing == nil || status.Processing.Status != "completed" {
		t.Fatalf("automatic run did not complete: %+v", status)
	}
}

func TestAutomaticProcessingFailureWaitsBeforeRetry(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.respond = func(string) (any, error) {
		return nil, errors.New("model unavailable")
	}
	setAutomaticInterval(t, s, "5m")
	now := time.Now()
	scheduleAt(t, s, now)
	scheduleAt(t, s, now.Add(5*time.Minute))
	if err := s.processNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingCount != 1 || status.Processing == nil || status.Processing.Status != "failed" {
		t.Fatalf("failed run did not preserve pending notifications: %+v", status)
	}
	scheduleAt(t, s, now.Add(6*time.Minute))
	if batchCount(t, s) != 1 {
		t.Fatal("failed run retried before the next interval")
	}
	scheduleAt(t, s, now.Add(10*time.Minute))
	if batchCount(t, s) != 2 {
		t.Fatal("failed run was not retried at the next interval")
	}
}

func TestDisablingAutomaticProcessingKeepsManualRun(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) {
		return finalResponse(inputSnapshot(t, prompt), false), nil
	}
	setAutomaticInterval(t, s, "5m")
	now := time.Now()
	scheduleAt(t, s, now)
	manual, err := s.RequestProcessing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	setAutomaticInterval(t, s, "off")
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingCount != 0 || status.Processing == nil || status.Processing.ID != manual.ID || status.Processing.Status != "completed" {
		t.Fatalf("disabling interfered with manual processing: %+v", status)
	}
	nextEvent := event()
	nextEvent.ExternalID = "new"
	seed(t, s, source, nextEvent)
	scheduleAt(t, s, now.Add(time.Hour))
	if batchCount(t, s) != 1 {
		t.Fatal("disabled automatic processing created a new batch")
	}
}

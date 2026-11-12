package notifications

import (
	"context"
	"testing"
	"time"
)

func oldIncompleteDecision(t *testing.T, f *activityGitHubFixture) (string, int64) {
	t.Helper()
	id := seed(t, f.s, f.source, f.event)
	batch := "old-incomplete"
	snapshot := mustJSON(map[string]any{"subjects": []any{map[string]any{"source": map[string]any{"url": f.event.URL, "evidence_complete": false}}}})
	if _, err := f.s.DB.Exec(`INSERT INTO notification_batches(id,status,created_at,updated_at,processor_snapshot) VALUES(?,'completed',?,?,?)`, batch, time.Now().UnixMilli(), time.Now().UnixMilli(), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO notification_batch_events(batch_id,event_id,revision) VALUES(?,?,1)`, batch, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE notification_events SET raw_status='archived',state='ignored' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	return batch, id
}
func TestRepairPreviewDoesNotChangeHistoryAndRequeuesSelectedOnly(t *testing.T) {
	f := newActivityFixture(t)
	_, id := oldIncompleteDecision(t, f)
	items, err := f.s.NotificationRepairPreview(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	var status string
	f.s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&status)
	if status != "archived" {
		t.Fatal("preview mutated history", status)
	}
	if err = f.s.RepairNotifications(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	f.s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&status)
	if status != "pending" {
		t.Fatal(status)
	}
	var calls int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_batches WHERE status IN ('queued','running')`).Scan(&calls)
	if calls != 0 || len(f.model.prompts) != 0 {
		t.Fatal("repair started processing")
	}
	var repairFrom string
	f.s.DB.QueryRow(`SELECT repair_from FROM notification_activity_subjects`).Scan(&repairFrom)
	if repairFrom == "" {
		t.Fatal("repair window missing")
	}
	if err = f.s.RepairNotifications(context.Background(), items); err == nil {
		t.Fatal("stale repair preview reused")
	}
}
func TestRepairSkipsNewerCompleteDecision(t *testing.T) {
	f := newActivityFixture(t)
	_, id := oldIncompleteDecision(t, f)
	snapshot := mustJSON(map[string]any{"subjects": []any{map[string]any{"source": map[string]any{"url": f.event.URL, "evidence_complete": true}}}})
	_, err := f.s.DB.Exec(`INSERT INTO notification_batches(id,status,created_at,updated_at,processor_snapshot) VALUES('new-complete','completed',?,?,?)`, time.Now().Add(time.Second).UnixMilli(), time.Now().UnixMilli(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO notification_batch_events VALUES('new-complete',?,1)`, id); err != nil {
		t.Fatal(err)
	}
	items, err := f.s.NotificationRepairPreview(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}
func TestRepairIncludesUnseenFeedbackOlderThanLatestNotification(t *testing.T) {
	f := newActivityFixture(t)
	f.event.Version = f.now.Format(time.RFC3339)
	oldIncompleteDecision(t, f)
	items, err := f.s.NotificationRepairPreview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.RepairNotifications(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	f.addReview()
	f.reviews[0]["submitted_at"] = f.now.Add(-4 * time.Hour).Format(time.RFC3339)
	// Repair must not baseline the missed review as merely historical.
	p := processAgent(t, f.s)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var done int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 2 {
		t.Fatal("old missed feedback was not reconsidered", done)
	}
}

func TestRepairPromotesUnseenContextWithoutReplayingKnownDecisions(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.reviews[0]["submitted_at"] = f.now.Add(-4 * time.Hour).Format(time.RFC3339)
	seed(t, f.s, f.source, f.event)
	// An earlier baseline saw this old feedback as context, without deciding it.
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.ActivityDelta.Activities) != 0 {
		t.Fatal("expected historical baseline", fresh.ActivityDelta)
	}
	oldIncompleteDecision(t, f)
	items, err := f.s.NotificationRepairPreview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.RepairNotifications(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	p := processAgent(t, f.s)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var done int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 2 {
		t.Fatal("missed context was not promoted for repair", done)
	}
}
func TestBootstrapDoesNotHideKnownIncompleteHistoricalWindow(t *testing.T) {
	f := newActivityFixture(t)
	f.event.Version = f.now.Add(-4 * time.Hour).Format(time.RFC3339)
	oldIncompleteDecision(t, f)
	f.addReview()
	f.reviews[0]["submitted_at"] = f.event.Version
	f.event.Version = f.now.Format(time.RFC3339)
	p := f.process(t)
	if p.Status != "completed" {
		t.Fatal(p)
	}
	var done int
	f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done)
	if done != 2 {
		t.Fatal("bootstrap hid an earlier incomplete window", done)
	}
}

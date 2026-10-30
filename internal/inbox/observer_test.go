package inbox

import (
	"context"
	"testing"
)

func TestObservedVisibilityAndDismissal(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "observer:test", Abstract: "Continue?", Summary: "Continue?", Context: "Terminal asks whether to continue", Question: "Continue?"}
	out, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	id := out.Question.ID
	if err = s.HideObserved(ctx, who.SessionID, true); err != nil {
		t.Fatal(err)
	}
	page, err := s.List(ctx, nil, false, 0)
	if err != nil || page.PendingCount != 0 || len(page.Items) != 0 {
		t.Fatal(page, err)
	}
	if _, err = s.Answer(ctx, nil, id, 1, "yes", ""); err == nil {
		t.Fatal("hidden question accepted reply")
	}
	_ = s.HideObserved(ctx, who.SessionID, false)
	page, _ = s.List(ctx, nil, false, 0)
	if len(page.Items) != 1 || page.Items[0].ID != id {
		t.Fatal("identity lost")
	}
	_, err = s.Dismiss(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.Observe(ctx, who, c, "question", "text", 2, 0)
	if err != nil || out.Question.Status != "dismissed" {
		t.Fatal(out, err)
	}
}
func TestObservedMenuAndChangedQuestion(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "observer:test", Abstract: "Approve?", Summary: "Approve?", Context: "Permission menu", Question: "Approve?"}
	out, err := s.Observe(ctx, who, c, "question", "terminal", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Answer(ctx, nil, out.Question.ID, 1, "yes", ""); err == nil {
		t.Fatal("menu accepted text")
	}
	c.Question = "Proceed?"
	updated, err := s.Observe(ctx, who, c, "question", "text", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Question.Revision != 2 {
		t.Fatal("revision not incremented")
	}
	old, err := s.Answer(ctx, nil, out.Question.ID, 1, "yes", "")
	if err != nil || old.Outcome != "stale_revision" {
		t.Fatal(old, err)
	}
}

func TestTerminalMonitoringRejectsLegacyCreationAndControlInput(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	s.UseTerminalMonitoring()
	c := Create{Key: "auto", Abstract: "Proceed?", Summary: "Proceed?", Context: "Agent asks whether to continue", Question: "Proceed?"}
	if _, err := s.Create(ctx, who, c); err == nil {
		t.Fatal("legacy tool bypassed monitoring")
	}
	out, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Answer(ctx, nil, out.Question.ID, out.Question.Revision, "yes\x1b[201~\rno", ""); err == nil {
		t.Fatal("terminal escape accepted")
	}
	if _, err = s.Answer(ctx, nil, out.Question.ID, out.Question.Revision, "yes, continue\nwith tests", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverUncertainDeliveryWithoutRetry(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "crash", Abstract: "Continue?", Summary: "Continue?", Context: "Question", Question: "Continue?"}
	out, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Answer(ctx, nil, out.Question.ID, 1, "yes", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordDelivery(ctx, out.Question.ID, "attempting"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	q, err := s.Get(ctx, out.Question.ID)
	if err != nil || q.Status != "answered" || q.Answer != "yes" || q.DeliveryStatus != "uncertain" {
		t.Fatal(q, err)
	}
	// Confirmed writes must never become retry candidates on restart.
	if err = s.RecordDelivery(ctx, q.ID, "submitted"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	q, _ = s.Get(ctx, q.ID)
	if q.DeliveryStatus != "submitted" {
		t.Fatal(q)
	}
}

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

func TestObservedChoicesUpdateRejectsStaleClicks(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "choices", Abstract: "Which approach?", Summary: "Choose an approach", Context: "Two approaches", Question: "Which approach?"}
	original, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Options = []Option{{ID: "choice-1", Label: "1. Keep it"}, {ID: "choice-2", Label: "2. Replace it"}}
	updated, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil || updated.Question.ID != original.Question.ID || updated.Question.Revision != original.Question.Revision+1 || len(updated.Question.Options) != 2 {
		t.Fatal(updated, err)
	}
	unchanged, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil || unchanged.Question.Revision != updated.Question.Revision {
		t.Fatal(unchanged, err)
	}
	c.Options[0].Label = "1. Keep it for now"
	changed, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil || changed.Question.Revision != updated.Question.Revision+1 {
		t.Fatal(changed, err)
	}
	stale, err := s.Answer(ctx, nil, changed.Question.ID, updated.Question.Revision, "", "choice-1")
	if err != nil || stale.Outcome != "stale_revision" {
		t.Fatal(stale, err)
	}
	answered, err := s.Answer(ctx, nil, changed.Question.ID, changed.Question.Revision, "", "choice-1")
	if err != nil || answered.Question.Answer != "1. Keep it for now" {
		t.Fatal(answered, err)
	}
}

func TestObservedChoicesCanBeRemoved(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "choices", Abstract: "Continue?", Summary: "Continue?", Context: "Question", Question: "Continue?", Options: []Option{{ID: "choice-1", Label: "Yes"}}}
	original, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Options = nil
	updated, err := s.Observe(ctx, who, c, "question", "terminal", 2, 0)
	if err != nil || len(updated.Question.Options) != 0 || updated.Question.Revision <= original.Question.Revision {
		t.Fatal(updated, err)
	}
	if _, err = s.Answer(ctx, nil, updated.Question.ID, updated.Question.Revision, "", "choice-1"); err == nil {
		t.Fatal("terminal menu accepted a choice")
	}
}

func TestObservedMessageUpdatesWithoutNewTerminalEvidence(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	c := Create{Key: "message", Abstract: "Ready for review", Summary: "Ready for review", Context: "Initial details", Question: "Start the review?"}
	original, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"context", "summary", "abstract"} {
		switch field {
		case "context":
			c.Context = "I require workspace membership for uploads. All 12 tests passed."
		case "summary":
			c.Summary = "I restricted image uploads to workspace members."
		case "abstract":
			c.Abstract = c.Summary
		}
		updated, err := s.Observe(ctx, who, c, "question", "text", 1, 0)
		if err != nil || updated.Question.ID != original.Question.ID || updated.Question.Revision != original.Question.Revision+1 || updated.Question.Context != c.Context || updated.Question.Summary != c.Summary || updated.Question.Abstract != c.Abstract {
			t.Fatal(field, updated, err)
		}
		old, err := s.Answer(ctx, nil, original.Question.ID, original.Question.Revision, "yes", "")
		if err != nil || old.Outcome != "stale_revision" {
			t.Fatal(old, err)
		}
		original = updated
	}
}

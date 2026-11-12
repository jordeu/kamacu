package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBootstrapIncludesFeedbackBeforeLatestThreadUpdate(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.reviews[0]["submitted_at"] = f.now.Add(-time.Hour).Format(time.RFC3339)
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.ActivityDelta.Activities) != 2 {
		t.Fatalf("earlier unseen review excluded: %+v", fresh.ActivityDelta)
	}
}

func TestRetryPromotesDraftCommentsAfterReviewCatalogRecovers(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	run := f.github.Run
	unavailable := true
	f.github.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		if unavailable && strings.Contains(args[len(args)-1], "/reviews?") {
			return errorResponse(404), errors.New("missing")
		}
		return run(ctx, args...)
	}
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil || fresh.ActivityDelta.Complete {
		t.Fatalf("expected partial scan: %+v %v", fresh.ActivityDelta, err)
	}
	unavailable = false
	fresh, err = f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil || !fresh.ActivityDelta.Complete {
		t.Fatalf("retry: %+v %v", fresh.ActivityDelta, err)
	}
	if len(fresh.ActivityDelta.Activities) != 2 {
		t.Fatalf("submitted review comment was left historical: %+v", fresh.ActivityDelta)
	}
	f.s.Providers["github"] = f.github
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	var done int
	if err = f.s.DB.QueryRow(`SELECT count(*) FROM notification_activity_versions WHERE context_only=0 AND processed_batch_id IS NOT NULL`).Scan(&done); err != nil || done != 2 {
		t.Fatalf("unacknowledged review: %d %v", done, err)
	}
}

func TestDeletedLatestPointerDoesNotBlockAuthoritativeStreams(t *testing.T) {
	f := newActivityFixture(t)
	f.addReview()
	f.event.LatestCommentURL = "https://api.github.com/repos/acme/repo/issues/comments/999"
	run := f.github.Run
	f.github.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.HasSuffix(args[len(args)-1], "/comments/999") {
			return errorResponse(404), errors.New("deleted")
		}
		return run(ctx, args...)
	}
	f.s.Providers["github"] = f.github
	if p := f.process(t); p.Status != "completed" {
		t.Fatal(p)
	}
	f.failComments = true
	fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
	if err != nil || fresh.ActivityDelta.Complete {
		t.Fatalf("required stream failure was hidden: %+v %v", fresh.ActivityDelta, err)
	}
}

func TestLegacySeedUsesLatestAcknowledgedContent(t *testing.T) {
	for _, body := range []string{"original", "edited"} {
		t.Run(body, func(t *testing.T) {
			f := newActivityFixture(t)
			id := seed(t, f.s, f.source, f.event)
			c := activityComment(101, body, "reviewer", f.event.Version)
			f.inline = []map[string]any{c}
			for i, text := range []string{"original", "edited"} {
				batch := "legacy-" + formatID(int64(i))
				ev := SubjectEvidence{URL: c["html_url"].(string), Actor: "reviewer", Text: text, Complete: true}
				snap := mustJSON(AgentSnapshot{Subjects: []AgentSubject{{Event: Event{URL: f.event.URL, EvidenceComplete: true, Evidence: []SubjectEvidence{ev}}}}})
				if _, err := f.s.DB.Exec(`INSERT INTO notification_batches(id,status,created_at,updated_at,processor_snapshot) VALUES(?,'completed',?,?,?)`, batch, time.Now().Add(time.Duration(i)*time.Second).UnixMilli(), time.Now().UnixMilli(), snap); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.DB.Exec(`INSERT INTO notification_batch_events(batch_id,event_id,revision) VALUES(?,?,1)`, batch, id); err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := f.github.Enrich(context.Background(), f.source, f.event)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if body == "edited" {
				want = 0
			}
			if len(fresh.ActivityDelta.Activities) != want {
				t.Fatalf("latest legacy version not respected: %+v", fresh.ActivityDelta)
			}
		})
	}
}

func TestCoverageSummaryPreservesInterstitialProse(t *testing.T) {
	no := false
	prefix := "🧪 Test coverage: 80%\n## Code Coverage Summary\n"
	for _, blocks := range []string{
		"<details>table one</details>\nBuild failed; investigate this warning.\n<details>table two</details>",
		"<details>outer<details>inner</details>table</details>\nBuild failed; investigate this warning.\n<details>table two</details>",
	} {
		e := compactActivityEvidence(SubjectEvidence{Actor: "github-actions[bot]", ViewerMention: &no, Text: prefix + blocks, Complete: true})
		if !strings.Contains(e.Text, "Build failed; investigate this warning.") || strings.Contains(e.Text, "table two") || !e.Summarized {
			t.Fatal(e)
		}
	}
	original := prefix + "<details>unclosed table"
	e := compactActivityEvidence(SubjectEvidence{Actor: "github-actions[bot]", ViewerMention: &no, Text: original, Complete: true})
	if e.Text != original || e.Summarized {
		t.Fatal("malformed report was changed", e)
	}
}

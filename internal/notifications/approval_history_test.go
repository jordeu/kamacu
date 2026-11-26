package notifications

import (
	"context"
	"fmt"
	"kamacu/internal/inbox"
	"kamacu/internal/store"
	"strings"
	"testing"
	"time"
)

func seedApprovalHistory(t *testing.T, s *Service, source Source, subject, reviewURL, actor, state string) {
	t.Helper()
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO notification_activity_subjects(source_id,subject_url,account) VALUES(?,?,'alice')`, source.ID, subject)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.DB.Exec(`INSERT INTO notification_activities(source_id,subject_url,resource_url,kind,digest,revision) VALUES(?,?,?,'review','dismissed',2)`, source.ID, subject, reviewURL)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	prior := SubjectEvidence{ID: reviewURL, URL: reviewURL, Kind: "review", Actor: actor, ReviewState: state, Text: "GitHub review state: " + state + ".", Complete: true}
	current := prior
	current.ReviewState = "DISMISSED"
	current.Text = "GitHub review state: DISMISSED."
	for i, e := range []SubjectEvidence{prior, current} {
		if _, err = s.DB.Exec(`INSERT INTO notification_activity_versions(activity_id,revision,digest,payload,context_only,fetched_at) VALUES(?,?,?,?,0,?)`, id, i+1, fmt.Sprint(i), mustJSON(e), time.Date(2026, 11, 26, 8, 26+i, 0, 0, time.UTC).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
}
func dismissedPRFacts() PRFacts {
	no := false
	return PRFacts{Repository: "acme/repo", Number: 7, URL: "https://github.com/acme/repo/pull/7", Viewer: "alice", Available: true, ReviewsComplete: true, ViewerEverApproved: &no, ViewerReviews: []ViewerReview{{ID: 71, State: "DISMISSED", CommitID: "approved-head", URL: "https://github.com/acme/repo/pull/7#pullrequestreview-71"}}}
}
func TestRecordedApprovalSurvivesDismissalWithStrictIdentityScope(t *testing.T) {
	for _, mode := range []string{"approved", "comment_only", "changes_requested", "wrong_actor", "wrong_subject", "wrong_review", "wrong_source", "wrong_account", "wrong_subject_account", "unavailable", "missing_history"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, source := fixture(t)
			seed(t, s, source, event())
			facts := dismissedPRFacts()
			subject, review, actor, state := facts.URL, facts.ViewerReviews[0].URL, "alice", "APPROVED"
			switch mode {
			case "comment_only":
				state = "COMMENTED"
			case "changes_requested":
				state = "CHANGES_REQUESTED"
			case "wrong_actor":
				actor = "bob"
			case "wrong_subject":
				subject = "https://github.com/acme/other/pull/7"
			case "wrong_review":
				review = "https://github.com/acme/repo/pull/7#pullrequestreview-72"
			}
			if mode != "missing_history" {
				seedApprovalHistory(t, s, source, subject, review, actor, state)
			}
			switch mode {
			case "wrong_source":
				source.ID = "a-different-source"
			case "wrong_account":
				if _, err := s.DB.Exec(`UPDATE notification_sources SET account='bob'`); err != nil {
					t.Fatal(err)
				}
			case "wrong_subject_account":
				if _, err := s.DB.Exec(`UPDATE notification_activity_subjects SET account='bob'`); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				if _, err := s.DB.Exec(`UPDATE notification_activity_versions SET payload=json_set(payload,'$.unavailable',json('true')) WHERE revision=1`); err != nil {
					t.Fatal(err)
				}
			}
			got := (GitHub{DB: s.DB}).WithRecordedApprovalHistory(context.Background(), source, facts)
			if got.ViewerReviews[0].State != "DISMISSED" {
				t.Fatal("current verdict was rewritten", got)
			}
			if mode == "approved" {
				if got.ViewerEverApproved == nil || !*got.ViewerEverApproved || !got.ViewerReviews[0].ApprovalPreviouslyObserved || !strings.HasSuffix(got.ViewerReviews[0].ApprovalEvidenceRef, "/V1") || got.ViewerReviews[0].ApprovalObservedAt == "" {
					t.Fatalf("lost original approval: %+v", got)
				}
				if facts.ViewerReviews[0].ApprovalPreviouslyObserved {
					t.Fatal("input history was mutated")
				}
			} else if got.ViewerEverApproved != nil || got.ViewerReviews[0].ApprovalPreviouslyObserved {
				t.Fatalf("unverified approval accepted: %+v", got)
			}
		})
	}
}
func TestPRContextAndInboxEvidenceRestoreHistoricalApproval(t *testing.T) {
	s, _, _, source := fixture(t)
	e := event()
	e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	e.Reason = "review_requested"
	id := seed(t, s, source, e)
	q := publish(t, s, id)
	facts := dismissedPRFacts()
	seedApprovalHistory(t, s, source, facts.URL, facts.ViewerReviews[0].URL, "alice", "APPROVED")
	g := GitHub{DB: s.DB, Run: func(_ context.Context, args ...string) ([]byte, error) {
		switch args[len(args)-1] {
		case "user":
			return response(`{"login":"alice"}`, ""), nil
		case "repos/acme/repo/pulls/7":
			return response(`{"id":7,"html_url":"https://github.com/acme/repo/pull/7","state":"open","draft":false,"head":{"sha":"nit-fixed"},"requested_reviewers":[{"login":"alice"}],"body":"Nit fixed","title":"Fix the nit"}`, ""), nil
		case "repos/acme/repo/pulls/7/reviews?per_page=100":
			return response(`[{"id":71,"user":{"login":"alice"},"state":"DISMISSED","commit_id":"approved-head","html_url":"https://github.com/acme/repo/pull/7#pullrequestreview-71"}]`, ""), nil
		default:
			return response(`[]`, ""), nil
		}
	}}
	source.Account = "alice"
	current := g.PRContext(context.Background(), source, "acme/repo", 7)
	if current.ViewerEverApproved == nil || !*current.ViewerEverApproved || current.ViewerReviews[0].State != "DISMISSED" || len(current.RequestedReviewers) != 1 {
		t.Fatal(current)
	}
	s.Providers["github"] = g
	raw, err := s.InboxEvidence(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"viewer_ever_approved":true`, `"state":"DISMISSED"`, `"approval_previously_observed":true`, `"approval_evidence_ref":"A`, `"requested_reviewers":["alice"]`} {
		if !strings.Contains(string(raw), expected) {
			t.Fatal("missing historical fact in inbox input", expected, string(raw))
		}
	}
	if err := store.PutPRLink(context.Background(), s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "review", Source: "test", Evidence: "Reviewing this PR", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	monitored, err := s.InboxEvidence(context.Background(), inbox.Question{Origin: "observer", TaskID: 1})
	if err != nil || !strings.Contains(string(monitored), `"viewer_ever_approved":true`) || !strings.Contains(string(monitored), `"state":"DISMISSED"`) {
		t.Fatal("review-session approval question lost its history", string(monitored), err)
	}
	// A generic PR-context lookup has no registered source argument; it must
	// still scope history to the authenticated viewer's registered GitHub source.
	generic := (GitHub{DB: s.DB}).WithRecordedApprovalHistory(context.Background(), Source{}, facts)
	if generic.ViewerEverApproved == nil || !*generic.ViewerEverApproved {
		t.Fatal(generic)
	}
}
func TestDismissalWithoutRecordedApprovalIsUnknown(t *testing.T) {
	f := (GitHub{}).WithRecordedApprovalHistory(context.Background(), Source{}, dismissedPRFacts())
	if f.ViewerEverApproved != nil {
		t.Fatal("dismissal was treated as a known negative", f)
	}
}

func TestHistoricalApprovalProofSurvivesCompaction(t *testing.T) {
	facts := dismissedPRFacts()
	yes := true
	facts.ViewerEverApproved = &yes
	facts.ViewerReviews[0].ApprovalPreviouslyObserved = true
	facts.ViewerReviews[0].ApprovalEvidenceRef = "A1/V1"
	for i := 0; i < 20; i++ {
		facts.ViewerReviews = append(facts.ViewerReviews, ViewerReview{ID: int64(100 + i), State: "COMMENTED", URL: fmt.Sprintf("https://github.com/acme/repo/pull/7#pullrequestreview-%d", 100+i)})
	}
	e := Event{PRFacts: &facts}
	compactFactDetails(&e, 1000)
	if e.PRFacts.ViewerEverApproved == nil || !*e.PRFacts.ViewerEverApproved || !e.PRFacts.ViewerReviewsTruncated {
		t.Fatal(e.PRFacts)
	}
	found := false
	for _, r := range e.PRFacts.ViewerReviews {
		if r.ID == 71 && r.State == "DISMISSED" && r.ApprovalEvidenceRef == "A1/V1" {
			found = true
		}
	}
	if !found {
		t.Fatal("compaction dropped the historical proof", e.PRFacts.ViewerReviews)
	}
}

func TestUnidentifiableDismissedReviewIsUnknown(t *testing.T) {
	for _, mode := range []string{"missing_id", "mismatched_url", "invalid_repository", "missing_number"} {
		t.Run(mode, func(t *testing.T) {
			facts := dismissedPRFacts()
			if mode == "invalid_repository" {
				facts.Repository = "invalid"
			}
			if mode == "missing_number" {
				facts.Number = 0
			}
			if mode == "missing_id" {
				facts.ViewerReviews[0].ID = 0
			} else {
				facts.ViewerReviews[0].URL = "https://github.com/acme/other/pull/7#pullrequestreview-71"
			}
			got := (GitHub{}).WithRecordedApprovalHistory(context.Background(), Source{}, facts)
			if got.ViewerEverApproved != nil {
				t.Fatal("unknown prior verdict was converted to no approval", got)
			}
		})
	}
}
func TestApprovalAccountMismatchCannotPreservePositiveFlag(t *testing.T) {
	facts := dismissedPRFacts()
	yes := true
	facts.ViewerEverApproved = &yes
	facts.ViewerReviews[0].ApprovalPreviouslyObserved = true
	facts.ViewerReviews[0].ApprovalEvidenceRef = "A1/V1"
	got := (GitHub{}).WithRecordedApprovalHistory(context.Background(), Source{Account: "bob"}, facts)
	if got.ViewerEverApproved != nil || got.ApprovalHistoryError == "" || got.Available || !got.Stale || got.ViewerReviews[0].ApprovalPreviouslyObserved || got.ViewerReviews[0].ApprovalEvidenceRef != "" {
		t.Fatal("mismatched account retained approval", got)
	}
	if !facts.ViewerReviews[0].ApprovalPreviouslyObserved {
		t.Fatal("input history was mutated")
	}

}
func TestRecordedApprovalMatchesCaseInsensitiveRepositoryURLs(t *testing.T) {
	s, _, _, source := fixture(t)
	seed(t, s, source, event())
	facts := dismissedPRFacts()
	subject := "https://github.com/ACME/Repo/pull/7"
	review := subject + "#pullrequestreview-71"
	facts.ViewerReviews[0].URL = review
	seedApprovalHistory(t, s, source, subject, review, "ALICE", "APPROVED")
	got := (GitHub{DB: s.DB}).WithRecordedApprovalHistory(context.Background(), source, facts)
	if got.ViewerEverApproved == nil || !*got.ViewerEverApproved || got.ViewerReviews[0].State != "DISMISSED" {
		t.Fatal("repository URL casing lost approval history", got)
	}
}

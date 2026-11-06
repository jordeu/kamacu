package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInvalidCitationIsCorrectedOnceWithoutWeakeningEvidence(t *testing.T) {
	for _, corrected := range []bool{false, true} {
		t.Run(fmt.Sprint(corrected), func(t *testing.T) {
			s, model, _, _ := fixture(t)
			e := event()
			invalid := intentResponse{Findings: []intentFinding{{Intent: "direct_question", EvidenceID: "context", Quote: "invented question"}}}
			second := invalid
			if corrected {
				second.Findings = []intentFinding{{Intent: "direct_question", EvidenceID: "context", Quote: e.Context}}
			}
			model.responses = []any{invalid, second, second}
			subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}}
			err := s.classifySubject(context.Background(), subject, "")
			if (err == nil) != corrected || len(model.prompts) != 2 || !strings.Contains(model.prompts[1], "CORRECTION") {
				t.Fatalf("unexpected repair outcome: err=%v calls=%d", err, len(model.prompts))
			}
		})
	}
}

func TestRepeatedIntentRetainsCurrentEvidenceAfterRecencyChecks(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	e.Version = "2026-11-04T12:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{
		{ID: "old", Text: "Old question?", Complete: true},
		{ID: "new", Text: "Current question?", Complete: true, Current: true, UpdatedAt: e.Version},
	}
	model.responses = []any{intentResponse{Findings: []intentFinding{
		{Intent: "direct_question", EvidenceID: "old", Quote: "Old question?"},
		{Intent: "direct_question", EvidenceID: "new", Quote: "Current question?"},
	}}}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, viewer: "alice"}
	if err := s.classifySubject(context.Background(), subject, ""); err != nil {
		t.Fatal(err)
	}
	for _, finding := range subject.findings {
		if finding.Intent == "direct_question" && finding.EvidenceID == "new" {
			return
		}
	}
	t.Fatalf("lost current question: %+v", subject.findings)
}

func TestMergedThreadNewFeedbackIsClassifiedBeforeReviewSuppression(t *testing.T) {
	s, model, _, source := fixture(t)
	e := event()
	e.Reason = "review_requested"
	seed(t, s, source, e)
	s.Providers["github"] = &changingFactsProvider{fakeProvider: &fakeProvider{contextText: "Please investigate this regression before responding."}, facts: PRFacts{Available: true, State: "merged"}}
	status := process(t, s, model, "review_feedback")
	if status.Status != "completed" || status.ActionCount != 1 || len(model.prompts) != 2 {
		t.Fatalf("lost new feedback: %+v calls=%d", status, len(model.prompts))
	}
	page, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if !strings.Contains(page.Items[0].Context, "regression") {
		t.Fatal("lost feedback context")
	}
}

func TestCurrentIssueBodyRemainsInClassifierInput(t *testing.T) {
	for _, knownCreation := range []bool{true, false} {
		t.Run(fmt.Sprint(knownCreation), func(t *testing.T) {
			s, model, _, _ := fixture(t)
			respondWithIntent(model, "direct_question")
			e := event()
			e.SubjectType = "Issue"
			e.URL = "https://github.com/acme/repo/issues/7"
			e.Version = "2026-11-04T12:00:00Z"
			e.EvidenceComplete = true
			body := SubjectEvidence{ID: e.URL, URL: e.URL, Text: "What do you think of this design?", UpdatedAt: e.Version, Complete: true}
			if knownCreation {
				body.CreatedAt = e.Version
			}
			e.Evidence = []SubjectEvidence{body}
			subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, viewer: "alice"}
			if err := s.classifySubject(context.Background(), subject, ""); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(model.prompts[0], body.Text) {
				t.Fatal("new issue body omitted")
			}
			var payload struct {
				Subjects []struct {
					Complete bool `json:"context_complete"`
				} `json:"subjects"`
			}
			raw := strings.Split(model.prompts[0], projectEvidenceMarker)[1]
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Subjects[0].Complete != knownCreation {
				t.Fatal("uncertain body activity was declared complete")
			}
			want := "needs_context"
			if knownCreation {
				want = "direct_question"
			}
			if len(subject.findings) != 1 || subject.findings[0].Intent != want {
				t.Fatal("incorrect issue activity scope", subject.findings)
			}
		})
	}
}

func TestClippedUnknownIssueBodyCannotBeDiscardedAsRoutine(t *testing.T) {
	s, model, _, _ := fixture(t)
	respondWithIntent(model, "routine")
	e := event()
	e.SubjectType = "Issue"
	e.Version = "2026-11-04T12:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: e.URL, URL: e.URL, Text: "Please investigate the issue", UpdatedAt: e.Version, Complete: true}}
	subject := &triageSubject{fresh: e, events: []RawNotification{{Event: e}}, viewer: "alice"}
	if err := s.classifySubject(context.Background(), subject, ""); err != nil {
		t.Fatal(err)
	}
	if len(subject.findings) != 1 || subject.findings[0].Intent != "needs_context" {
		t.Fatal("uncertain body activity was silently discarded")
	}
}

func TestSubjectCandidatesFitWithAllLinksAndRevisions(t *testing.T) {
	for _, count := range []int{12, 160} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s, _, tools, _ := fixture(t)
			tools.result = `[]`
			var subjects []*triageSubject
			var events []RawNotification
			for i := 0; i < count; i++ {
				e := event()
				e.Repository = "acme/" + strings.Repeat("r", 100)
				e.URL = fmt.Sprintf("https://github.com/%s/pull/%d", e.Repository, i+1)
				first := RawNotification{ID: int64(i*2 + 1), Event: e}
				second := first
				second.ID++
				events = append(events, first, second)
				subjects = append(subjects, &triageSubject{fresh: e, events: []RawNotification{first, second}, findings: []intentFinding{{Intent: "review_request", Quote: strings.Repeat("界", 666)}}})
			}
			d, err := s.renderTriage(context.Background(), subjects, nil, RuleSet{})
			if err != nil {
				t.Fatal(err)
			}
			actions, _, err := validateBatch(d, events, nil)
			if err != nil {
				t.Fatal("valid review group failed validation", err)
			}
			if len(actions) != count {
				t.Fatal("ordinary group was split", len(actions))
			}
			if count == 160 && len(actions) != count {
				t.Fatal("oversized link group was not split")
			}
			covered := map[int64]bool{}
			for _, action := range actions {
				if len(action.Request.Context) > inboxCardContextLimit {
					t.Fatal("oversized inbox context")
				}
				for _, id := range action.EventIDs {
					if covered[id] {
						t.Fatal("duplicate provenance", id)
					}
					covered[id] = true
				}
				for _, subject := range subjects {
					first, second := subject.events[0].ID, subject.events[1].ID
					a, b := false, false
					for _, id := range action.EventIDs {
						a = a || id == first
						b = b || id == second
					}
					if a != b {
						t.Fatal("split revisions of the same subject")
					}
					linked := strings.Contains(action.Request.Context, "\n- "+subject.fresh.URL+" (")
					if linked != a {
						t.Fatal("links disagree with this card's scope", subject.fresh.URL)
					}
				}
			}
			if len(covered) != len(events) {
				t.Fatal("lost event provenance")
			}
		})
	}
}

func TestBodyPointerDoesNotMakeAnOldBodyMentionCurrent(t *testing.T) {
	e := event()
	e.Version = "2026-11-04T12:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: e.URL, URL: e.URL, Text: "@alice old question", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: e.Version, Current: true, Complete: true}}
	if mention := mentionStatus([]RawNotification{{Event: e}}, e, "alice"); mention != nil {
		t.Fatal("subject updated_at or pointer was treated as a fresh body mention")
	}
}

func TestChangedEvidenceCannotBeRevalidatedBackToOldClassification(t *testing.T) {
	previous := event()
	previous.Context = "Old request"
	previous.EvidenceComplete = true
	subject := &triageSubject{viewer: "alice", events: []RawNotification{{Event: previous}}, fresh: previous, findings: []intentFinding{{Intent: "review_request"}}, classifiedFindings: []intentFinding{{Intent: "review_request"}}}
	subject.fresh.Context = "New unrelated question"
	refreshIntentRules(subject, previous, RuleSet{})
	unchanged := subject.fresh
	refreshIntentRules(subject, unchanged, RuleSet{})
	if len(subject.findings) != 1 || subject.findings[0].Intent != "needs_context" {
		t.Fatal("revived old classification after an evidence change")
	}
}

func TestNotificationTimestampLagDoesNotHideRecentQuestion(t *testing.T) {
	e := event()
	e.Version = "2026-11-04T12:00:05Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{{ID: "recent-comment", URL: e.URL + "#issuecomment-2", Text: "Please check this before responding", UpdatedAt: "2026-11-04T12:00:00Z", Current: true, Complete: true}}
	evidence, complete := classifierEvidence([]RawNotification{{Event: e}}, e)
	if len(evidence) != 1 || evidence[0].Text != e.Evidence[0].Text || complete {
		t.Fatal("notification delivery lag discarded a recent question or claimed certainty")
	}
}

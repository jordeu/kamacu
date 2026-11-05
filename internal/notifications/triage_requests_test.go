package notifications

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"kamacu/internal/settings"
)

func TestReviewAssignmentRequiresVerifiedCurrentIdentity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts PRFacts
	}{
		{"unavailable", PRFacts{State: "open", Viewer: "alice", RequestedReviewers: []string{"alice"}}},
		{"stale", PRFacts{Available: true, Stale: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"alice"}}},
		{"closed", PRFacts{Available: true, State: "closed", Viewer: "alice", RequestedReviewers: []string{"alice"}}},
		{"different account", PRFacts{Available: true, State: "open", Viewer: "bob", RequestedReviewers: []string{"alice"}}},
		{"team only", PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"team:reviewers"}}},
		{"sticky notification only", PRFacts{Available: true, State: "open", Viewer: "alice"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := event()
			e.Reason, e.PRFacts = "review_requested", &tc.facts
			if currentReviewRequestEvidence(&triageSubject{viewer: "alice", fresh: e}) != nil {
				t.Fatal("unverified review request promoted")
			}
		})
	}
}

func TestRemovedReviewerInvalidatesPreparedClassification(t *testing.T) {
	e := event()
	e.PRFacts = &PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"alice"}}
	fresh := e
	facts := *e.PRFacts
	facts.RequestedReviewers = []string{}
	fresh.PRFacts = &facts
	finding := intentFinding{Intent: "review_request", EvidenceID: "current-review-request"}
	subject := &triageSubject{viewer: "alice", fresh: fresh, events: []RawNotification{{Event: e}}, findings: []intentFinding{finding}, classifiedFindings: []intentFinding{finding}}
	refreshIntentPolicy(subject, e, settings.NotificationPolicy{})
	if !subject.classificationStale || len(subject.findings) != 1 || subject.findings[0].Intent != "needs_context" {
		t.Fatalf("removed reviewer assignment was kept: %+v", subject.findings)
	}
}

func TestReplyDoesNotPublishInventedToolAccessFailure(t *testing.T) {
	for _, repeats := range []bool{false, true} {
		t.Run(fmt.Sprint(repeats), func(t *testing.T) {
			s, model, tools, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Check the existing sessions", ""); err != nil {
				t.Fatal(err)
			}
			bad := Step{Action: "ask", Summary: "Could not run your request: the Kamacu MCP tools (list_sessions, get_task, start_task_agent) are not available to this worker.", Context: "No such tool available.", Question: "How should I proceed now that the Kamacu MCP tools are unavailable to this worker?"}
			model.steps = []Step{bad, {Action: "tool", ToolName: "list_sessions", Arguments: `{"project_id":1}`}, {Action: "done", Message: "Checked existing sessions."}}
			if repeats {
				model.steps = []Step{bad, bad}
			}
			if err := s.executeNext(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if repeats {
				if got.DeliveryStatus != "failed" || len(tools.calls) != 0 {
					t.Fatalf("invented access failure published or executed: %+v", got)
				}
			} else if got.DeliveryStatus != "succeeded" || len(tools.calls) != 1 || tools.calls[0] != "list_sessions" {
				t.Fatalf("valid next tool action was not executed: %+v, %v", got, tools.calls)
			}
			if !strings.Contains(model.prompts[1], "APPLICATION CORRECTION") {
				t.Fatal("missing correction of the invented failure")
			}
		})
	}
}

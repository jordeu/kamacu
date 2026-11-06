package notifications

import (
	"context"
	"fmt"
	"strings"
	"testing"
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
	refreshIntentRules(subject, e, RuleSet{})
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
			if !strings.Contains(model.prompts[len(model.prompts)-1], "APPLICATION CORRECTION") {
				t.Fatal("missing correction of the invented failure")
			}
		})
	}
}

func TestImplementationAgentRequiresFollowupDelivery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session string
		ask     bool
		want    string
	}{
		{name: "done without instructions", want: "uncertain"},
		{name: "ask without instructions", ask: true, want: "uncertain"},
		{name: "instructions to another task", session: "other", want: "uncertain"},
		{name: "instructions delivered", session: "correct", want: "succeeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, model, tools, source := fixture(t)
			ctx := context.Background()
			q := publish(t, s, seed(t, s, source, event()))
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Resume the implementation agent to check new comments and commits", ""); err != nil {
				t.Fatal(err)
			}
			model.steps = []Step{{Action: "tool", ToolName: "start_task_agent", Arguments: `{"task_id":1,"resume":true}`}}
			if tc.session != "" {
				model.steps = append(model.steps, Step{Action: "tool", ToolName: "send_session_message", Arguments: fmt.Sprintf(`{"session_id":%q,"message":"Check new comments and commits on https://github.com/acme/repo/pull/7 and handle the feedback"}`, tc.session)})
			}
			last := Step{Action: "done", Message: "Follow-up handed to the task agent."}
			if tc.ask {
				last = ask()
			}
			model.steps = append(model.steps, last)
			if err := s.executeNext(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.DeliveryStatus != tc.want {
				t.Fatalf("got %s, want %s: %s", got.DeliveryStatus, tc.want, got.ExecutionResult)
			}
			if tc.want == "uncertain" && !strings.Contains(got.ExecutionResult, "follow-up instructions were not sent") && !strings.Contains(got.ExecutionResult, "different project") {
				t.Fatalf("missing handoff failure: %s", got.ExecutionResult)
			}
			if tc.session == "correct" && (len(tools.calls) != 2 || tools.calls[1] != "send_session_message") {
				t.Fatalf("missing delivery: %v", tools.calls)
			}
		})
	}
}

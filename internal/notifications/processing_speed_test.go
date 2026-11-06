package notifications

import (
	"context"
	"fmt"
	"kamacu/internal/store"
	"strings"
	"testing"
	"time"
)

func TestGitHubReadCacheIsPhaseScopedAndExcludesIdentity(t *testing.T) {
	calls := map[string]int{}
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		p := args[len(args)-1]
		calls[p]++
		return response(fmt.Sprintf(`{"read":%d}`, calls[p]), ""), nil
	}}
	phase := withGitHubReadCache(context.Background())
	for i := 0; i < 2; i++ {
		raw, _, err := g.api(phase, "github.com", "repos/acme/repo/pulls/7")
		if err != nil || string(raw) != `{"read":1}` {
			t.Fatal(string(raw), err)
		}
	}
	fresh := freshGitHubReadCache(phase)
	raw, _, err := g.api(fresh, "github.com", "repos/acme/repo/pulls/7")
	if err != nil || string(raw) != `{"read":2}` {
		t.Fatal("revalidation reused old facts", string(raw), err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err = g.api(phase, "github.com", "user"); err != nil {
			t.Fatal(err)
		}
	}
	if calls["user"] != 2 {
		t.Fatal("identity was cached")
	}
	canceled, cancel := context.WithCancel(phase)
	cancel()
	if _, _, err = g.api(canceled, "github.com", "repos/acme/repo/pulls/7"); err == nil {
		t.Fatal("cached read ignored cancellation")
	}
}

func TestSessionRoutingConditionUsesFactsWithoutModel(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		stale       bool
		want        string
	}{
		{"active", "working", false, "false"}, {"exited", "exited", false, "unknown"}, {"stale", "idle", true, "unknown"}, {"uncertain", "unknown", false, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, model, _, _ := fixture(t)
			owner := int64(1)
			e := event()
			e.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: owner, Role: "implementation"}}}
			e.RelatedSessions = []store.SessionWork{{SessionID: "session", TaskID: &owner, State: tc.state, StateStale: tc.stale, StateObservedAt: time.Now()}}
			f := intentFinding{Intent: "direct_question", EvidenceID: "context", Quote: "Please check"}
			subject := &triageSubject{projectID: 1, fresh: e, events: []RawNotification{{Event: e}}, findings: []intentFinding{f}}
			set := RuleSet{Rules: []NotificationRule{{ID: "R005", Enabled: true, Clauses: []RuleClause{{ID: "R005.1", Category: "action", Effect: "route_investigation", OnUnknown: "hold", When: RuleExpression{Op: "semantic", Text: sessionRoutingCondition}}}}}}
			if got := evalExpression(set.Rules[0].Clauses[0].When, subjectRuleFacts(subject, &f), nil); got != tc.want {
				t.Fatal(got, tc.want)
			}
			if err := s.evaluateActivities(context.Background(), []*triageSubject{subject}, set); err != nil {
				t.Fatal(err)
			}
			if len(model.prompts) != 0 {
				t.Fatal("session availability used a model")
			}
			if tc.want == "unknown" && subject.decisions[0].Outcome != "held" {
				t.Fatal(subject.decisions)
			}
		})
	}
}

func TestWriterPromptOmitsProjectCatalogAndUnadmittedBodies(t *testing.T) {
	s, model, _, source := fixture(t)
	e := event()
	e.Context = strings.Repeat("UNADMITTED BODY ", 10000)
	e.Evidence = []SubjectEvidence{{ID: "comment", Actor: "reviewer", Text: "Please check this change. " + e.Context, Complete: true}}
	f := intentFinding{Intent: "direct_question", EvidenceID: "comment", Quote: "Please check this change."}
	subject := &triageSubject{projectID: 1, fresh: e, events: []RawNotification{{ID: 1, SourceID: source.ID, Event: e}}, findings: []intentFinding{f}}
	decision, err := s.renderTriage(context.Background(), []*triageSubject{subject}, nil, RuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	size := 0
	model.respond = func(prompt string) (any, error) {
		size = len(prompt)
		if strings.Contains(prompt, "UNADMITTED BODY") || strings.Contains(prompt, "project_work") {
			t.Fatal("writer received unrelated context")
		}
		if !strings.Contains(prompt, f.Quote) || !strings.Contains(prompt, "route_investigation") {
			t.Fatal("writer lost citation or plan")
		}
		return fixtureProjectCards(prompt, nil)
	}
	if err = s.draftProjectCards(context.Background(), &decision, []*triageSubject{subject}, RuleSet{}, ""); err != nil {
		t.Fatal(err)
	}
	if size > 10000 {
		t.Fatal("unexpected writer size", size)
	}
	t.Logf("writer prompt: %d bytes with %d bytes of original discussion", size, len(e.Context))
}

func TestRoutingFindsNestedSessionsAndDoesNotInferAbsence(t *testing.T) {
	owner := int64(1)
	e := event()
	e.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: owner, Role: "implementation"}, Sessions: []store.SessionWork{{SessionID: "nested", TaskID: &owner, State: "idle", StateObservedAt: time.Now()}}}}
	f := sessionRoutingFact(e)
	if !f.Known || f.Value != false {
		t.Fatal("nested session was ignored", f)
	}
	e.RelatedWork[0].Sessions = nil
	if f = sessionRoutingFact(e); f.Known {
		t.Fatal("bounded history was treated as proof of absence", f)
	}
}

func TestRoutingRefreshUsesLiveManagerEvenWithoutGitHubChanges(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "implementation", Source: "explicit", Evidence: "implementation owner", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	live := true
	s.TaskHasSession = func(id int64) bool { return id == 1 && live }
	e := event()
	if err := s.refreshRoutingContext(ctx, &e); err != nil {
		t.Fatal(err)
	}
	if f := sessionRoutingFact(e); !f.Known || f.Value != false {
		t.Fatal(f)
	}
	live = false
	subject := &triageSubject{fresh: e, events: []RawNotification{{SourceID: source.ID, Event: e}}}
	if err := s.revalidateNotificationSubject(ctx, subject, map[string]Source{source.ID: source}, RuleSet{}); err != nil {
		t.Fatal(err)
	}
	if f := sessionRoutingFact(subject.fresh); !f.Known || f.Value != true {
		t.Fatal("session fact was not refreshed", f)
	}
}

func TestReviewSessionRoutingUsesReviewOwner(t *testing.T) {
	s, _, _, _ := fixture(t)
	e := event()
	e.RelatedWork = []store.RelatedTask{{Link: store.PRLink{TaskID: 1, Role: "implementation"}}, {Link: store.PRLink{TaskID: 2, Role: "review"}}}
	s.TaskHasSession = func(id int64) bool { return id == 2 }
	implementation := s.routingForRole(e, "implementation")
	review := s.routingForRole(e, "review")
	e.SessionRouting = &implementation
	e.ReviewSessionRouting = &review
	f := intentFinding{Intent: "review_request"}
	if got := subjectRuleFacts(&triageSubject{fresh: e}, &f)["workflow.needs_session"]; !got.Known || got.Value != false {
		t.Fatal(got)
	}
	if !implementation.Known || implementation.Value != true {
		t.Fatal(implementation)
	}
}

func TestWriterKeepsAuthorAndLatestReviewStateWithoutBodies(t *testing.T) {
	s, model, _, source := fixture(t)
	e := event()
	e.Evidence = []SubjectEvidence{
		{ID: "body", URL: e.URL, Actor: "alice", Text: "UNRELATED DESCRIPTION"},
		{ID: "approval", Actor: "reviewer", ReviewState: "APPROVED", UpdatedAt: "2026-11-06T01:00:00Z", Text: "Approved"},
		{ID: "changes", Actor: "reviewer", ReviewState: "CHANGES_REQUESTED", UpdatedAt: "2026-11-06T02:00:00Z", Text: "UNADMITTED REVIEW BODY"},
	}
	f := intentFinding{Intent: "review_feedback", EvidenceID: "approval", Quote: "Approved"}
	subject := &triageSubject{projectID: 1, fresh: e, events: []RawNotification{{ID: 1, SourceID: source.ID, Event: e}}, findings: []intentFinding{f}}
	decision, err := s.renderTriage(context.Background(), []*triageSubject{subject}, nil, RuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	model.respond = func(prompt string) (any, error) {
		if !strings.Contains(prompt, `"author":"alice"`) || !strings.Contains(prompt, `"review_state":"CHANGES_REQUESTED"`) {
			t.Fatal("writer lost ownership or latest review state")
		}
		if strings.Contains(prompt, "UNRELATED DESCRIPTION") || strings.Contains(prompt, "UNADMITTED REVIEW BODY") {
			t.Fatal("writer received unadmitted bodies")
		}
		return fixtureProjectCards(prompt, nil)
	}
	if err = s.draftProjectCards(context.Background(), &decision, []*triageSubject{subject}, RuleSet{}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestWriterCorrectionRespectsSmallPromptBudget(t *testing.T) {
	prompt := strings.Repeat("p", 30000)
	corrected := projectCorrectionPrompt(prompt, fmt.Errorf("%s", strings.Repeat("error", 1000)), []byte(strings.Repeat("invalid", 2000)), "Return complete valid JSON", 32000)
	if len(corrected) > 32000 || !strings.HasPrefix(corrected, prompt) {
		t.Fatal("correction exceeded budget or dropped admitted input", len(corrected))
	}
}

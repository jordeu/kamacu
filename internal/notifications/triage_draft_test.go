package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/settings"
)

func TestPolicyIgnoreDoesNotBecomeInspectionBecauseBodyRecencyIsUnknown(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		t.Run(map[bool]string{false: "retrieved", true: "missing evidence"}[incomplete], func(t *testing.T) {
			s, model, _, _ := fixture(t)
			e := event()
			e.Version = "2026-11-04T12:00:00Z"
			e.EvidenceComplete = !incomplete
			e.Evidence = []SubjectEvidence{{ID: e.URL, URL: e.URL, Text: "A change to the service", UpdatedAt: e.Version, Complete: true}}
			const preference = "Ignore acme/repo unless directly mentioned."
			model.responses = []any{intentResponse{Findings: []intentFinding{
				{Intent: "policy_ignore", EvidenceID: e.URL, Quote: "A change", PolicyQuote: preference},
				{Intent: "routine", EvidenceID: e.URL, Quote: "service"},
			}}}
			subject := &triageSubject{fresh: e, events: []RawNotification{{ID: 1, Event: e}}, viewer: "alice"}
			if err := s.classifySubject(context.Background(), subject, preference); err != nil {
				t.Fatal(err)
			}
			decision, err := s.renderTriage(context.Background(), []*triageSubject{subject}, nil, settings.NotificationPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			if incomplete && len(decision.Actions) != 1 || !incomplete && len(decision.Actions) != 0 {
				t.Fatalf("unexpected policy outcome: %+v, %+v", subject.findings, decision)
			}
			if !strings.Contains(model.prompts[0], `"viewer":"alice"`) || !strings.Contains(model.prompts[0], `"retrieval_complete":`) {
				t.Fatal("classifier did not receive recipient and retrieval facts")
			}
		})
	}
}

func TestTriageDraftIncludesDiscussionAndPreservesScope(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	e.Title = "Multi-region compute ADR"
	e.Version = "2026-11-04T12:00:00Z"
	e.EvidenceComplete = true
	e.Evidence = []SubjectEvidence{
		{ID: "reply", Text: "I updated the ADR after review.", UpdatedAt: e.Version, Complete: true},
		{ID: "decision", Text: "Should resilience come before cost? The cost gate saves 0.43%.", UpdatedAt: e.Version, Complete: true},
	}
	subject := &triageSubject{fresh: e, viewer: "alice", events: []RawNotification{{ID: 7, Event: e}}, findings: []intentFinding{{Intent: "needs_context", EvidenceID: "reply", Quote: e.Evidence[0].Text}}}
	decision, err := s.renderTriage(context.Background(), []*triageSubject{subject}, nil, settings.NotificationPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	model.respond = func(prompt string) (any, error) {
		if !strings.Contains(prompt, "The cost gate saves 0.43%.") || !strings.Contains(prompt, "Investigate before answering") {
			t.Fatal("draft only received the first quote or lost user preferences")
		}
		return triageDraft{Summary: "Multi-region ADR needs a resilience-versus-cost decision", Context: "The author asks whether resilience should lead the milestone.", Question: "Should I investigate resilience-first placement before drafting your response?", Options: []string{"Create a task to investigate resilience-first placement", "Draft a response about the milestone tradeoffs"}}, nil
	}
	if err := s.draftTriageCards(context.Background(), &decision, []*triageSubject{subject}, "Investigate before answering"); err != nil {
		t.Fatal(err)
	}
	a := decision.Actions[0]
	if len(a.EventIDs) != 1 || a.EventIDs[0] != 7 || len(a.Options) != 3 || a.Options[2] != "Leave this activity alone" || !strings.Contains(a.Question, "resilience-first") || !strings.Contains(a.Context, e.URL) || !strings.Contains(a.Context, e.Evidence[0].Text) {
		t.Fatalf("lost scope, sources or specific options: %+v", a)
	}
	if !isTriageQuestion(inbox.Question{Question: a.Question, Context: a.Context}, "needs_context") {
		t.Fatal("personalized question lost template identity")
	}
	// The same source evidence must still deduplicate after personalizing prose.
	if _, err := decodeTriageDraft([]byte(`{"summary":"x","context":"x","question":"x","options":["x"],"discard_ids":[7]}`)); err == nil {
		t.Fatal("drafting was allowed to change disposition")
	}
}

func TestDraftFailurePreservesScopeWithoutPublishingFallback(t *testing.T) {
	s, model, _, _ := fixture(t)
	e := event()
	e.Title = "Retry capacity fallback"
	subject := &triageSubject{fresh: e, events: []RawNotification{{ID: 7, Event: e}}}
	decision := BatchDecision{Actions: []BatchAction{{EventIDs: []int64{7}, Question: triageQuestions["needs_context"], Context: e.URL}}}
	model.respond = func(string) (any, error) { return nil, errors.New("model unavailable") }
	if err := s.draftTriageCards(context.Background(), &decision, []*triageSubject{subject}, ""); err == nil {
		t.Fatal("failed drafting was accepted")
	}
	if len(decision.Actions) != 1 || decision.Actions[0].EventIDs[0] != 7 {
		t.Fatal("failed drafting changed scope")
	}
	if raw, _ := json.Marshal(decision); !strings.Contains(string(raw), e.URL) {
		t.Fatal("fallback lost source URL")
	}
}

func TestPersonalizedCardsDeduplicateAndDismissWithoutExecution(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	respondWithIntent(model, "direct_question")
	classify := model.respond
	model.respond = func(prompt string) (any, error) {
		if !strings.HasPrefix(prompt, projectDraftPrompt) {
			return classify(prompt)
		}
		return triageDraft{Summary: "Decide how to investigate capacity retries", Context: "The reviewer asks about capacity retry behavior.", Question: "Should I investigate capacity retries before drafting a response?", Options: []string{"Investigate capacity retries in a task", "Draft a response about capacity retries"}}, nil
	}
	seed(t, s, source, event())
	if status := process(t, s, model, "direct_question"); status.Status != "completed" || status.ActionCount != 1 {
		t.Fatal(status)
	}
	page, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	q := page.Items[0]
	if !strings.Contains(q.Question, "capacity retries") {
		t.Fatal("personalized draft was not published")
	}
	update := event()
	update.Version = "v2"
	seed(t, s, source, update)
	if status := process(t, s, model, "direct_question"); status.Status != "completed" || status.ActionCount != 0 {
		t.Fatalf("personalized question duplicated: %+v", status)
	}
	before := len(model.prompts)
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "triage_noop"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if len(model.prompts) != before || len(tools.calls) != 0 {
		t.Fatal("dismissing a drafted card invoked execution")
	}
}

func TestConcreteQuestionReplacesEarlierInspectionCard(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if status := process(t, s, model, "needs_context"); status.Status != "completed" {
		t.Fatal(status)
	}
	before, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	update := event()
	update.Version = "v2"
	seed(t, s, source, update)
	respondWithIntent(model, "direct_question")
	if status := process(t, s, model, "direct_question"); status.Status != "completed" || status.ActionCount != 1 {
		t.Fatal(status)
	}
	old, err := s.Inbox.Get(ctx, before.Items[0].ID)
	if err != nil || old.Status == "pending" {
		t.Fatalf("old generic inspection card survived a concrete next step: %+v, %v", old, err)
	}
}

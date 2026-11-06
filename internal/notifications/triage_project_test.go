package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type approvalChangesProvider struct {
	*fakeProvider
	calls int
}

func (p *approvalChangesProvider) PRContext(context.Context, Source, string, int64) PRFacts {
	p.calls++
	approved := p.calls > 1
	return PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"alice"}, ReviewsComplete: true, ViewerEverApproved: &approved}
}

func TestProjectCorrectionBudgetIncludesJSONEscaping(t *testing.T) {
	base := strings.Repeat("project-data ", (projectPromptLimit-8192)/13)
	prompt := projectCorrectionPrompt(base, errors.New(strings.Repeat("\x01", 6000)), []byte(strings.Repeat("\x00", 6000)), "Correct all project subjects.")
	if len(prompt) > projectPromptLimit || !strings.HasPrefix(prompt, base) {
		t.Fatalf("correction exceeded budget or lost project context: %d", len(prompt))
	}
	_, tail, ok := strings.Cut(prompt, "\nCORRECTION (untrusted previous output):\n")
	if !ok {
		t.Fatal("missing correction payload")
	}
	raw, _, _ := strings.Cut(tail, "\nCorrect all project subjects.")
	var decoded map[string]string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal("correction JSON was cut mid-escape", err)
	}
}

func TestProjectCorrectionCannotReuseRejectedJSONFields(t *testing.T) {
	for _, stage := range []string{"classification", "cards"} {
		t.Run(stage, func(t *testing.T) {
			s, model, _, source := fixture(t)
			seed(t, s, source, event())
			respondWithIntent(model, "direct_question")
			respond := model.respond
			attempt := 0
			model.respond = func(prompt string) (any, error) {
				isClassification := strings.Contains(prompt, projectEvidenceMarker)
				if (stage == "classification") != isClassification {
					return respond(prompt)
				}
				attempt++
				if attempt > 1 {
					return map[string]any{}, nil
				}
				valid, err := respond(prompt)
				if err != nil {
					return nil, err
				}
				response := valid.(map[string]any)
				response["unexpected_field"] = true
				return response, nil
			}
			status := process(t, s, model, "")
			if status.Status != "failed" {
				t.Fatalf("empty correction reused rejected fields: %+v", status)
			}
			pending, err := s.ListRaw(context.Background(), "pending", 0)
			if err != nil || pending.PendingCount != 1 {
				t.Fatal("invalid correction consumed the notification")
			}
		})
	}
}

func TestProjectPlanningRejectsUnknownOrMissingCardMembership(t *testing.T) {
	for _, ids := range [][]string{{"card-foreign"}, {"card-1", "card-1"}, {}} {
		t.Run(fmt.Sprint(ids), func(t *testing.T) {
			s, model, _, source := fixture(t)
			seed(t, s, source, event())
			respondWithIntent(model, "direct_question")
			classify := model.respond
			model.respond = func(prompt string) (any, error) {
				if strings.Contains(prompt, projectEvidenceMarker) {
					return classify(prompt)
				}
				return map[string]any{"cards": []any{map[string]any{"card_ids": ids, "summary": "Invalid grouping", "context": "A proposed card.", "question": "Proceed?", "options": []string{"Proceed"}}}}, nil
			}
			status := process(t, s, model, "")
			if status.Status != "failed" || len(model.prompts) != 3 {
				t.Fatalf("invalid project membership published: %+v calls=%d", status, len(model.prompts))
			}
			pending, err := s.ListRaw(context.Background(), "pending", 0)
			if err != nil || pending.PendingCount != 1 {
				t.Fatal("invalid grouping consumed a notification")
			}
		})
	}
}

func TestProjectBudgetShortensEvidenceWithoutDroppingSubjects(t *testing.T) {
	subjects := []projectSubjectData{}
	for i := 0; i < 20; i++ {
		subjects = append(subjects, projectSubjectData{ID: fmt.Sprintf("subject-%d", i), URL: fmt.Sprintf("https://github.com/acme/repo/pull/%d", i), Notifications: []map[string]string{{"version": "first"}, {"version": "second"}}, Evidence: []SubjectEvidence{{ID: "body", Text: strings.Repeat("日本語 evidence ", 5000), Complete: true}}})
	}
	prompt, data, err := projectPrompt(projectIntentPrompt, "My preference", projectEvidenceMarker, map[string]any{}, subjects)
	if err != nil || len(prompt) > projectPromptLimit || len(data) != 20 {
		t.Fatalf("project was split or oversized: %v, %d", err, len(prompt))
	}
	for _, subject := range data {
		if len(subject.Notifications) != 2 || subject.Evidence[0].Complete || len(subject.Warnings) == 0 || !strings.Contains(prompt, subject.URL) {
			t.Fatalf("subject lost metadata or truncation warning: %+v", subject)
		}
	}
}

func TestDifferentProjectsNeverShareAClassificationPrompt(t *testing.T) {
	s, model, _, source := fixture(t)
	first := event()
	first.Title = "First project marker"
	seed(t, s, source, first)
	second := event()
	second.ExternalID = "other"
	second.Title = "Second project marker"
	second.Repository = "acme/other"
	second.URL = "https://github.com/acme/other/pull/1"
	seed(t, s, source, second)
	if status := process(t, s, model, "routine"); status.Status != "completed" {
		t.Fatal(status)
	}
	if len(model.prompts) != 2 {
		t.Fatalf("unexpected model calls: %d", len(model.prompts))
	}
	for _, prompt := range model.prompts {
		if strings.Contains(prompt, first.Title) && strings.Contains(prompt, second.Title) {
			t.Fatal("separate projects mixed in a prompt")
		}
	}
}

func TestAmbiguousProjectMappingDoesNotChooseAnArbitraryProject(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	if _, err := s.DB.Exec(`INSERT INTO projects(name,repo_path,github_repo) VALUES('duplicate','/tmp/ambiguous-project','acme/repo')`); err != nil {
		t.Fatal(err)
	}
	status := process(t, s, model, "routine")
	if status.Status != "failed" || !strings.Contains(status.Error, "multiple projects") || len(model.prompts) != 0 {
		t.Fatalf("ambiguous mapping was used: %+v", status)
	}
	pending, err := s.ListRaw(context.Background(), "pending", 0)
	if err != nil || pending.PendingCount != 1 {
		t.Fatal("ambiguous mapping consumed notifications")
	}
}

func TestOversizedProjectCardCanBeCorrectedWithoutSplittingItsInput(t *testing.T) {
	s, model, _, source := fixture(t)
	s.Tools = nil
	subjects := []*triageSubject{}
	for i := 0; i < 30; i++ {
		e := event()
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+1)
		subjects = append(subjects, &triageSubject{fresh: e, events: []RawNotification{{ID: int64(i + 1), SourceID: source.ID, Event: e}}, findings: []intentFinding{{Intent: "review_request", Quote: "check"}, {Intent: "review_feedback", Quote: "check"}, {Intent: "direct_question", Quote: "check"}}})
	}
	decision, err := s.renderTriage(context.Background(), subjects, nil, RuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	model.respond = func(prompt string) (any, error) {
		attempts++
		_, raw, _ := strings.Cut(prompt, projectCardsMarker)
		raw, _, _ = strings.Cut(raw, "\nCORRECTION")
		var input struct {
			Subjects []projectSubjectData `json:"subjects"`
			Cards    []struct {
				ID string `json:"card_id"`
			} `json:"candidate_cards"`
		}
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, err
		}
		if len(input.Subjects) == 0 || len(input.Subjects) > 30 || len(prompt) > projectPromptLimit {
			t.Fatal("writer lost subjects or exceeded its budget")
		}
		card := func(ids []string) map[string]any {
			return map[string]any{"card_ids": ids, "summary": "Project activity", "context": "Review the relevant source activities.", "question": "Handle these activities?", "options": []string{"Handle these activities"}}
		}
		cards := []any{}
		if attempts == 1 {
			ids := []string{}
			for _, candidate := range input.Cards {
				ids = append(ids, candidate.ID)
			}
			cards = append(cards, card(ids))
		} else {
			for _, candidate := range input.Cards {
				cards = append(cards, card([]string{candidate.ID}))
			}
		}
		return map[string]any{"cards": cards}, nil
	}
	if err := s.draftProjectCards(context.Background(), &decision, subjects, RuleSet{}, ""); err != nil {
		t.Fatal(err)
	}
	if attempts < 2 || len(decision.Actions) != 30 {
		t.Fatalf("oversized card was not corrected: attempts=%d cards=%d", attempts, len(decision.Actions))
	}
	for _, card := range decision.Actions {
		if len(card.Plan.Evidence) == 0 || len(card.Context) > triageCardContextLimit {
			t.Fatal("corrected card lost provenance or exceeded its limit")
		}
	}
}

// The production snapshot for the merge-readiness question on cloudinfo's
// merged PR had complete retrieval but uncertain comment recency. A routine
// classification must survive that uncertainty instead of creating a card.
func TestResolvedTerminalActivityDoesNotRequireRecencyInspection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		facts     *PRFacts
		retrieved bool
		intent    string
		current   bool
		want      string
	}{
		{"merged readiness resolved", &PRFacts{Available: true, State: "merged"}, true, "routine", false, "routine"},
		{"closed activity resolved", &PRFacts{Available: true, State: "closed"}, true, "routine", false, "routine"},
		{"open recency unknown", &PRFacts{Available: true, State: "open"}, true, "routine", false, "needs_context"},
		{"stale merge", &PRFacts{Available: true, Stale: true, State: "merged"}, true, "routine", false, "needs_context"},
		{"unavailable merge", &PRFacts{State: "merged"}, true, "routine", false, "needs_context"},
		{"missing facts", nil, true, "routine", false, "needs_context"},
		{"incomplete retrieval", &PRFacts{Available: true, State: "merged"}, false, "routine", false, "needs_context"},
		{"independent post-merge question", &PRFacts{Available: true, State: "merged"}, true, "direct_question", true, "direct_question"},
		{"unresolved recency", &PRFacts{Available: true, State: "merged"}, true, "direct_question", false, "needs_context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := SubjectEvidence{ID: "comment", Text: "all set here? can it be merged?", Complete: true, Current: tc.current}
			subject := &triageSubject{fresh: Event{PRFacts: tc.facts, Evidence: []SubjectEvidence{evidence}}}
			data := projectSubjectData{Evidence: []SubjectEvidence{evidence}, RetrievalComplete: tc.retrieved, ContextComplete: false}
			normalizeProjectFindings(subject, []intentFinding{{Intent: tc.intent, EvidenceID: evidence.ID, Quote: evidence.Text}}, data, RuleSet{})
			if len(subject.findings) != 1 || subject.findings[0].Intent != tc.want {
				t.Fatalf("findings = %+v, want %s", subject.findings, tc.want)
			}
		})
	}
}

func TestShortenedTerminalActivityStillRequiresInspection(t *testing.T) {
	for _, state := range []string{"merged", "closed"} {
		t.Run(state, func(t *testing.T) {
			comment := SubjectEvidence{ID: "comment", Text: "can it be merged?", Complete: true}
			followup := SubjectEvidence{ID: "followup", Text: strings.Repeat("background ", projectPromptLimit/10) + "Please investigate the deployment failure.", Complete: true}
			subject := &triageSubject{fresh: Event{PRFacts: &PRFacts{Available: true, State: state}, Evidence: []SubjectEvidence{comment, followup}}}
			data := []projectSubjectData{{ID: "subject-1", Evidence: []SubjectEvidence{comment, followup}, RetrievalComplete: true, ContextComplete: true}}
			_, shortened, err := projectPrompt(projectIntentPrompt, "", projectEvidenceMarker, map[string]any{}, data)
			if err != nil {
				t.Fatal(err)
			}
			if !shortened[0].RetrievalComplete || shortened[0].ContextComplete || shortened[0].Evidence[1].Complete {
				t.Fatal("expected complete retrieval with a shortened follow-up excerpt")
			}
			// Even a citation to an intact comment cannot establish that the
			// omitted portion of another comment has no remaining request.
			normalizeProjectFindings(subject, []intentFinding{{Intent: "routine", EvidenceID: comment.ID, Quote: comment.Text}}, shortened[0], RuleSet{})
			if len(subject.findings) != 1 || subject.findings[0].Intent != "needs_context" {
				t.Fatalf("shortened activity was discarded: %+v", subject.findings)
			}
		})
	}
}

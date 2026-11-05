package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kamacu/internal/settings"
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

func TestApprovalChangeReclassifiesWholeProjectAgainstUserPreferences(t *testing.T) {
	s, model, _, source := fixture(t)
	s.Providers["github"] = &approvalChangesProvider{fakeProvider: &fakeProvider{}}
	const preference = "Ignore review requests after I have approved the PR."
	if err := settings.Set(s.DB, settings.KeyNotificationPrompt, preference); err != nil {
		t.Fatal(err)
	}
	seed(t, s, source, event())
	classifications := 0
	model.respond = func(prompt string) (any, error) {
		if _, raw, ok := strings.Cut(prompt, projectEvidenceMarker); ok {
			classifications++
			var data struct {
				Subjects []projectSubjectData `json:"subjects"`
			}
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				return nil, err
			}
			items := []map[string]any{}
			for _, subject := range data.Subjects {
				finding := intentFinding{Intent: "review_request"}
				for _, evidence := range subject.Evidence {
					if evidence.ID == "current-review-request" {
						finding.EvidenceID, finding.Quote = evidence.ID, evidence.Text
					}
				}
				if subject.Facts.(map[string]any)["viewer_ever_approved"] == true {
					finding.Intent, finding.PolicyQuote = "policy_ignore", preference
				}
				items = append(items, map[string]any{"subject_id": subject.ID, "findings": []intentFinding{finding}})
			}
			return map[string]any{"subjects": items}, nil
		}
		return fixtureProjectCards(prompt, nil)
	}
	status := process(t, s, model, "")
	if status.Status != "completed" || status.ActionCount != 0 || classifications != 2 {
		t.Fatalf("approval change bypassed the user's preference: %+v classifications=%d", status, classifications)
	}
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

func TestProjectPromptsIncludeEverySubjectAndCombineRepeatedNotifications(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	const preference = "Collect the dependency-helper updates into one decision. Offer starting maintenance-cycle for the project."
	if err := settings.Set(s.DB, settings.KeyNotificationPrompt, preference); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE tasks SET title='maintenance-cycle',recurrent=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		e := event()
		e.Title, e.URL, e.ExternalID = fmt.Sprintf("Dependency %d", i+1), fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+1), fmt.Sprint(i)
		e.Version = "2026-11-05T04:00:00Z"
		e.PRFacts = &PRFacts{Available: true, State: "open", Viewer: "alice", RequestedReviewers: []string{"alice"}}
		e.Evidence = []SubjectEvidence{{ID: e.URL, URL: e.URL, Actor: "dependency-helper[bot]", Text: e.Title, Complete: true, UpdatedAt: e.Version}}
		seed(t, s, source, e)
		if i == 0 {
			e.Version = "2026-11-05T04:01:00Z"
			seed(t, s, source, e)
		}
	}
	respondWithIntent(model, "routine")
	classify := model.respond
	model.respond = func(prompt string) (any, error) {
		if !strings.Contains(prompt, preference) || !strings.Contains(prompt, "Dependency 1") || !strings.Contains(prompt, "Dependency 2") {
			t.Fatal("model did not receive the full project and user preferences")
		}
		if strings.Contains(prompt, projectEvidenceMarker) {
			var data struct {
				Subjects []projectSubjectData `json:"subjects"`
			}
			_, raw, _ := strings.Cut(prompt, projectEvidenceMarker)
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Subjects) != 2 || len(data.Subjects[0].Notifications) != 2 || len(data.Subjects[1].Notifications) != 1 {
				t.Fatalf("revisions were split or lost: %+v", data.Subjects)
			}
			if !strings.Contains(prompt, `"requested_reviewers":["alice"]`) {
				t.Fatal("verified reviewer facts omitted")
			}
			return classify(prompt)
		}
		if !strings.Contains(prompt, `"recurrent":true`) {
			t.Fatal("project workflow metadata missing")
		}
		return map[string]any{"cards": []any{map[string]any{"card_ids": []string{"card-1", "card-2"}, "summary": "Two dependency updates await your review", "context": "Both updates request your review and can use the project maintenance workflow.", "question": "Run maintenance-cycle for both updates?", "options": []string{"Start maintenance-cycle for both updates", "Summarize the updates first"}}}}, nil
	}
	status := process(t, s, model, "")
	if status.Status != "completed" || status.ActionCount != 1 || len(model.prompts) != 2 {
		t.Fatalf("unexpected project processing: %+v calls=%d", status, len(model.prompts))
	}
	page, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, page.Items[0].ID)
	if err != nil || len(ids) != 3 || !strings.Contains(page.Items[0].Question, "maintenance-cycle") {
		t.Fatalf("LLM grouping lost its scope or chosen workflow: %v %v", ids, err)
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

func TestPreferenceChangeDuringProjectDraftingKeepsNotificationsPending(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.onGenerate = func(prompt string) {
		if strings.Contains(prompt, projectCardsMarker) {
			if err := settings.Set(s.DB, settings.KeyNotificationPrompt, "New preference"); err != nil {
				t.Fatal(err)
			}
		}
	}
	status := process(t, s, model, "direct_question")
	if status.Status != "failed" || !strings.Contains(status.Error, "preferences changed") {
		t.Fatalf("stale-preference card published: %+v", status)
	}
	pending, err := s.ListRaw(context.Background(), "pending", 0)
	if err != nil || pending.PendingCount != 1 {
		t.Fatal("preference change consumed notifications")
	}
}

func TestOversizedProjectCardCanBeCorrectedWithoutSplittingItsInput(t *testing.T) {
	s, model, _, source := fixture(t)
	s.Tools = nil
	subjects := []*triageSubject{}
	for i := 0; i < 65; i++ {
		e := event()
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", i+1)
		subjects = append(subjects, &triageSubject{fresh: e, events: []RawNotification{{ID: int64(i + 1), SourceID: source.ID, Event: e}}, findings: []intentFinding{{Intent: "review_request", Quote: "check"}, {Intent: "review_feedback", Quote: "check"}, {Intent: "direct_question", Quote: "check"}}})
	}
	decision, err := s.renderTriage(context.Background(), subjects, nil, settings.NotificationPolicy{})
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
		if len(input.Subjects) != 65 {
			t.Fatal("correction split the project input")
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
	if err := s.draftProjectCards(context.Background(), &decision, subjects, settings.NotificationPolicy{}, ""); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(decision.Actions) != 65 {
		t.Fatalf("oversized card was not corrected: attempts=%d cards=%d", attempts, len(decision.Actions))
	}
	for _, card := range decision.Actions {
		records, _, present := readTriageEvidence(card.Context)
		if !present || len(records) != 3 || len(card.Context) > triageCardContextLimit {
			t.Fatal("corrected card lost provenance or exceeded its limit")
		}
	}
}

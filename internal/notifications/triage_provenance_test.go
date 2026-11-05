package notifications

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGroupedCardCannotDeduplicateFeedbackUsingAnotherSubjectQuote(t *testing.T) {
	s, model, _, source := fixture(t)
	first := event()
	first.Version = "2026-11-05T04:00:00Z"
	first.Evidence = []SubjectEvidence{{ID: first.URL + "#issuecomment-1", URL: first.URL + "#issuecomment-1", Text: "Question for first PR?", UpdatedAt: first.Version, Complete: true}}
	second := first
	second.ExternalID = "other"
	second.URL = "https://github.com/acme/repo/pull/8"
	second.Evidence = []SubjectEvidence{{ID: second.URL + "#issuecomment-1", URL: second.URL + "#issuecomment-1", Text: "Shared feedback", UpdatedAt: second.Version, Complete: true}}
	seed(t, s, source, first)
	seed(t, s, source, second)
	newFeedback := false
	model.respond = func(prompt string) (any, error) {
		if _, raw, ok := strings.Cut(prompt, projectEvidenceMarker); ok {
			var data struct {
				Subjects []projectSubjectData `json:"subjects"`
			}
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				return nil, err
			}
			items := []map[string]any{}
			for _, subject := range data.Subjects {
				intent := "review_feedback"
				if subject.URL == first.URL && !newFeedback {
					intent = "direct_question"
				}
				evidence := subject.Evidence[0]
				items = append(items, map[string]any{"subject_id": subject.ID, "findings": []intentFinding{{Intent: intent, EvidenceID: evidence.ID, Quote: evidence.Text}}})
			}
			return map[string]any{"subjects": items}, nil
		}
		_, raw, _ := strings.Cut(prompt, projectCardsMarker)
		var data struct {
			Cards []struct {
				ID string `json:"card_id"`
			} `json:"candidate_cards"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return nil, err
		}
		ids := []string{}
		for _, card := range data.Cards {
			ids = append(ids, card.ID)
		}
		return map[string]any{"cards": []any{map[string]any{"card_ids": ids, "summary": "Project activity", "context": "Related activity needs a decision.", "question": "Handle this activity?", "options": []string{"Handle these activities"}}}}, nil
	}
	if status := process(t, s, model, ""); status.Status != "completed" || status.ActionCount != 1 {
		t.Fatal(status)
	}
	newFeedback = true
	first.Version = "2026-11-05T05:00:00Z"
	first.Evidence = []SubjectEvidence{{ID: first.URL + "#issuecomment-2", URL: first.URL + "#issuecomment-2", Text: "Shared feedback", UpdatedAt: first.Version, Complete: true}}
	seed(t, s, source, first)
	if status := process(t, s, model, ""); status.Status != "completed" || status.ActionCount != 1 {
		t.Fatalf("new feedback on first PR was confused with second PR's old quote: %+v", status)
	}
	first.Version = "2026-11-05T06:00:00Z"
	first.Evidence[0].UpdatedAt = first.Version
	seed(t, s, source, first)
	if status := process(t, s, model, ""); status.Status != "completed" || status.ActionCount != 0 {
		t.Fatalf("identical feedback did not deduplicate against its own subject: %+v", status)
	}
}

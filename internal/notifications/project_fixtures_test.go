package notifications

import (
	"encoding/json"
	"strings"
)

// Adapt older single-subject fixtures to the project response envelope. The
// production decoder still receives and validates the real project contract.
func marshalFixtureResponse(prompt string, response any) ([]byte, error) {
	if findings, ok := response.(intentResponse); ok && strings.Contains(prompt, projectEvidenceMarker) {
		_, raw, _ := strings.Cut(prompt, projectEvidenceMarker)
		raw, _, _ = strings.Cut(raw, "\nCORRECTION")
		var payload struct {
			Subjects []projectSubjectData `json:"subjects"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
		items := []map[string]any{}
		for _, subject := range payload.Subjects {
			items = append(items, map[string]any{"subject_id": subject.ID, "findings": findings.Findings})
		}
		response = map[string]any{"subjects": items}
	}
	if draft, ok := response.(triageDraft); ok && strings.Contains(prompt, projectCardsMarker) {
		return fixtureProjectCardsJSON(prompt, &draft)
	}
	return json.Marshal(response)
}

func fixtureProjectCardsJSON(prompt string, draft *triageDraft) ([]byte, error) {
	response, err := fixtureProjectCards(prompt, draft)
	if err != nil {
		return nil, err
	}
	return json.Marshal(response)
}

func fixtureProjectCards(prompt string, draft *triageDraft) (any, error) {
	_, raw, _ := strings.Cut(prompt, projectCardsMarker)
	raw, _, _ = strings.Cut(raw, "\nCORRECTION")
	var payload struct {
		Candidates []struct {
			ID   string      `json:"card_id"`
			Card BatchAction `json:"card"`
		} `json:"candidate_cards"`
		Policy struct {
			Task string `json:"batch_review_task"`
		} `json:"structured_preferences"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	cards := []map[string]any{}
	for _, candidate := range payload.Candidates {
		body := draft
		if body == nil {
			body = &triageDraft{Summary: candidate.Card.Summary, Context: candidate.Card.Context, Question: candidate.Card.Question, Options: candidate.Card.Options}
			if body.Context == "" {
				body.Context = "Current source activities awaiting your decision."
			}
			if body.Question == "" {
				body.Question = "How should I handle these activities?"
			}
		}
		options := []string{}
		for _, option := range body.Options {
			if option != "Leave this activity alone" {
				options = append(options, option)
			}
		}
		if len(options) == 0 {
			options = []string{"Investigate this subject"}
		}
		if payload.Policy.Task != "" && len(cards) > 0 {
			cards[0]["card_ids"] = append(cards[0]["card_ids"].([]string), candidate.ID)
			continue
		}
		cards = append(cards, map[string]any{"card_ids": []string{candidate.ID}, "summary": body.Summary, "context": body.Context, "question": body.Question, "options": options})
	}
	return map[string]any{"cards": cards}, nil
}

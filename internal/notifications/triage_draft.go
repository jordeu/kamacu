package notifications

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"kamacu/internal/inbox"
)

type triageDraft struct {
	Summary  string   `json:"summary"`
	Context  string   `json:"context"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

var triageDraftSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"summary", "context", "question", "options"},
	"properties": map[string]any{
		"summary": map[string]string{"type": "string"}, "context": map[string]string{"type": "string"}, "question": map[string]string{"type": "string"},
		"options": map[string]any{"type": "array", "minItems": 1, "maxItems": 2, "items": map[string]string{"type": "string"}},
	},
}

func decodeTriageDraft(raw []byte) (triageDraft, error) {
	var draft triageDraft
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&draft); err != nil {
		return draft, err
	}
	if d.Decode(new(any)) != io.EOF || strings.TrimSpace(draft.Summary) == "" || len(draft.Summary) > 1600 || strings.TrimSpace(draft.Context) == "" || len(draft.Context) > 8000 || strings.TrimSpace(draft.Question) == "" || len(draft.Question) > 2000 || len(draft.Options) < 1 || len(draft.Options) > 2 {
		return draft, errors.New("invalid notification card draft")
	}
	seen := map[string]bool{}
	for _, option := range draft.Options {
		if strings.TrimSpace(option) == "" || len(option) > 500 || seen[option] || option == "Leave this activity alone" {
			return draft, errors.New("invalid notification card options")
		}
		seen[option] = true
	}
	return draft, nil
}

func isTriageQuestion(q inbox.Question, intent string) bool {
	if records, _, present := readTriageEvidence(q.Context); present {
		for _, record := range records {
			if record.Intent == intent {
				return true
			}
		}
		return false
	}
	return q.Question == triageQuestions[intent] || strings.Contains(q.Context, "<!-- kamacu-triage:"+intent+" -->")
}

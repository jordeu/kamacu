package settings

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// CustomPrompt is a reusable literal text snippet inserted into a task agent.
type CustomPrompt struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

func validateCustomPrompts(value string) error {
	var prompts []CustomPrompt
	if err := json.Unmarshal([]byte(value), &prompts); err != nil || prompts == nil {
		return errors.New("Custom prompts must be a JSON array of titles and texts.")
	}
	if len(prompts) > 100 {
		return errors.New("You can save up to 100 custom prompts.")
	}
	for _, prompt := range prompts {
		if strings.TrimSpace(prompt.Title) == "" || strings.TrimSpace(prompt.Text) == "" {
			return errors.New("Each prompt needs a title and text.")
		}
		if utf8.RuneCountInString(prompt.Title) > 80 {
			return errors.New("Prompt titles must be 80 characters or fewer.")
		}
		if utf8.RuneCountInString(prompt.Text) > 20000 {
			return errors.New("Prompt text must be 20,000 characters or fewer.")
		}
	}
	return nil
}

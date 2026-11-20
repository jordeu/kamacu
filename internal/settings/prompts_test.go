package settings_test

import (
	"strings"
	"testing"

	"kamacu/internal/settings"
)

func TestCustomPromptsValidation(t *testing.T) {
	for _, value := range []string{
		`[]`,
		`[{"title":"Explain","text":"Explain the changes.\nInclude examples."}]`,
	} {
		if err := settings.Validate(settings.KeyCustomPrompts, value); err != nil {
			t.Errorf("valid prompts %s: %v", value, err)
		}
	}
	for _, value := range []string{
		``, `null`, `{}`, `[null]`, `[{}]`,
		`[{"title":" ","text":"test"}]`,
		`[{"title":"Test","text":"\n "}]`,
		`[{"title":12,"text":"test"}]`,
		`[{"title":"` + strings.Repeat("a", 81) + `","text":"test"}]`,
		`[{"title":"Test","text":"` + strings.Repeat("a", 20001) + `"}]`,
		`[` + strings.Repeat(`{"title":"Test","text":"test"},`, 100) + `{"title":"Test","text":"test"}]`,
	} {
		if err := settings.Validate(settings.KeyCustomPrompts, value); err == nil {
			t.Errorf("invalid prompts accepted: %.100s", value)
		}
	}
}

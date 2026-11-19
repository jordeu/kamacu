package inbox

import (
	"context"
	"strings"
	"testing"
)

func TestGeneratedHeadlineContract(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		invalid    bool
	}{
		{"event", "Fix merged; release still pending", false},
		{"unicode limit", strings.Repeat("界", MaxHeadlineCharacters), false},
		{"too long", strings.Repeat("界", MaxHeadlineCharacters+1), true},
		{"empty", " \t", true},
		{"multiple lines", "PR approved\nMerge checks pending", true},
		{"invalid UTF8", string([]byte{0xff}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateGeneratedHeadline(tc.text); (err != nil) != tc.invalid {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacySummaryRemainsIntact(t *testing.T) {
	s, who := fixture(t)
	legacy := strings.Repeat("A detailed source summary. ", 20)
	out, err := s.Create(context.Background(), who, Create{Key: "legacy-summary", Abstract: "Legacy preview", Summary: legacy, Context: "Full source context", Question: "Continue?"})
	if err != nil || out.Question.Summary != legacy || out.Question.Context != "Full source context" {
		t.Fatal(out, err)
	}
}

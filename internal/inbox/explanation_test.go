package inbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExplanationBoundsPreserveFullSource(t *testing.T) {
	for _, tc := range []struct {
		name, explanation string
		invalid           bool
	}{
		{"old message", "", false},
		{"long exact payload", strings.Repeat("界", MaxExplanationBytes/3), false},
		{"over byte limit", strings.Repeat("界", MaxExplanationBytes/3+1), true},
		{"invalid UTF8", string([]byte{0xff}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, who := fixture(t)
			c := Create{Key: tc.name, Abstract: "Ready for review", Summary: "Ready", Question: "Publish?", Context: "The full source text stays here, including all test logs and the exact proposed review.", Explanation: tc.explanation}
			out, err := s.Create(context.Background(), who, c)
			if tc.invalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatal(out, err)
				}
				return
			}
			if err != nil || out.Question.Explanation != tc.explanation || out.Question.Context != c.Context {
				t.Fatal(out, err)
			}
		})
	}
}

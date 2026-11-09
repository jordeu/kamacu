package inbox

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestLegacyCompletionValidatesBeforeReadingTask(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	unknown := Identity{TaskID: 999, SessionID: "missing"}
	if _, err := s.Complete(ctx, unknown, Create{}); !errors.Is(err, ErrIdentity) {
		t.Fatalf("invalid identity returned a metadata error: %v", err)
	}
	s.UseTerminalMonitoring()
	if _, err := s.Complete(ctx, unknown, Create{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("disabled legacy reporting read task metadata: %v", err)
	}
}

func TestLegacyCompletionDoesNotAcceptAgentRecommendations(t *testing.T) {
	s, who := fixture(t)
	out, err := s.Complete(context.Background(), who, Create{Key: "done", Abstract: "Done", Summary: "Done", Context: "Tests passed", Options: []Option{{ID: MoveToDone, Label: "Done", Recommended: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range out.Question.Options {
		if option.Recommended {
			t.Fatalf("agent-provided recommendation survived server-defined controls: %+v", option)
		}
	}
}

func TestCompletionControlsIndependentOfRecommendations(t *testing.T) {
	for _, tc := range []struct {
		name, source, status string
		taskID               int64
		want                 []string
	}{
		{"todo", "manual", "todo", 1, []string{MoveToReview, MoveToDone, CloseSession}},
		{"in progress", "manual", "in_progress", 1, []string{MoveToReview, MoveToDone, CloseSession}},
		{"in review", "manual", "in_review", 1, []string{MoveToDone, CloseSession}},
		{"done", "manual", "done", 1, []string{CloseSession}},
		{"PR review", "github_pr", "todo", 1, []string{CloseSession}},
		{"Scratchpad", "", "", 0, []string{CloseSession}},
		{"missing context", "", "", 1, []string{CloseSession}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, suggested := range [][]string{nil, {MoveToDone}, {MoveToReview, MoveToDone}} {
				got := CompletionOptions(tc.taskID, tc.source, tc.status, suggested)
				var ids []string
				for _, o := range got {
					ids = append(ids, o.ID)
					if o.Recommended != (o.ID != CloseSession && slices.Contains(suggested, o.ID)) {
						t.Fatalf("incorrect recommendation: %+v", o)
					}
				}
				if !slices.Equal(ids, tc.want) {
					t.Fatalf("actions=%v want=%v", ids, tc.want)
				}
			}
		})
	}
}

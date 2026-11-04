package notifications

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/diagnostics"
	"kamacu/internal/monitor"
)

type recordingModel struct {
	model    *fakeModel
	recorder *diagnostics.Recorder
}

func (m recordingModel) GenerateJSON(ctx context.Context, p monitor.Profile, prompt string, schema any) ([]byte, error) {
	raw, err := m.model.GenerateJSON(ctx, p, prompt, schema)
	m.recorder.Record(ctx, map[string]any{"purpose": "notification", "prompt": prompt, "response": string(raw), "schema": schema, "model": p.Model})
	return raw, err
}
func checkTranscriptGroup(t *testing.T, dir, kind, id string, wantCalls int) {
	t.Helper()
	groups, calls := 0, 0
	err := filepath.WalkDir(filepath.Join(dir, "llm"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "prompt.md" {
			calls++
		}
		if d.Name() == "group.json" {
			groups++
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var v struct{ Group diagnostics.Group }
			if e = json.Unmarshal(data, &v); e != nil {
				return e
			}
			if v.Group.Kind != kind || v.Group.ID != id {
				t.Errorf("wrong group: %+v", v.Group)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if groups != 1 || calls != wantCalls {
		t.Fatalf("groups=%d calls=%d, want 1/%d", groups, calls, wantCalls)
	}
}
func TestProcessingTranscriptsIncludeRepositoriesAndFollowups(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	first := seed(t, s, source, event())
	other := event()
	other.ExternalID = "other-thread"
	other.Repository = "acme/other"
	other.URL = "https://github.com/acme/other/pull/9"
	second := seed(t, s, source, other)
	dir := t.TempDir()
	rec, err := diagnostics.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = recordingModel{model, rec}
	model.responses = []any{BatchDecision{ToolName: "list_sessions", Arguments: `{"project_id":2}`}, BatchDecision{DiscardIDs: []int64{second}}, BatchDecision{DiscardIDs: []int64{first}}}
	p, err := s.RequestProcessing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.Processing.Status != "completed" {
		t.Fatalf("%+v %v", status.Processing, err)
	}
	checkTranscriptGroup(t, dir, "batch", p.ID, 3)
}
func TestReplyTranscriptsShareRun(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	q := publish(t, s, seed(t, s, source, event()))
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Check and fix", ""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rec, err := diagnostics.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = recordingModel{model, rec}
	model.steps = []Step{{Action: "tool", ToolName: "list_sessions", Arguments: `{"project_id":1}`}, {Action: "done", Message: "Checked sessions"}}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	checkTranscriptGroup(t, dir, "reply", q.ID, 2)
}

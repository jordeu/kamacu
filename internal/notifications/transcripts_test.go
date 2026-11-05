package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/diagnostics"
	"kamacu/internal/monitor"
)

type recordingModel struct {
	model    *fakeModel
	recorder *diagnostics.Recorder
}

func (m recordingModel) GenerateValidatedJSON(ctx context.Context, p monitor.Profile, prompt string, schema any, validate func([]byte) error) ([]byte, error) {
	raw, err := m.model.GenerateValidatedJSON(ctx, p, prompt, schema, validate)
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	m.recorder.Record(ctx, map[string]any{"error": detail, "purpose": "notification", "prompt": prompt, "response": string(raw), "schema": schema, "model": p.Model})
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
	seed(t, s, source, event())
	other := event()
	other.ExternalID = "other-thread"
	other.Repository = "acme/other"
	other.URL = "https://github.com/acme/other/pull/9"
	seed(t, s, source, other)
	dir := t.TempDir()
	rec, err := diagnostics.NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Model = recordingModel{model, rec}
	respondWithIntent(model, "routine")
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
	checkTranscriptGroup(t, dir, "batch", p.ID, 2)
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

type transcriptTransport struct{ content string }

func (tr transcriptTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tr.content}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: req}, nil
}
func TestValidationOutcomesAreCaptured(t *testing.T) {
	for _, tc := range []struct {
		name, content    string
		batch, wantError bool
	}{
		{name: "reply invalid JSON", content: "not JSON", wantError: true},
		{name: "reply invalid contract", content: `{"action":"ask"}`, wantError: true},
		{name: "reply valid", content: `{"action":"done","message":"Finished"}`},
		{name: "batch invalid JSON", content: "not JSON", batch: true, wantError: true},
		{name: "batch trailing content", content: `{} {}`, batch: true, wantError: true},
		{name: "batch invalid tool", content: `{"tool_name":"delete_task","arguments":"{}"}`, batch: true, wantError: true},
		{name: "batch invalid arguments", content: `{"tool_name":"list_sessions","arguments":"null"}`, batch: true, wantError: true},
		{name: "batch unknown event", content: `{"discard_ids":[999999]}`, batch: true, wantError: true},
		{name: "batch invalid action", content: `{"actions":[{"event_ids":[1]}]}`, batch: true, wantError: true},
		{name: "batch valid", content: `{"subjects":[{"subject_id":"subject-1","findings":[{"intent":"routine","evidence_id":"context","quote":"Review requested","policy_quote":""}]}]}`, batch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, source := fixture(t)
			ctx := context.Background()
			dir := t.TempDir()
			rec, err := diagnostics.NewRecorder(dir)
			if err != nil {
				t.Fatal(err)
			}
			metrics := &monitor.Metrics{}
			s.Model = monitor.Backend{Transcripts: rec, Metrics: metrics, Client: &http.Client{Transport: transcriptTransport{tc.content}}}
			if tc.batch {
				seed(t, s, source, event())
				if _, err = s.RequestProcessing(ctx); err != nil {
					t.Fatal(err)
				}
				if err = s.processNext(ctx); err != nil {
					t.Fatal(err)
				}
				status, err := s.Status(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if (status.Processing.Status == "failed") != tc.wantError {
					t.Fatalf("batch: %+v", status.Processing)
				}
			} else {
				_, err = s.infer(ctx, "test prompt")
				if (err != nil) != tc.wantError {
					t.Fatalf("inference error: %v", err)
				}
			}
			calls := 0
			err = filepath.WalkDir(filepath.Join(dir, "llm"), func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.Name() == "metadata.json" {
					calls++
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					var v struct{ Error string }
					if err = json.Unmarshal(data, &v); err != nil {
						return err
					}
					if (v.Error != "") != tc.wantError {
						t.Fatalf("incorrect transcript error: %s", data)
					}
					response, err := os.ReadFile(filepath.Join(filepath.Dir(path), "response.md"))
					if err != nil {
						return err
					}
					if len(response) == 0 {
						t.Fatal("raw response lost")
					}
				}
				if d.Name() == "summary.md" {
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					if tc.wantError && (!strings.Contains(string(data), "Error:") || strings.Contains(string(data), "| OK |")) {
						t.Fatalf("incorrect summary: %s", data)
					}
					if !tc.wantError && !strings.Contains(string(data), "| OK |") {
						t.Fatalf("missing success: %s", data)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if tc.batch && tc.wantError {
				wantCalls = 2 // The bounded correction is recorded separately.
			}
			if calls != wantCalls {
				t.Fatalf("expected %d interactions, got %d", wantCalls, calls)
			}
			usage := metrics.Snapshot()[1]
			if int(usage.Calls) != wantCalls || (usage.Failures == usage.Calls) != tc.wantError {
				t.Fatalf("incorrect metrics: %+v", usage)
			}
		})
	}
}

package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"kamacu/internal/observation"
)

func TestClassifierCollectsExplanationAndFullContextInOneCall(t *testing.T) {
	const source = "The PR is approved. CI passed. E2E may have tested the old artifact. Check the version before merging. Should I check and merge?"
	const explanation = "The PR is approved and CI passed. We still need to check that E2E tested the new version. I will merge only if that check and the merge checks pass."
	want := Decision{State: "waiting", Kind: "question", Evidence: "Should I check and merge?", Summary: "Approved; check the tested version before merging.", Context: source, Explanation: explanation, Question: "Should I check and merge?", InputMode: "text"}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	b := Backend{Client: &http.Client{Transport: notificationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var request struct {
			ResponseFormat struct {
				JSONSchema struct {
					Schema struct {
						Required []string `json:"required"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(request.ResponseFormat.JSONSchema.Schema.Required, "explanation") {
			t.Fatal("explanation missing from collection schema")
		}
		response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(raw)}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
	})}}
	got, err := b.Classify(context.Background(), Profile{Backend: "openai_compatible", Model: "test", Endpoint: "https://model.invalid"}, observation.Snapshot{Screen: source})
	if err != nil || calls != 1 || got.Explanation != explanation || got.Context != source || got.Question != want.Question {
		t.Fatal(got, err, calls)
	}
}

func TestClassifierRejectsMissingExplanationsOnlyForNewAttention(t *testing.T) {
	for _, tc := range []struct {
		name, kind, state, explanation string
		omit, wantError                bool
	}{
		{"missing", "question", "waiting", "", true, true},
		{"blank", "question", "waiting", " \n\t", false, true},
		{"completion", "completion", "idle", "", false, true},
		{"working", "none", "working", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := map[string]any{"state": tc.state, "kind": tc.kind, "evidence": "Ready", "summary": "Ready", "context": "Full source", "question": "Continue?", "input_mode": "text", "explanation": tc.explanation}
			if tc.kind != "question" {
				d["input_mode"] = "none"
				d["question"] = ""
			}
			if tc.omit {
				delete(d, "explanation")
			}
			raw, _ := json.Marshal(d)
			b := Backend{Client: &http.Client{Transport: notificationTransport(func(*http.Request) (*http.Response, error) {
				response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
			})}}
			_, err := b.Classify(context.Background(), Profile{Backend: "openai_compatible", Endpoint: "https://model.invalid", Model: "test"}, observation.Snapshot{Screen: "Ready"})
			if (err != nil) != tc.wantError {
				t.Fatal(err)
			}
		})
	}
}

package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/observation"
)

// Opt-in model quality check, rather than a mocked assertion that simply
// returns the expected prose. Keep short requests short and substantive
// responses useful without opening the full context.
func TestInstalledLoginExplanationQuality(t *testing.T) {
	if os.Getenv("KAMACU_MONITOR_MESSAGES") != "1" {
		t.Skip("opt-in explanation quality check")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, tc := range []struct {
		name, source string
		facts        []string
		maxWords     int
		kind         string
	}{
		{"review", "Problem: a prod pod peaked at 1.7 GiB; only 950 MiB is JVM memory, with the remainder native memory. Dropping the 2Gi pod limit to 1792Mi is unsafe. Solution: reduce DuckDB's cap from 256MB to 128MB and jemalloc decay from 5 s to 1 s. This reduces native memory before a later pod limit reduction. Config only, low complexity. Risk: larger queries may fail or spill; faster purging may cost CPU. Which review effort should I use? Low (recommended), Medium, High, Xhigh.", []string{"1.7", "1792", "128", "native"}, 0, "question"},
		{"implementation", "The upload endpoint allowed non-members to submit files. I now require authentication and workspace membership, cap uploads at 8 MiB, and check PNG/JPEG signatures. All 12 tests passed. Existing attachments are unchanged. Implementation is complete.", []string{"member", "8 MiB", "PNG", "JPEG"}, 0, "completion"},
		{"troubleshooting", "The worker times out because the upstream rate limit is 100 requests/minute. I can lower concurrency, which slows the backlog, or add retries, which needs a code change and may duplicate work unless we add idempotency. Which approach should I take?", []string{"100", "concurrency", "retr", "duplic"}, 0, "question"},
		{"completion", "The Terraform fix is merged but has not been applied. It is not live. The next nightly run must confirm the fix after Terraform is applied. My implementation work is complete.", []string{"Terraform", "appl", "nightly"}, 0, "completion"},
		{"permission", "Allow this command?\n  go test ./internal/inbox\n❯ 1. Yes\n  2. No\nEnter to confirm, Esc to cancel", []string{"go test ./internal/inbox"}, 65, "permission"},
		{"error", "Error: migration failed because the database user lacks ALTER permission. The agent has stopped. No migration was applied. Fix the database permissions before restarting the migration.", []string{"ALTER", "permission", "appl"}, 0, "error"},
		{"simple question", "Which branch should I use?", []string{"branch"}, 40, "question"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			screen := tc.source
			if tc.kind != "permission" {
				screen += "\n> "
			}
			result, err := (Backend{}).Classify(ctx, Profile{Backend: "claude_cli", Model: "sonnet"}, observation.Snapshot{Screen: screen, At: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			if result.Kind != tc.kind {
				t.Errorf("expected %s, got %s", tc.kind, result.Kind)
			}
			if tc.kind == "permission" && result.InputMode != "terminal" {
				t.Errorf("native permission needs terminal input, got %s", result.InputMode)
			}
			t.Log(result.Explanation)
			if err := inbox.ValidateGeneratedExplanation(result.Explanation); err != nil {
				t.Fatal(err)
			}
			for _, internalDetail := range []string{"task context", "board move", "working directory", "not a git repository"} {
				if strings.Contains(strings.ToLower(result.Explanation), internalDetail) {
					t.Errorf("exposed internal detail %q", internalDetail)
				}
			}
			for _, fact := range tc.facts {
				if !strings.Contains(strings.ToLower(result.Explanation), strings.ToLower(fact)) {
					t.Errorf("missing decisive fact %q", fact)
				}
			}
			if tc.maxWords > 0 && len(strings.Fields(result.Explanation)) > tc.maxWords {
				t.Errorf("simple request expanded to %d words", len(strings.Fields(result.Explanation)))
			}
		})
	}
}

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
	if err != nil || calls != 1 || got.Summary != want.Summary || got.Explanation != explanation || got.Context != source || got.Question != want.Question {
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

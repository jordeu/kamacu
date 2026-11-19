package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/observation"
)

func conversationScreen(body string) string {
	return body + "\n\n────────────────────────────────\n❯ \u00a0\n────────────────────────────────\n  ⏵⏵ auto mode on (shift+tab to cycle)"
}

func TestInstalledClaudeConversationalApprovals(t *testing.T) {
	if os.Getenv("KAMACU_MONITOR_CONVERSATIONAL") != "1" {
		t.Skip("opt-in synthetic check using the installed Claude login")
	}
	for _, tc := range []struct {
		name, question, body string
		count                int
	}{
		{"publish_review", "Should I publish this review?", "I drafted a review with two inline comments. Nothing is posted.\nShould I publish this review?\n1. Publish as above (approve, 2 inline comments)\n2. Publish the inline comments without approving\n3. Change something first", 3},
		{"edit_description", `Do you want me to change "four" to "five"?`, "The description is out of date.\nDo you want me to change \"four\" to \"five\"?\n1. Yes, update that line.\n2. Leave the description as it is.", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			d, err := (Backend{}).Classify(ctx, Profile{Backend: "claude_cli", Model: "sonnet"}, observation.Snapshot{Screen: conversationScreen(tc.body), At: time.Now()}, TaskContext{Engine: "claude"})
			if err != nil || d.Kind != "question" || d.InputMode != "text" || d.OptionsSuggested || len(d.Options) != tc.count {
				t.Fatal("conversational approval lost its copied choices", d, err)
			}
		})
	}
}

func TestConversationalChoicesRejectWrongModeAndLostOptions(t *testing.T) {
	for _, tc := range []struct {
		name, question, body string
		options              []string
	}{
		{"post_review", "Should I post it?", "Should I post it?\n  1. Yes, post it: approve with these 2 inline comments\n  2. Post without approving: the same 2 comments as a plain comment review\n  3. Change something first", []string{"1. Yes, post it: approve with these 2 inline comments", "2. Post without approving: the same 2 comments as a plain comment review", "3. Change something first"}},
		{"publish_review", "Should I publish this review?", "Should I publish this review?\n  1. Publish as above (approve, 2 inline comments)\n  2. Publish the inline comments without approving\n  3. Change something first", []string{"1. Publish as above (approve, 2 inline comments)", "2. Publish the inline comments without approving", "3. Change something first"}},
		{"wrapped_question", `Do you want me to change "four" to "five"?`, "The description is out of date. Do you want me to change \"four\" to\n\"five\"?\n  - 1. Yes, update that line.\n  - 2. Leave the description as it is.", []string{"1. Yes, update that line.", "2. Leave the description as it is."}},
		{"update_toast", `Do you want me to change "four" to "five"?`, "Do you want me to change \"four\" to \"five\"?\n  - 1. Yes, update that line.\n  - 2. Leave the description as it is.\n                                        ✔ Update installed · Restart to update", []string{"1. Yes, update that line.", "2. Leave the description as it is."}},
		{"wrapped_option", "Should I publish?", "Should I publish?\n  1. Post the approval with both\n     inline comments\n  2. Change the draft", []string{"1. Post the approval with both inline comments", "2. Change the draft"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen := conversationScreen(tc.body)
			obs := observation.Snapshot{Screen: screen}
			evidence := strings.Split(tc.body, "\n")[0]
			d := Decision{State: "waiting", Kind: "question", Evidence: evidence, Question: tc.question, InputMode: "terminal"}
			if err := validateDecision(d, obs); !errors.Is(err, errConversationalChoices) {
				t.Fatalf("accepted wrong mode: %v", err)
			}
			d.InputMode = "text"
			if err := validateDecision(d, obs); !errors.Is(err, errConversationalChoices) {
				t.Fatalf("accepted missing choices: %v", err)
			}
			d.Options = tc.options[:1]
			if err := validateDecision(d, obs); !errors.Is(err, errConversationalChoices) {
				t.Fatalf("accepted partial choices: %v", err)
			}
			d.Options = tc.options
			if err := validateDecision(d, obs); err != nil {
				t.Fatal(err)
			}
			// A cosmetic footer change cannot remove the same visible choices.
			obs.Screen += " · 1 agent"
			if err := validateDecision(d, obs); err != nil {
				t.Fatal(err)
			}
			d.Options, d.InputMode, d.Kind = nil, "terminal", "permission"
			if err := validateDecision(d, obs); !errors.Is(err, errConversationalChoices) {
				t.Fatalf("conversational approval became native permission: %v", err)
			}
		})
	}
}

func TestConversationalChoiceCheckPreservesNativeAndUncertainInput(t *testing.T) {
	for _, tc := range []struct{ name, screen, question, kind string }{
		{"permission", "Allow this command?\n  git status\n❯ 1. Yes\n  2. No\nEnter to confirm, Esc to cancel", "Allow this command?", "permission"},
		{"native_question", "☐ Next\nContinue?\n❯ 1. Yes\n  2. No\nEnter to select · ↑/↓ to navigate · Esc to cancel", "Continue?", "question"},
		{"uncertain_input", "Should I publish?\n1. Publish\n2. Edit\n❯ ", "Should I publish?", "question"},
		{"plan_before_question", conversationScreen("My plan:\n1. Update the parser\n2. Run tests\nShould I continue?"), "Should I continue?", "question"},
		{"quoted_list", conversationScreen("Should I continue?\n```\n1. A logged example\n2. Another logged example\n```"), "Should I continue?", "question"},
		{"typed_input", "Should I publish?\n1. Publish\n2. Edit\n────────────────────────────────\n❯ wait\n────────────────────────────────\n⏵⏵ auto mode on", "Should I publish?", "question"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Decision{State: "waiting", Kind: tc.kind, Evidence: tc.question, Question: tc.question, InputMode: "terminal"}
			if err := validateDecision(d, observation.Snapshot{Screen: tc.screen}); err != nil {
				t.Fatalf("native or uncertain input was forced into text: %v", err)
			}
		})
	}
}

type choiceTransport func(*http.Request) (*http.Response, error)

func (f choiceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConversationalChoiceRepairIsBoundedAndUsesSameContext(t *testing.T) {
	for _, outcome := range []string{"corrected", "still_wrong", "canceled"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			obs := observation.Snapshot{Screen: conversationScreen("Should I publish?\n1. Publish the review\n2. Edit the draft")}
			calls := 0
			backend := Backend{Client: &http.Client{Transport: choiceTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if calls == 2 && !strings.Contains(string(body), "CLASSIFICATION CORRECTION") {
					t.Error("repair omitted the correction")
				}
				d := Decision{State: "waiting", Kind: "question", Evidence: "Should I publish?", Summary: "Review draft is ready", Question: "Should I publish?", InputMode: "terminal", Explanation: "The review is drafted. Choose whether to publish or edit it."}
				if outcome == "corrected" && calls == 2 {
					d.InputMode, d.Options = "text", []string{"1. Publish the review", "2. Edit the draft"}
				}
				if outcome == "canceled" {
					cancel()
				}
				raw, _ := json.Marshal(d)
				response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response))), Header: make(http.Header)}, nil
			})}}
			d, err := backend.Classify(ctx, Profile{Backend: "openai_compatible", Endpoint: "http://classifier.test/v1"}, obs)
			if outcome == "corrected" {
				if err != nil || calls != 2 || d.InputMode != "text" || len(d.Options) != 2 {
					t.Fatal(d, err, calls)
				}
			} else if outcome == "still_wrong" {
				if !errors.Is(err, errConversationalChoices) || calls != 2 {
					t.Fatal(err, calls)
				}
			} else if calls != 1 || !errors.Is(err, context.Canceled) {
				t.Fatal("repair ignored parent cancellation", err, calls)
			}
		})
	}
}

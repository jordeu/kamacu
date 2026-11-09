package notifications

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadinessNoticeSeparatesDeliveredAndPendingInstructions(t *testing.T) {
	run := ActionRun{Action: json.RawMessage(`{"version":1,"actions":[{"id":"reviews","kind":"start_reviews"}]}`), Steps: []ActionStep{
		{Key: "reviews/pr/7/submit", Kind: "send_session_message", State: "succeeded"},
		{Key: "reviews/pr/8/ready", Kind: "wait_ready", State: "blocked", Arguments: json.RawMessage(`{"task_id":1266}`), Result: "agent is busy"},
	}}
	copy := actionAttentionCopy(run, false, true, "")
	if !strings.Contains(copy.summary, "1 review prompt:") || !strings.Contains(copy.context, "1 other instruction delivery succeeded") || !strings.Contains(copy.context, "Task 1266:") || !strings.Contains(copy.context, "agent is busy") {
		t.Fatal(copy)
	}
}

func TestUnknownDeliveryNoticeNeverClaimsThePromptWasNotSent(t *testing.T) {
	run := ActionRun{Steps: []ActionStep{{Key: "review/submit", Kind: "send_session_message", State: "uncertain", Result: "connection interrupted"}}}
	copy := actionAttentionCopy(run, true, false, "")
	if !strings.Contains(copy.context, "may already have taken effect") || !strings.Contains(copy.context, "Automatic retry is unavailable") || strings.Contains(copy.context, "stopped before sending") {
		t.Fatal(copy)
	}
}

func TestAttentionNoticeUsesLatestRecheckedReadiness(t *testing.T) {
	run := ActionRun{Steps: []ActionStep{
		{Key: "run/ready", Kind: "wait_ready", State: "blocked", Result: "old timeout"},
		{Key: "run/ready/recheck_1", Kind: "wait_ready", State: "succeeded"},
		{Key: "run/submit", Kind: "send_session_message", State: "uncertain", Result: "new delivery error"},
	}}
	copy := actionAttentionCopy(run, true, false, "")
	if strings.Contains(copy.context, "old timeout") || !strings.Contains(copy.context, "new delivery error") {
		t.Fatal(copy)
	}
}

func TestReadinessNoticeDoesNotSuggestUnavailableRetry(t *testing.T) {
	run := ActionRun{Steps: []ActionStep{{Key: "run/ready", Kind: "wait_ready", State: "blocked", Arguments: json.RawMessage(`{"task_id":123}`), Result: "agent is busy"}}}
	copy := actionAttentionCopy(run, false, false, "")
	if strings.Contains(copy.context, "retry sending") || strings.Contains(copy.question, "retry") || !strings.Contains(copy.context, "cannot be safely retried automatically") || !strings.Contains(copy.question, "new instructions") {
		t.Fatal(copy)
	}
}

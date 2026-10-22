package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const inboxInstructions = `When feedback is needed for a Kamacu task or Scratchpad (KAMACU_TASK_ID=0), use ask_feedback with a self-contained task summary, decision context, question, and optional simple answer options. The task and session are supplied by Kamacu. Present the same question as ordinary terminal text, NOT a blocking native question dialog, then wait_for_feedback repeatedly (up to 20 seconds per call). Process new terminal messages between polls. Do not continue other work while waiting. A user may answer in the inbox or terminal. BEFORE acting on a terminal answer, call resolve_feedback with the question ID and revision; the first answer recorded by Kamacu wins, so use the returned answer on a conflict. A clarification is not an answer: clarify in chat and keep polling. Polling is a heartbeat: after 5 minutes without a poll, or when the session stops, the question is archived unanswered. Poll cancellation does not withdraw it. An expired question stays archived until explicitly reopened; only reopen if the answer is still needed. Dismissed questions may only be reopened on the user's explicit instruction (user_requested=true), never automatically. Expired/dismissed means no answer, never approval. On resume, use list_feedback to recover questions for this task, explicitly reopen an unanswered question to take ownership, and use its new revision. Acknowledge an inbox answer with acknowledge_feedback before acting. Only one pending question is allowed per session; existing_pending means use the returned question rather than create another. If the question or context changed, create a new question after the previous one is terminal. Never claim a question was archived until the tool confirms it.`

func registerInboxTools(s *mcp.Server, b *bridge) {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	id := str("Question ID returned by ask_feedback or list_feedback")
	revision := map[string]any{"type": "integer", "minimum": 1, "description": "Revision from the latest question result; protects against stale answers after reopening"}
	common := func() map[string]any { return map[string]any{"question_id": id, "revision": revision} }
	add := func(name, action, desc string, props map[string]any, required ...string) {
		if required == nil {
			required = []string{}
		}
		s.AddTool(&mcp.Tool{Name: name, Description: desc, InputSchema: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, withRecover(name, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if b.taskID < 0 || b.sessionID == "" {
				return nil, fmt.Errorf("feedback requires KAMACU_TASK_ID and KAMACU_SESSION_ID from a live Kamacu task or Scratchpad agent")
			}
			var args map[string]json.RawMessage
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, fmt.Errorf("invalid feedback arguments: %w", err)
				}
			}
			for key := range args {
				if _, ok := props[key]; !ok {
					return nil, fmt.Errorf("unknown feedback argument %q", key)
				}
			}
			for _, key := range required {
				if _, ok := args[key]; !ok {
					return nil, fmt.Errorf("missing feedback argument %q", key)
				}
			}
			copyBridge := *b
			// Ordinary bridge calls fail fast after 10s. Feedback waits are bounded
			// at 20s server-side, with cancellation propagated through the SDK ctx.
			client := *b.client
			client.Timeout = 25 * time.Second
			copyBridge.client = &client
			body := req.Params.Arguments
			if len(body) == 0 || string(body) == "null" {
				body = json.RawMessage(`{}`)
			}
			return copyBridge.call(ctx, http.MethodPost, "/api/agent-feedback/"+action, bytes.NewReader(body))
		}))
	}
	add("ask_feedback", "ask", inboxInstructions, map[string]any{
		"request_key": str("Stable unique key for this question; reuse on retries to prevent duplicates"),
		"summary":     str("Required task summary, sufficient to understand what the agent is doing (max 4000 bytes)"),
		"context":     str("Required Markdown decision context: relevant findings, alternatives, tradeoffs and recommendation (max 32000 bytes)"),
		"question":    str("The specific decision the user needs to make (max 4000 bytes)"),
		"options":     map[string]any{"type": "array", "maxItems": 10, "items": map[string]any{"type": "object", "properties": map[string]any{"id": str("Stable unique option ID"), "label": str("Simple answer label")}, "required": []string{"id", "label"}, "additionalProperties": false}},
	}, "request_key", "summary", "context", "question")
	add("wait_for_feedback", "wait", "Wait up to 20 seconds for a feedback answer; renews the 5-minute heartbeat once per call. Returns pending, answered, dismissed, or expired. Process terminal input before polling again. Cancelled waits leave the question intact; expired questions are NOT reopened by polling.", map[string]any{"question_id": id, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 20}}, "question_id")
	props := common()
	props["answer"] = str("User's terminal answer; report before acting. Do not treat a request for clarification as an answer.")
	props["option_id"] = str("Alternatively the selected option ID; do not also supply answer")
	add("resolve_feedback", "resolve", "Record the user's terminal answer and archive the question. First recorded answer wins; obey the returned question state/answer, including already_resolved or stale_revision.", props, "question_id", "revision")
	props = common()
	props["user_requested"] = map[string]any{"type": "boolean", "description": "Set true ONLY when the user explicitly instructed reopening a dismissed question"}
	add("reopen_feedback", "reopen", "Explicitly reopen an unanswered question still needed by this task; transfers ownership to this session and returns a new revision. Dismissed questions require explicit user instruction. Answered questions cannot reopen.", props, "question_id", "revision")
	add("acknowledge_feedback", "acknowledge", "Confirm that the agent received the archived question result. Acknowledge inbox answers before acting; pending questions cannot be acknowledged.", common(), "question_id", "revision")
	add("list_feedback", "list", "Recover feedback questions for the current task or Scratchpad on resume, newest first, 10 per page. Does NOT renew heartbeats or reopen anything; use wait_for_feedback for a pending owned question.", map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}})
}

func feedbackIdentityFromEnv() (int64, string) {
	id, err := strconv.ParseInt(os.Getenv("KAMACU_TASK_ID"), 10, 64)
	if err != nil {
		id = -1
	}
	return id, os.Getenv("KAMACU_SESSION_ID")
}

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const inboxInstructions = `Kamacu detects agent status, questions, and completions automatically from terminal output. Ask the user and report completed work directly in the terminal with enough context to decide. Do not call ask_feedback, report_completion, or reopen_feedback for new messages. Ordinary inbox replies arrive as plain terminal input and require no acknowledgement. Compatibility tools remain for recovering messages created before terminal monitoring: only when an incoming legacy reply explicitly contains question_id and revision, acknowledge that existing reply before acting. A dismissal is never approval. HTML previews still use the artifact tools.`

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
			body := req.Params.Arguments
			if len(body) == 0 || string(body) == "null" {
				body = json.RawMessage(`{}`)
			}
			return b.call(ctx, http.MethodPost, "/api/agent-feedback/"+action, bytes.NewReader(body))
		}))
	}
	add("report_completion", "complete", "Legacy compatibility only. New completion summaries belong in the terminal and are detected automatically; do not call this tool for new work.", map[string]any{
		"request_key": str("Unique key for this completed work; reuse on retries"),
		"abstract":    map[string]any{"type": "string", "minLength": 1, "maxLength": 480, "description": "Self-contained plain-text sidebar message, up to 480 characters. Include concrete outcome and verification for completions; decision, essential findings, tradeoffs and recommendation for questions. Not a title or teaser."},
		"summary":     str("Summary of completed work (max 4000 bytes)"),
		"context":     str("Markdown details: changes, verification and remaining limitations (max 32000 bytes)"),
	}, "request_key", "abstract", "summary", "context")
	add("ask_feedback", "ask", inboxInstructions, map[string]any{
		"request_key": str("Stable unique key for this question; reuse on retries to prevent duplicates"),
		"abstract":    map[string]any{"type": "string", "minLength": 1, "maxLength": 480, "description": "Self-contained plain-text sidebar message, up to 480 characters. Include concrete outcome and verification for completions; decision, essential findings, tradeoffs and recommendation for questions. Not a title or teaser."},
		"summary":     str("Required task summary, sufficient to understand what the agent is doing (max 4000 bytes)"),
		"context":     str("Required Markdown decision context: relevant findings, alternatives, tradeoffs and recommendation (max 32000 bytes)"),
		"question":    str("The specific decision the user needs to make (max 4000 bytes)"),
		"options":     map[string]any{"type": "array", "maxItems": 10, "items": map[string]any{"type": "object", "properties": map[string]any{"id": str("Stable unique option ID"), "label": str("Simple answer label")}, "required": []string{"id", "label"}, "additionalProperties": false}},
	}, "request_key", "abstract", "summary", "context", "question")
	props := common()
	props["answer"] = str("User's terminal answer to pending feedback; report before acting. Fresh instructions after dismissal are new requests, not answers to the archive. Do not treat clarification as an answer.")
	props["option_id"] = str("Alternatively the selected option ID; do not also supply answer")
	add("resolve_feedback", "resolve", "Record the user's terminal answer to pending feedback and archive the question. First recorded answer wins; obey saved answers and review stale_revision. If the message was dismissed or expired, its empty answer does not override an explicit fresh terminal instruction; handle that as a new request without resolving or reopening the archive.", props, "question_id", "revision")
	props = common()
	props["user_requested"] = map[string]any{"type": "boolean", "description": "Set true ONLY when the user explicitly instructed reopening a dismissed question"}
	add("reopen_feedback", "reopen", "Explicitly reopen an unanswered question still needed by this task; transfers ownership to this session and returns a new revision. Dismissed questions require explicit user instruction. Answered questions cannot reopen.", props, "question_id", "revision")
	add("acknowledge_feedback", "acknowledge", "Confirm that the agent received the archived question result. Acknowledge inbox answers before acting; pending questions cannot be acknowledged.", common(), "question_id", "revision")
	add("list_feedback", "list", "Recover feedback questions for the current task or Scratchpad on resume, newest first, 10 per page. Does not reopen anything. For a pending owned question, end your turn and await terminal delivery.", map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0}})
}

func feedbackIdentityFromEnv() (int64, string) {
	id, err := strconv.ParseInt(os.Getenv("KAMACU_TASK_ID"), 10, 64)
	if err != nil {
		id = -1
	}
	return id, os.Getenv("KAMACU_SESSION_ID")
}

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

const inboxInstructions = `When work is complete, display a summary and call report_completion with changes, verification and limitations. When feedback is needed, first display the question and numbered options in the terminal, then call ask_feedback with the same question and self-contained decision context. After either tool confirms a pending message, end your turn. Do not poll or keep a tool call open: Kamacu injects inbox answers directly into the originating terminal. A delivered message includes question_id and revision; call acknowledge_feedback before acting and use the returned answer. A text answer with empty reason is the next instruction. For reason=seen, end the turn without further work or another completion. Kamacu handles reason=exit by stopping the session. For a user answer typed directly in the terminal, call resolve_feedback first; the first recorded answer wins. Clarification is not an answer. Pending questions remain open while the session is alive. A stopped session archives pending questions unanswered. On resume or delivery failure, use list_feedback to recover durable answers and acknowledge them. Reopen unanswered questions only if still needed; dismissed questions require explicit user instruction and user_requested=true. Only one question may be pending per session; existing_pending means use that question. Dismissal or expiration never implies approval.`

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
	add("report_completion", "complete", "Required whenever requested work is complete and you are waiting for further input, even without a question. Provide a concise summary and Markdown details of changes, verification and limitations. Kamacu supplies Seen and Seen and exit session actions; free text is the next prompt. End your turn after posting; Kamacu injects replies into the terminal. Acknowledge injected answers before acting. On Seen, acknowledge then end your turn without reposting. Exit is handled by Kamacu.", map[string]any{
		"request_key": str("Unique key for this completed work; reuse on retries"),
		"summary":     str("Summary of completed work (max 4000 bytes)"),
		"context":     str("Markdown details: changes, verification and remaining limitations (max 32000 bytes)"),
	}, "request_key", "summary", "context")
	add("ask_feedback", "ask", inboxInstructions, map[string]any{
		"request_key": str("Stable unique key for this question; reuse on retries to prevent duplicates"),
		"summary":     str("Required task summary, sufficient to understand what the agent is doing (max 4000 bytes)"),
		"context":     str("Required Markdown decision context: relevant findings, alternatives, tradeoffs and recommendation (max 32000 bytes)"),
		"question":    str("The specific decision the user needs to make (max 4000 bytes)"),
		"options":     map[string]any{"type": "array", "maxItems": 10, "items": map[string]any{"type": "object", "properties": map[string]any{"id": str("Stable unique option ID"), "label": str("Simple answer label")}, "required": []string{"id", "label"}, "additionalProperties": false}},
	}, "request_key", "summary", "context", "question")
	props := common()
	props["answer"] = str("User's terminal answer; report before acting. Do not treat a request for clarification as an answer.")
	props["option_id"] = str("Alternatively the selected option ID; do not also supply answer")
	add("resolve_feedback", "resolve", "Record the user's terminal answer and archive the question. First recorded answer wins; obey the returned question state/answer, including already_resolved or stale_revision.", props, "question_id", "revision")
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

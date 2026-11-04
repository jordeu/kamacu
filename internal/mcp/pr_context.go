package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPRContextTools(s *mcp.Server, b *bridge) {
	s.AddTool(&mcp.Tool{Name: "get_pr_context", Description: "Read current GitHub PR status, draft, head commit, requested reviewers, authenticated viewer and complete viewer review/approval history, plus linked implementation/review tasks and recent/live sessions with evidence-backed work summaries. Terminal observations are distinct from GitHub facts; unavailable reviews mean unknown, not no approval. Use get_session_output for deeper evidence. Accepts repository owner/repo and number, or a full PR URL in repository.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"repository": map[string]string{"type": "string"}, "number": map[string]string{"type": "integer"}}, "required": []string{"repository"}}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a struct {
			Repository string `json:"repository"`
			Number     int64  `json:"number"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
			return nil, err
		}
		q := url.Values{"repository": {a.Repository}, "number": {strconv.FormatInt(a.Number, 10)}}
		// Full review pagination has a bounded 30s server deadline.
		lookup := *b
		client := *b.client
		client.Timeout = 35 * time.Second
		lookup.client = &client
		return lookup.call(ctx, http.MethodGet, "/api/pr-context?"+q.Encode(), nil)
	})
	s.AddTool(&mcp.Tool{Name: "get_task_pr_context", Description: "Read a task's explicit PR links, roles, evidence, uncertainty and correction provenance, plus recent work summaries and live sessions. Look up each PR with get_pr_context for verified status. Incidental links are explicit rejected associations.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]string{"type": "integer"}}, "required": []string{"task_id"}}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a struct {
			TaskID int64 `json:"task_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
			return nil, err
		}
		return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/tasks/%d/pr-context", a.TaskID), nil)
	})
	s.AddTool(&mcp.Tool{Name: "set_task_pr_link", Description: "Explicitly associate/correct a task PR link, protected from inferred terminal updates. role is implementation, review, or incidental to reject an incorrect association. Provide evidence explaining the correction. Changes local context only; no GitHub writes.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]string{"type": "integer"}, "repository": map[string]string{"type": "string"}, "number": map[string]string{"type": "integer"}, "role": map[string]any{"type": "string", "enum": []string{"implementation", "review", "incidental"}}, "evidence": map[string]string{"type": "string"}, "uncertainty": map[string]string{"type": "string"}}, "required": []string{"task_id", "repository", "role", "evidence"}}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a struct {
			TaskID int64 `json:"task_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
			return nil, err
		}
		return b.call(ctx, http.MethodPut, fmt.Sprintf("/api/tasks/%d/pr-links", a.TaskID), bytes.NewReader(req.Params.Arguments))
	})
}

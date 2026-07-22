package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTaskTools registers the six task resource MCP tools (D-07
// per-resource split): list_tasks, get_task, create_task, update_task,
// move_task, delete_task. Each tool is a thin HTTP-bridge handler that
// builds a path and optional body, then delegates the response-handling
// half to bridge.call (Plan 01's shared helper). MCPTASK-01..06.
//
// The bridge performs NO validation — Kamacu's existing handlers validate
// (empty title → 400 "title is required"; invalid status → 400 "invalid
// status"; missing task → 404 "task not found"; PR-review source on /move
// → 409 "PR reviews are not board tasks"). The bridge sends fields as-is
// and lets the 400/404/409 surface verbatim through the D-05 wrap (SC4
// satisfied by NOT duplicating validation in the bridge).
//
// The low-level Server.AddTool is used with an explicit map[string]any
// InputSchema — NOT the typed generic mcp.AddTool[In, Out] helper (Phase
// 06 lock; SDK v1.6.1's Tool.InputSchema field is `any`).
func registerTaskTools(s *mcp.Server, b *bridge) {
	// 1. list_tasks (MCPTASK-01).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_tasks",
			Description: "List tasks on the Kamacu board. Returns the raw JSON array from GET /api/tasks (all manual tasks across all projects) when project_id is omitted, or GET /api/projects/{id}/tasks (the board fetch for that project) when project_id is supplied. Ordered by status then position ASC.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "Optional project id. When supplied, lists tasks for that project via GET /api/projects/{id}/tasks. When omitted, lists all manual tasks across all projects via GET /api/tasks.",
					},
				},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listTasks(ctx, req)
		},
	)

	// 2. get_task (MCPTASK-02).
	s.AddTool(
		&mcp.Tool{
			Name:        "get_task",
			Description: "Fetch a single task by id. Returns the raw JSON object from GET /api/tasks/{id}. A missing/invalid task id surfaces Kamacu's 404 body verbatim.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "integer",
						"description": "The task id.",
					},
				},
				"required": []string{"task_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.getTask(ctx, req)
		},
	)

	// 3. create_task (MCPTASK-03).
	s.AddTool(
		&mcp.Tool{
			Name:        "create_task",
			Description: "Create a new task on a project's board (status always 'todo' at the top of the To Do column). Returns the raw JSON object from POST /api/projects/{id}/tasks. The worktree + branch auto-provision synchronously (up to 30s, recorded on the task row — provisioning failure does NOT block creation, it lands in worktree_error). The agent is NOT auto-started.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project id to create the task under.",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "The task title (trimmed, must be non-empty).",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional task description (markdown).",
					},
				},
				"required": []string{"project_id", "title"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.createTask(ctx, req)
		},
	)

	// 4. update_task (MCPTASK-04, D-03 flat optional args).
	s.AddTool(
		&mcp.Tool{
			Name:        "update_task",
			Description: "Partial-PATCH a task's editable fields (title and/or description). Returns the raw JSON object from PATCH /api/tasks/{id}. Only supplied fields are written; omitted fields are left untouched. Status and position changes go through move_task exclusively.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "integer",
						"description": "The task id.",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Optional new title (trimmed, must be non-empty when supplied).",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional new description (clearing with an empty string is legal).",
					},
				},
				"required": []string{"task_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.updateTask(ctx, req)
		},
	)

	// 5. move_task (MCPTASK-05, D-06 after_id: null — top of column).
	s.AddTool(
		&mcp.Tool{
			Name:        "move_task",
			Description: "Move a task to the top of a target status column. Returns the raw JSON object from POST /api/tasks/{id}/move. The task always lands at the TOP of the target column (the bridge sends after_id: null; Kamacu computes position via MIN(position) - 1.0). PR-review tasks (source != 'manual') are rejected by Kamacu with 409.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "integer",
						"description": "The task id.",
					},
					"status": map[string]any{
						"type":        "string",
						"enum":        []string{"todo", "in_progress", "in_review", "done"},
						"description": "Target column. The task lands at the TOP of the target column.",
					},
				},
				"required": []string{"task_id", "status"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.moveTask(ctx, req)
		},
	)

	// 6. delete_task (MCPTASK-06).
	s.AddTool(
		&mcp.Tool{
			Name:        "delete_task",
			Description: "Hard-delete a task. Returns Kamacu's response (204 on success, 404 if the task id is missing). The existing gated cleanup runs server-side BEFORE the row delete: the task's running sessions (agent AND bash) are stopped, its live tmux sessions are killed, and its tmux_sessions rows are removed. Branch and worktree directory are preserved (worktree cleanup is a separate flow).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "integer",
						"description": "The task id.",
					},
				},
				"required": []string{"task_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.deleteTask(ctx, req)
		},
	)
}

// listTasks is the body of the list_tasks tool handler (MCPTASK-01). It
// calls GET /api/tasks (the unscoped manual-only list — D-01 from Plan 01)
// when project_id is omitted, or GET /api/projects/{id}/tasks (the board
// fetch) when project_id is supplied. The handler delegates the response-
// handling half to bridge.call.
func (b *bridge) listTasks(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID *int64 `json:"project_id"`
	}
	// Only unmarshal when there are arguments — an empty/nil Arguments slice
	// would error on json.Unmarshal, but the MCP SDK may deliver a request
	// with no arguments at all (e.g. tests, the SDK zero value).
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("list_tasks: invalid arguments: %w", err)
		}
	}
	if args.ProjectID != nil {
		return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/projects/%d/tasks", *args.ProjectID), nil)
	}
	return b.call(ctx, http.MethodGet, "/api/tasks", nil)
}

// getTask is the body of the get_task tool handler (MCPTASK-02). It calls
// GET /api/tasks/{id}. A missing/invalid task_id surfaces Kamacu's 404 body
// verbatim through the D-05 wrap.
func (b *bridge) getTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		TaskID int64 `json:"task_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("get_task: invalid arguments: %w", err)
	}
	return b.call(ctx, http.MethodGet, fmt.Sprintf("/api/tasks/%d", args.TaskID), nil)
}

// createTask is the body of the create_task tool handler (MCPTASK-03). It
// POSTs {title, description} to /api/projects/{id}/tasks. The agent is NOT
// auto-started (the underlying Kamacu handler never starts one); the worktree
// auto-provisions synchronously and failures land in worktree_error without
// blocking creation.
func (b *bridge) createTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID   int64  `json:"project_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("create_task: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{"title": args.Title, "description": args.Description})
	if err != nil {
		return nil, fmt.Errorf("create_task: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPost, fmt.Sprintf("/api/projects/%d/tasks", args.ProjectID), bytes.NewReader(body))
}

// updateTask is the body of the update_task tool handler (MCPTASK-04,
// D-03 flat optional args). It PATCHes only the supplied fields to
// /api/tasks/{id} — pointer fields distinguish nil (omitted) from "" (clear).
// Kamacu's partial-PATCH leaves omitted keys untouched; an empty body returns
// the current row without error.
func (b *bridge) updateTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		TaskID      int64   `json:"task_id"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("update_task: invalid arguments: %w", err)
	}
	body := map[string]any{}
	if args.Title != nil {
		body["title"] = *args.Title
	}
	if args.Description != nil {
		body["description"] = *args.Description
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("update_task: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/tasks/%d", args.TaskID), bytes.NewReader(jsonBody))
}

// moveTask is the body of the move_task tool handler (MCPTASK-05,
// D-06 after_id: null — top of column). It POSTs {status, after_id: null} to
// /api/tasks/{id}/move. The MCP tool exposes NO after_id arg (the bridge
// hardcodes it); Kamacu's handler computes the new position via
// MIN(position) - 1.0 for null after_id (top of the target column).
func (b *bridge) moveTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		TaskID int64  `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("move_task: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{"status": args.Status, "after_id": nil})
	if err != nil {
		return nil, fmt.Errorf("move_task: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPost, fmt.Sprintf("/api/tasks/%d/move", args.TaskID), bytes.NewReader(body))
}

// deleteTask is the body of the delete_task tool handler (MCPTASK-06). It
// DELETEs /api/tasks/{id}. The existing gated cleanup (stop sessions, kill
// tmux, delete row) runs unchanged through the bridge.
func (b *bridge) deleteTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		TaskID int64 `json:"task_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("delete_task: invalid arguments: %w", err)
	}
	return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/tasks/%d", args.TaskID), nil)
}

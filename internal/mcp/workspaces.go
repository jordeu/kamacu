package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerWorkspaceTools registers the workspace resource's five MCP tools
// (D-07 per-resource split):
//
//   - list_workspaces (MCPPROJ-06): GET /api/workspaces — every workspace,
//     name-sorted, returned as a JSON array (never nil).
//   - create_workspace (MCPPROJ-06): POST /api/workspaces with {name} body.
//     Kamacu rejects empty names (400 "name is required") and case-insensitive
//     duplicates (409 "a workspace with that name already exists").
//   - update_workspace (MCPPROJ-06, D-03 single-field rename): PATCH
//     /api/workspaces/{id} with {name} body (when supplied). The default
//     Personal workspace IS renamable (D-04); a case-insensitive duplicate
//     excluding self → 409.
//   - delete_workspace (MCPPROJ-06): DELETE /api/workspaces/{id}. The v1.9
//     two-guard delete runs unchanged — is_default → 409 "the default
//     workspace can't be deleted"; non-empty COUNT → 409 "move or remove its
//     N project(s) first"; empty non-default → 204 or 404.
//   - move_project_to_workspace (MCPPROJ-07): PATCH /api/projects/{id} with
//     {workspace_id} body — reuses the v1.9 transfer surface (projects.go's
//     PATCH handler WorkspaceID branch). Registered HERE (not in
//     registerProjectTools) because it is a workspace-management action
//     (transferring a project INTO a workspace); the body contains ONLY
//     workspace_id (D-03 excludes workspace_id from update_project
//     specifically because this dedicated tool owns the transfer).
//
// Each tool is a thin HTTP-bridge handler that builds a path and optional body,
// then delegates the response-handling half to bridge.call (Plan 01's shared
// helper). The bridge performs NO validation — Kamacu's existing handlers
// validate (empty name → 400 "name is required"; case-insensitive dup → 409;
// is_default delete → 409 "the default workspace can't be deleted"; non-empty
// delete → 409 "move or remove its N project(s) first"; missing workspace →
// 404 "workspace not found"; move target missing → 400 "workspace not found").
// The bridge sends fields as-is and lets the 400/404/409 surface verbatim
// through the D-05 wrap (SC4 satisfied by NOT duplicating validation in the
// bridge).
//
// The low-level Server.AddTool is used with an explicit map[string]any
// InputSchema — NOT the typed generic mcp.AddTool[In, Out] helper (Phase 06
// locked this; SDK v1.6.1's Tool.InputSchema field is `any`).
func registerWorkspaceTools(s *mcp.Server, b *bridge) {
	// 1. list_workspaces (MCPPROJ-06).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_workspaces",
			Description: "List all Kamacu workspaces. Returns the raw JSON array from GET /api/workspaces (never nil — empty array when no workspaces exist), name-sorted case-insensitively.",
			// InputSchema MUST be a JSON Schema with type:"object" (Pitfall 2:
			// AddTool panics otherwise). list_workspaces takes no args; the
			// properties map is empty.
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listWorkspaces(ctx, req)
		},
	)

	// 2. create_workspace (MCPPROJ-06).
	s.AddTool(
		&mcp.Tool{
			Name:        "create_workspace",
			Description: "Create a new Kamacu workspace by name. Returns the raw JSON object from POST /api/workspaces (201 on success). A case-insensitive duplicate of an existing workspace name is rejected 409; an empty name is rejected 400.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Workspace name. Case-insensitive duplicate of an existing workspace → 409.",
					},
				},
				"required": []string{"name"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.createWorkspace(ctx, req)
		},
	)

	// 3. update_workspace (MCPPROJ-06, D-03 single-field rename).
	s.AddTool(
		&mcp.Tool{
			Name:        "update_workspace",
			Description: "Rename a workspace. Returns the raw JSON object from PATCH /api/workspaces/{id}. The default workspace (is_default) IS renamable. A case-insensitive duplicate with a DIFFERENT workspace → 409; an empty body → 400 \"nothing to update\". A non-existent workspace_id → 404.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "The workspace id.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "New workspace name. The default workspace IS renamable. Case-insensitive duplicate (excluding self) → 409.",
					},
				},
				"required": []string{"workspace_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.updateWorkspace(ctx, req)
		},
	)

	// 4. delete_workspace (MCPPROJ-06 — v1.9 guarded delete).
	s.AddTool(
		&mcp.Tool{
			Name:        "delete_workspace",
			Description: "Delete a workspace. Returns Kamacu's response (204 on success, 404 if the workspace id is missing). The v1.9 two-guard delete runs BEFORE any mutation: the default workspace (is_default) → 409 \"the default workspace can't be deleted\"; a workspace still owning projects → 409 \"move or remove its N project(s) first\". An empty, non-default workspace is removed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "The workspace id. The default workspace (is_default) → 409. A workspace still owning projects → 409 with a count-carrying message.",
					},
				},
				"required": []string{"workspace_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.deleteWorkspace(ctx, req)
		},
	)

	// 5. move_project_to_workspace (MCPPROJ-07 — reuses v1.9 PATCH /api/projects/{id}).
	// Registered in registerWorkspaceTools (NOT registerProjectTools) because it
	// is a workspace-management action (transferring a project INTO a
	// workspace), even though it PATCHes the project route. The body contains
	// ONLY workspace_id (D-03 excludes workspace_id from update_project
	// specifically because this dedicated tool owns the transfer).
	s.AddTool(
		&mcp.Tool{
			Name:        "move_project_to_workspace",
			Description: "Transfer a project into a workspace. Returns the raw JSON object from PATCH /api/projects/{id}. Bridges to the existing project PATCH route's workspace_id branch (NOT a /api/workspaces route) — the body contains ONLY workspace_id. Kamacu validates the target workspace exists → 400 \"workspace not found\" if not.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "The project to transfer.",
					},
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "The target workspace id. Must exist → 400 \"workspace not found\" if not.",
					},
				},
				"required": []string{"project_id", "workspace_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.moveProjectToWorkspace(ctx, req)
		},
	)
}

// listWorkspaces is the body of the list_workspaces tool handler (MCPPROJ-06).
// It calls GET /api/workspaces — no args, no body. Kamacu's list handler
// returns the workspaces array (never nil — empty array when no workspaces
// exist), name-sorted case-insensitively.
func (b *bridge) listWorkspaces(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// list_workspaces takes no args — req.Params.Arguments is ignored. The
	// handler is a one-line delegate to bridge.call.
	return b.call(ctx, http.MethodGet, "/api/workspaces", nil)
}

// createWorkspace is the body of the create_workspace tool handler
// (MCPPROJ-06). It POSTs {name} to /api/workspaces. Kamacu validates the name
// (empty → 400 "name is required"; case-insensitive dup → 409 "a workspace
// with that name already exists"). The bridge performs NO validation — it
// sends the field as-is and lets Kamacu's 400/409 surface verbatim.
func (b *bridge) createWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("create_workspace: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{"name": args.Name})
	if err != nil {
		return nil, fmt.Errorf("create_workspace: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPost, "/api/workspaces", bytes.NewReader(body))
}

// updateWorkspace is the body of the update_workspace tool handler
// (MCPPROJ-06, D-03 single-field rename). It PATCHes /api/workspaces/{id} with
// {name} when supplied. The Name field is a *string so omitted = nil (Kamacu's
// PATCH handler returns 400 "nothing to update" for a nil name); supplied (incl.
// explicit "") triggers the rename. The bridge performs NO validation — Kamacu's
// existing gates (empty after trim → 400 "name is required"; case-insensitive
// dup excluding self → 409; default workspace IS renamable) apply unchanged.
//
// Per D-03 the InputSchema and args struct are single-field (name only) —
// matching the underlying PATCH shape. Workspace transfer is its own dedicated
// tool (move_project_to_workspace).
func (b *bridge) updateWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		WorkspaceID int64   `json:"workspace_id"`
		Name        *string `json:"name"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("update_workspace: invalid arguments: %w", err)
	}
	// D-03: build the body map conditionally — only non-nil fields are sent.
	// An omitted name (nil pointer) yields an empty body, which Kamacu's
	// PATCH handler rejects with 400 "nothing to update". An explicit name
	// (incl. "") triggers the rename (Kamacu trims and validates).
	body := map[string]any{}
	if args.Name != nil {
		body["name"] = *args.Name
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("update_workspace: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/workspaces/%d", args.WorkspaceID), bytes.NewReader(jsonBody))
}

// deleteWorkspace is the body of the delete_workspace tool handler
// (MCPPROJ-06). It DELETEs /api/workspaces/{id}. The v1.9 two-guard delete runs
// unchanged through the bridge: is_default → 409 "the default workspace can't
// be deleted"; non-empty COUNT → 409 "move or remove its N project(s) first";
// empty non-default → 204 or 404. A missing workspace surfaces Kamacu's 404
// body verbatim through the D-05 wrap.
func (b *bridge) deleteWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		WorkspaceID int64 `json:"workspace_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("delete_workspace: invalid arguments: %w", err)
	}
	return b.call(ctx, http.MethodDelete, fmt.Sprintf("/api/workspaces/%d", args.WorkspaceID), nil)
}

// moveProjectToWorkspace is the body of the move_project_to_workspace tool
// handler (MCPPROJ-07). It PATCHes /api/projects/{id} (NOT a /api/workspaces
// route) with a body containing ONLY workspace_id. The bridge reuses the v1.9
// transfer surface — Kamacu's PATCH /api/projects/{id} WorkspaceID branch
// (projects.go:597-615) validates the target exists via
// `SELECT 1 FROM workspaces WHERE id = ?` → 400 "workspace not found" if not.
//
// The body contains ONLY workspace_id (no name, no description, etc.) — this
// is the dedicated transfer tool. update_project (Plan 03) excludes
// workspace_id from its InputSchema and args struct specifically because this
// tool owns the transfer (D-03 / 07-RESEARCH Pitfall 3 double-lock).
func (b *bridge) moveProjectToWorkspace(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID   int64 `json:"project_id"`
		WorkspaceID int64 `json:"workspace_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("move_project_to_workspace: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{"workspace_id": args.WorkspaceID})
	if err != nil {
		return nil, fmt.Errorf("move_project_to_workspace: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPatch, fmt.Sprintf("/api/projects/%d", args.ProjectID), bytes.NewReader(body))
}

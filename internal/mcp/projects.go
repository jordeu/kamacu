package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerProjectTools registers the project resource's MCP tools (D-07
// per-resource split). Phase 07 Plan 01 ships exactly ONE project tool here —
// list_projects — inherited from Phase 06 and updated for the real
// workspace_id? arg (D-02 / MCPPROJ-01). The remaining four project tools
// (get_project / create_project / update_project / delete_project) are added
// by Plan 03 in this same file.
//
// The low-level Server.AddTool is used with an explicit map[string]any
// InputSchema — NOT the typed generic mcp.AddTool[In, Out] helper (Phase 06
// locked this; SDK v1.6.1's Tool.InputSchema field is `any`).
func registerProjectTools(s *mcp.Server, b *bridge) {
	s.AddTool(
		&mcp.Tool{
			Name:        "list_projects",
			Description: "List all Kamacu projects, optionally scoped to a workspace. Returns the raw JSON array from GET /api/projects (with ?workspace_id=N when workspace_id is supplied).",
			// InputSchema MUST be a JSON Schema with type:"object" (Pitfall 2:
			// AddTool panics otherwise). A plain map[string]any keeps the file
			// readable without pulling in jsonschema-go's typed API. The stale
			// project_id no-op arg from Phase 06 is REMOVED (07-RESEARCH
			// Pitfall 6); the real workspace_id? arg is the only property.
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"workspace_id": map[string]any{
						"type":        "integer",
						"description": "Optional workspace id. When supplied, only projects in that workspace are returned ( Kamacu appends ?workspace_id=N to GET /api/projects ).",
					},
				},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listProjects(ctx, req)
		},
	)
}

// listProjects is the body of the list_projects tool handler. It calls
// GET /api/projects, optionally with ?workspace_id=N when the caller supplies
// the workspace_id arg (D-02 / MCPPROJ-01). The handler delegates the
// response-handling half to bridge.call — transport / non-200 / 200
// passthrough behavior is identical to every other Phase 07 tool.
//
// Backward-compat note: when workspace_id is omitted (or args is empty), the
// request path is the bare /api/projects — same as Phase 06's listProjects,
// so existing callers (the SPA, Phase 06's tool invocation before this
// rename) see byte-for-byte identical responses.
func (b *bridge) listProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		WorkspaceID *int64 `json:"workspace_id"`
	}
	// Only unmarshal when there are arguments — an empty/nil Arguments slice
	// would error on json.Unmarshal, but the MCP SDK may deliver a request
	// with no arguments at all (e.g. tests, the SDK zero value).
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("list_projects: invalid arguments: %w", err)
		}
	}
	path := "/api/projects"
	if args.WorkspaceID != nil {
		path = fmt.Sprintf("/api/projects?workspace_id=%d", *args.WorkspaceID)
	}
	return b.call(ctx, http.MethodGet, path, nil)
}

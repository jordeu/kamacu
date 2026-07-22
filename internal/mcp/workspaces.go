package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerWorkspaceTools registers the workspace resource's MCP tools (D-07
// per-resource split). Plan 01 ships an EMPTY body — Plan 04 fills this with
// the five workspace tools (list_workspaces / create_workspace /
// update_workspace / delete_workspace / move_project_to_workspace) bridging
// to the existing Kamacu workspace endpoints plus the v1.9 PATCH
// /api/projects/{id} workspace_id field for the transfer tool.
//
// The shell exists so registerTools in server.go compiles as a three-line
// delegator; Plans 02/03/04 each own exactly one of the three register*Tools
// bodies, giving the wave-2 plans clean parallel boundaries (no shared file
// edits).
func registerWorkspaceTools(s *mcp.Server, b *bridge) {
	// Plan 04 adds s.AddTool calls here. Until then: nothing.
	_ = s
	_ = b
}

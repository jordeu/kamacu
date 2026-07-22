package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTaskTools registers the task resource's MCP tools (D-07 per-resource
// split). Plan 01 ships an EMPTY body — Plan 02 fills this with the six task
// tools (list_tasks / get_task / create_task / update_task / move_task /
// delete_task) bridging to the existing Kamacu task endpoints plus the
// Plan-01 D-01 unscoped GET /api/tasks route.
//
// The shell exists so registerTools in server.go compiles as a three-line
// delegator; Plans 02/03/04 each own exactly one of the three register*Tools
// bodies, giving the wave-2 plans clean parallel boundaries (no shared file
// edits).
func registerTaskTools(s *mcp.Server, b *bridge) {
	// Plan 02 adds s.AddTool calls here. Until then: nothing.
	_ = s
	_ = b
}

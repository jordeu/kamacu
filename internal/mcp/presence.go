package mcp

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPresenceTools registers the user-activity surface (read-only).
//
// The D-14 import gate holds here too: this file imports only context /
// net/http / the SDK — presence is reached over HTTP via the bridge, never
// as a Go import.
func registerPresenceTools(s *mcp.Server, b *bridge) {
	// get_user_activity — the context-awareness read. The description is
	// written for the agent deciding what to DO with the snapshot, not for
	// a human browsing tool docs.
	s.AddTool(
		&mcp.Tool{
			Name:        "get_user_activity",
			Description: "Returns what the user is currently doing in the Kamacu UI: which project/task/agent session they are viewing (viewing), whether they are active or away and for how long (active, away_for_seconds), and which sessions they are actively typing into (typing_in). Use this for context awareness: prioritize work on the task the user is focused on, avoid churning sessions the user is actively driving (they appear in typing_in), and prefer batching non-urgent questions when the user is away. All data is metadata only; no keystroke content is ever captured.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		withRecover("get_user_activity", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.getUserActivity(ctx, req)
		}),
	)
}

// getUserActivity is the body of the get_user_activity tool handler. It
// takes no arguments and bridges the token-gated GET /api/presence
// snapshot verbatim (the list_tasks passthrough pattern).
func (b *bridge) getUserActivity(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return b.call(ctx, http.MethodGet, "/api/presence", nil)
}

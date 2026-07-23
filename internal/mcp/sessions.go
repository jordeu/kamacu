package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withRecover wraps a session tool handler so a panic in the handler body is
// converted to a returned error instead of unwinding through the SDK's
// tools/call dispatch, which has NO recover of its own (verified by
// TestSC2_HandlerReturningError_ProducesCleanJSONRPCErrResponse — server.go:753
// in the SDK calls st.handler directly). This closes Phase 06 Open Question 1
// and Pitfall 2: a panic in a session handler — including Plan 03's streaming
// subscribe_session_output — surfaces as a clean JSON-RPC error.code = -32603
// (via the bridge.call non-nil err wrap) rather than desyncing the stdio
// JSON-RPC stream by crashing the subcommand.
//
// Named return values (result, err) are REQUIRED so the deferred recover can
// overwrite them before they reach the caller. A nil result + non-nil err is
// the contract the SDK turns into a -32603 response.
//
// The wrapper is applied to EVERY session tool handler closure passed to
// s.AddTool in this file (subscribe in Plan 03 gets it too — every handler
// that touches a session).
func withRecover(name string, h mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("kamacu %s: panic: %v", name, r)
				result = nil
			}
		}()
		return h(ctx, req)
	}
}

// registerSessionTools registers the non-streaming MCP session tools
// (D-07 per-resource split): list_sessions, get_session, get_session_output.
// subscribe_session_output is ADDED to this same file by Plan 03 (it has the
// streaming surface and the withRecover wrapper is already in scope here).
//
// Each tool is a thin HTTP-bridge handler that builds a path (and for
// get_session_output an optional ?bytes=N query) and delegates the response-
// handling half to bridge.call (Plan 01's shared helper). The bridge performs
// NO validation — Kamacu's existing handlers validate (404 for unknown
// session id, 400 for a malformed bytes query). SC4 is satisfied by NOT
// duplicating validation in the bridge.
//
// Read-only isolation (D-14): this file imports ONLY stdlib + the SDK. It
// does NOT import the in-repo session engine package that owns PTY lifetimes
// — the bridge speaks HTTP only; no PTY-write primitive and no WS FrameData
// type are in scope anywhere here. The acceptance-criteria grep
// (sessions.go mentions zero references to the session engine package) is
// the type-level proof.
func registerSessionTools(s *mcp.Server, b *bridge) {
	// 1. list_sessions (MCPSESS-01).
	s.AddTool(
		&mcp.Tool{
			Name:        "list_sessions",
			Description: "List live Kamacu terminal sessions (bash tabs AND agent sessions). Read-only. Returns a JSON array of session rows; orphaned restored-tmux survivor rows are filtered out so every listed id is operable. When project_id is supplied, restricts the list to sessions whose task is in that project; when task_id is supplied, restricts to sessions for that task; otherwise lists all live sessions.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "integer",
						"description": "Optional project id; restricts the list to sessions whose task is in that project.",
					},
					"task_id": map[string]any{
						"type":        "integer",
						"description": "Optional task id; restricts the list to sessions for that task.",
					},
				},
			},
		},
		withRecover("list_sessions", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.listSessions(ctx, req)
		}),
	)

	// 2. get_session (MCPSESS-02).
	s.AddTool(
		&mcp.Tool{
			Name:        "get_session",
			Description: "Fetch a single session by id. Returns the raw JSON object from GET /api/sessions/{id} (status + taskId/label + taskTitle/projectName/agentName JOIN). A missing/invalid session id surfaces Kamacu's 404 body verbatim through the HTTP NNN error wrap.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session id.",
					},
				},
				"required": []string{"session_id"},
			},
		},
		withRecover("get_session", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.getSession(ctx, req)
		}),
	)

	// 3. get_session_output (MCPSESS-03).
	s.AddTool(
		&mcp.Tool{
			Name:        "get_session_output",
			Description: "Fetch a snapshot of a session's PTY ring buffer as a base64 envelope (encoding=base64, output=<b64>, bytes=N, clamped=bool). Read-only. Default bytes=4096; clamped server-side to 512KiB. A missing session id surfaces Kamacu's 404 verbatim.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session id.",
					},
					"bytes": map[string]any{
						"type":        "integer",
						"description": "Optional: last N bytes of the PTY ring to return. Default 4096; clamped server-side to ~512KiB.",
					},
				},
				"required": []string{"session_id"},
			},
		},
		withRecover("get_session_output", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.getSessionOutput(ctx, req)
		}),
	)
}

// listSessions is the body of the list_sessions tool handler (MCPSESS-01). It
// calls GET /api/sessions, optionally with ?project_id=N or ?task_id=N. The
// Kamacu JSON array is passed through as TextContent AFTER orphaned (restored-
// tmux survivor) rows are filtered out — those rows are SPA-only markers
// (id="" + orphaned:true + tmuxName=...) that the frontend turns into a
// one-shot reattach affordance; the bridge must NOT expose them to the agent
// because they are not operable session ids (D-13).
//
// D-13 filter contract:
//   - Parse the Kamacu body as []map[string]any.
//   - Drop every entry whose "orphaned" field is truthy (bool true).
//   - Re-marshal and return a fresh TextContent.
//   - On ANY shape surprise (parse error, non-array, non-bool orphaned), fall
//     back to returning the original Kamacu body unchanged (degrade to
//     passthrough — never error on a shape surprise).
//
// project_id takes precedence over task_id when both are supplied (matches
// the existing Kamacu list_sessions handler's filter precedence; supplying
// both is rare and either resolution is fine).
func (b *bridge) listSessions(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID *int64 `json:"project_id"`
		TaskID    *int64 `json:"task_id"`
	}
	// Only unmarshal when there are arguments — an empty/nil Arguments slice
	// would error on json.Unmarshal, but the SDK may deliver a request with
	// no arguments at all (e.g. tests, the SDK zero value). Mirrors listTasks
	// at tasks.go:189.
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("list_sessions: invalid arguments: %w", err)
		}
	}
	query := ""
	if args.ProjectID != nil {
		query = fmt.Sprintf("?project_id=%d", *args.ProjectID)
	} else if args.TaskID != nil {
		query = fmt.Sprintf("?task_id=%d", *args.TaskID)
	}
	res, err := b.call(ctx, http.MethodGet, "/api/sessions"+query, nil)
	if err != nil {
		return nil, err
	}

	// D-13 orphan filter. The Kamacu list still includes orphaned rows for
	// the SPA's reattach affordance; the bridge must NOT expose them.
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		// Unexpected content shape — degrade to passthrough (never error).
		return res, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &rows); err != nil {
		// Not a JSON array — degrade to passthrough (e.g. an error body).
		return res, nil
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if isOrphanedRow(row) {
			continue
		}
		filtered = append(filtered, row)
	}
	out, err := json.Marshal(filtered)
	if err != nil {
		// Marshal should never fail for decoded JSON, but degrade safely.
		return res, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(out)},
		},
	}, nil
}

// isOrphanedRow reports whether the Kamacu session row is an orphaned
// restored-tmux survivor (id="" + orphaned:true). Returns false on any shape
// surprise so non-orphaned rows are never accidentally dropped.
func isOrphanedRow(row map[string]any) bool {
	v, ok := row["orphaned"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// getSession is the body of the get_session tool handler (MCPSESS-02). It
// calls GET /api/sessions/{id}. A missing/invalid session id surfaces Kamacu's
// 404 verbatim through bridge.call's non-2xx wrap (Phase 07 D-05).
//
// url.PathEscape prevents a malformed id from injecting path segments
// (T-08-10). The Kamacu id is opaque ASCII; PathEscape is a no-op for legal
// ids and a hard reject for everything else.
func (b *bridge) getSession(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("get_session: invalid arguments: %w", err)
	}
	escaped := url.PathEscape(args.SessionID)
	return b.call(ctx, http.MethodGet, "/api/sessions/"+escaped, nil)
}

// getSessionOutput is the body of the get_session_output tool handler
// (MCPSESS-03). It calls GET /api/sessions/{id}/output, optionally with
// ?bytes=N. The Kamacu handler (Plan 01) performs the clamping (default 4096,
// max 512KiB) and emits the D-08 envelope {encoding:"base64", output:<b64>,
// bytes:N, clamped:bool}; the bridge passes it through as TextContent
// unchanged (consistent with Phase 07's raw-JSON passthrough).
func (b *bridge) getSessionOutput(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		SessionID string `json:"session_id"`
		Bytes     *int   `json:"bytes"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("get_session_output: invalid arguments: %w", err)
	}
	escaped := url.PathEscape(args.SessionID)
	path := "/api/sessions/" + escaped + "/output"
	if args.Bytes != nil {
		path += "?bytes=" + strconv.Itoa(*args.Bytes)
	}
	return b.call(ctx, http.MethodGet, path, nil)
}

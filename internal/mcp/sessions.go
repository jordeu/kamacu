package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// subscribeTailCap is D-04's 1 MiB hard cap on the accumulated subscribe
// buffer. When the live tail exceeds this size, the handler drops from the
// FRONT and keeps the MOST RECENT subscribeTailCap bytes (what the agent most
// needs — what just happened) and sets truncated:true in the D-08 envelope.
// T-08-11 mitigation.
const subscribeTailCap = 1 << 20

// defaultSubscribeSeconds and maxSubscribeSeconds mirror the Kamacu-side
// caps (Plan 01 defaultSubscribeDuration/maxSubscribeDuration). The bridge
// clamps BEFORE building the request so a misbehaving agent CLI cannot drive
// the wire past 300s (T-08-15) — Kamacu enforces its own bound server-side,
// but the bridge-level clamp is the first line of defense.
const (
	defaultSubscribeSeconds = 30
	maxSubscribeSeconds     = 300
)

// subscribeClient is the DEDICATED streaming HTTP client for subscribe.
// D-11 / Pitfall 1: the bridge's b.client (bridge.go:53) pins a 10s Timeout
// for Phase 06's fail-fast posture on the non-streaming tools; using it for
// subscribe would silently kill every >10s tail. This client has NO Timeout
// — the SDK handler ctx (cancelled on notifications/cancelled, verified at
// go-sdk@v1.6.1/mcp/transport.go:205-218) is the cancellation mechanism.
// Request-scoped: each subscribe call builds its request with
// http.NewRequestWithContext carrying the handler ctx, so a Read on a
// cancelled request returns promptly with a context-cancellation error.
var subscribeClient = &http.Client{}

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
// — the bridge speaks HTTP only; no PTY-write primitive and no WS PTY-input
// frame type are in scope anywhere here. The acceptance-criteria grep
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

	// 4. subscribe_session_output (MCPSESS-04) — the streaming tool.
	s.AddTool(
		&mcp.Tool{
			Name:        "subscribe_session_output",
			Description: "Tail live PTY output for a session for a bounded duration (default 30s, cap 300s). Read-only — collect-and-return; cancels cleanly on notifications/cancelled, returning whatever was collected so far as a partial result. Returns ONE CallToolResult whose TextContent is a JSON envelope {encoding:\"base64\", output:<b64>, bytes:N, truncated:bool, exited:bool, exitCode?:int, stopRequested?:bool}. The buffer is capped at ~1 MiB keeping the MOST RECENT bytes (truncated:true when the cap was hit). include_history defaults false (live-only); pass true to include the ring replay as the first bytes.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session id to tail.",
					},
					"duration_seconds": map[string]any{
						"type":        "integer",
						"description": "Optional: how long to tail, in seconds. Default 30; clamped to [1, 300].",
					},
					"include_history": map[string]any{
						"type":        "boolean",
						"description": "Optional: include the ring replay as the first bytes (default false = live-only).",
					},
				},
				"required": []string{"session_id"},
			},
		},
		withRecover("subscribe_session_output", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.subscribeSessionOutput(ctx, req)
		}),
	)

	// 5. start_task_agent (MCPSESS-05) — the MCP-only reversal of the "no
	// auto-start" rule. The agent (the user's delegate) can spawn or resume
	// the agent session for a task. The browser's explicit Start button is
	// unchanged; task creation does NOT auto-spawn. The agent IS the user's
	// delegate at the same trust level as delete_task / open_review.
	s.AddTool(
		&mcp.Tool{
			Name:        "start_task_agent",
			Description: "Start the agent session for a task (spawns or resumes the task's Claude Code / opencode agent in its worktree). Bridges POST /api/sessions with {\"kind\":\"agent\",\"task_id\":N,\"resume\":bool} and returns Kamacu's 201 session-row JSON verbatim. Kamacu enforces one running agent per task — a second call surfaces 409 \"agent session already running\". resume:true resumes the task's stored agent session id; false/omitted = fresh spawn. The agent is the user's delegate — same trust level as delete_task and open_review.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "integer",
						"description": "The task id whose agent session to start. Required — agents run in the task's worktree.",
					},
					"resume": map[string]any{
						"type":        "boolean",
						"description": "Optional: resume the task's stored agent session (default false = fresh spawn).",
					},
				},
				"required": []string{"task_id"},
			},
		},
		withRecover("start_task_agent", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.startTaskAgent(ctx, req)
		}),
	)

	// 6. send_session_message (MCPSESS-06) — the MCP-only reversal of D-14's
	// read-only-terminal contract. The agent (the user's delegate) can now
	// write a prompt/message to ANOTHER session's PTY stdin via
	// POST /api/sessions/{id}/input. The browser WS interactive surface is
	// unchanged; this is an agent-delegate affordance at the same trust
	// level as start_task_agent, delete_task, and delete_project.
	//
	// D-14 import gate holds: this file still does NOT import
	// internal/session — the relaxed write prohibition lands server-side in
	// the endpoint that internally calls WriteInput, reached over HTTP only.
	s.AddTool(
		&mcp.Tool{
			Name:        "send_session_message",
			Description: "Send a message/prompt to a session by writing bytes to the session's PTY stdin. Bridges POST /api/sessions/{id}/input with {\"message\":\"<string>\"} and returns Kamacu's 200 {\"bytes_written\":N} verbatim. Kamacu appends a trailing newline if absent so the prompt fires. The browser WebSocket stays the authoritative interactive surface — this is an agent-delegate affordance at the same trust level as start_task_agent and delete_task. No content sanitization (verbatim passthrough); no bulk send (one session, one message per call — loop to send more).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "The session id to send the message to.",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "The message/prompt to write to the session's PTY. A trailing newline is appended server-side if absent so the prompt fires.",
					},
				},
				"required": []string{"session_id", "message"},
			},
		},
		withRecover("send_session_message", func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return b.sendSessionMessage(ctx, req)
		}),
	)
}

// emptySubscribeEnvelope is the D-01 strict-edge return: when the handler ctx
// is cancelled BEFORE subscribeClient.Do succeeds (cancel-before-attach),
// there is no partial buffer to return — but D-01's "returns ONE
// CallToolResult" still holds. The contract returns a valid zero-byte D-08
// envelope (NOT a transport error) so the agent CLI sees a clean result.
// The SDK surfaces this as a JSON-RPC success carrying the envelope text;
// SC3's "returns the partial stream collected so far" is satisfied because
// zero bytes were streamed before cancel.
func emptySubscribeEnvelope() *mcp.CallToolResult {
	body, err := json.Marshal(subscribeEnvelope{
		Encoding:  "base64",
		Output:    "",
		Bytes:     0,
		Truncated: false,
		Exited:    false,
	})
	if err != nil {
		// json.Marshal of a fixed struct with no pointers cannot fail in
		// practice; degrade to a static literal so this NEVER returns nil.
		body = []byte(`{"encoding":"base64","output":"","bytes":0,"truncated":false,"exited":false}`)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}
}

// subscribeEnvelope is the D-08 wire shape returned by subscribe_session_output
// as TextContent. The base64 of the accumulated PTY bytes plus metadata the
// agent can reason about (was it truncated? did the session exit mid-tail?).
// ExitCode is *int so it is omitted entirely (omitempty) when the session
// did not exit; a present ExitCode:0 is meaningful (clean exit).
type subscribeEnvelope struct {
	Encoding      string `json:"encoding"`            // always "base64"
	Output        string `json:"output"`              // base64 of the accumulated bytes
	Bytes         int    `json:"bytes"`               // raw byte count of the tail
	Truncated     bool   `json:"truncated"`           // D-04: tail cap (1 MiB) was hit
	Exited        bool   `json:"exited"`              // D-03: session exited mid-tail
	ExitCode      *int   `json:"exitCode,omitempty"`  // D-03: present iff exited
	StopRequested bool   `json:"stopRequested,omitempty"` // D-03: present iff server-requested stop
}

// subscribeSessionOutput is the body of the subscribe_session_output tool
// handler (MCPSESS-04). It diverges from the Phase 06/07 bridge pattern:
//
//   - It does NOT call b.call (which buffers the full body via
//     io.ReadAll(LimitReader(maxBodyBytes)) — bridge.go:94). Subscribe streams
//     up to 300s of live PTY output; a body-buffering helper defeats the
//     point and caps the tail before it can grow naturally.
//   - It does NOT use b.client (the bridge's *http.Client with a 10s Timeout
//     at bridge.go:53). A 30s+ tail would time out at 10s every time
//     (Pitfall 1). It uses the package-level subscribeClient (no Timeout)
//     instead — the SDK handler ctx is the cancellation mechanism.
//
// Flow (D-01 collect-and-return + D-03 exit marker + D-04 cap):
//  1. Parse + validate session_id; clamp duration_seconds to [1, 300].
//  2. Build the streaming GET with the handler ctx (NewRequestWithContext)
//     so a ctx cancel propagates to a closed connection (SC3).
//  3. Cancel-before-attach guard (D-01 strict-edge): if subscribeClient.Do
//     returns a context.Cancellation error BEFORE the stream attached,
//     return (emptySubscribeEnvelope, nil) — a clean result, not a transport
//     error.
//  4. Accumulate raw octets in 8 KiB chunks; trim-front on overflow so the
//     buffer holds the MOST RECENT subscribeTailCap bytes (D-04). resp.Body.Read
//     returns io.EOF on stream close AND a context-cancellation error on
//     ctx cancel — both break the loop and fall through to envelope assembly.
//     No separate select needed: the request carries the ctx, so Read
//     unblocks on cancel just as on EOF.
//  5. Follow-up GET /api/sessions/{id} for the exit marker (08-RESEARCH Open
//     Q2 option b). Uses b.do (the 10s client — fine for a quick info GET);
//     a 404 or transport error leaves exited=false (the session's final
//     state is unknown; the partial stream is still the load-bearing result
//     per SC3). Never let a follow-up failure turn the subscribe into an error.
//  6. Assemble the D-08 envelope: base64 the accumulated buffer, marshal the
//     envelope, return (*CallToolResult, nil).
//
// Read-only contract (D-14): this method issues ONLY HTTP GETs (the streaming
// subscribe + the follow-up info). It does NOT spawn a goroutine — the
// handler body is the reader (request-scoped; dies with the tool call).
func (b *bridge) subscribeSessionOutput(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		SessionID       string `json:"session_id"`
		DurationSeconds *int   `json:"duration_seconds"`
		IncludeHistory  *bool  `json:"include_history"`
	}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("subscribe_session_output: invalid arguments: %w", err)
		}
	}
	if args.SessionID == "" {
		return nil, errors.New("subscribe_session_output: session_id is required")
	}
	// Clamp duration: default 30s; floor 1s; ceiling maxSubscribeSeconds (300).
	effective := defaultSubscribeSeconds
	if args.DurationSeconds != nil {
		effective = *args.DurationSeconds
	}
	if effective < 1 {
		effective = 1
	}
	if effective > maxSubscribeSeconds {
		effective = maxSubscribeSeconds
	}
	includeHistory := args.IncludeHistory != nil && *args.IncludeHistory

	// Build the streaming GET. The request carries the handler ctx — when the
	// SDK cancels it (notifications/cancelled), the in-flight Do and any
	// subsequent Read return promptly with a context-cancellation error.
	// This is the SC3 cancellation primitive (D-01/D-03).
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		b.base+"/api/sessions/"+url.PathEscape(args.SessionID)+fmt.Sprintf("/subscribe?duration_seconds=%d&include_history=%v", effective, includeHistory),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("kamacu subscribe: build request: %w", err)
	}
	httpReq.Header.Set("X-Kamacu-Token", b.token)

	resp, err := subscribeClient.Do(httpReq)
	if err != nil {
		// Cancel-before-attach guard (D-01 strict-edge): if the handler ctx
		// was cancelled before/during subscribeClient.Do succeeded (the stream
		// never attached; there is no partial buffer to return), return an
		// empty D-08 envelope as a normal result — NOT a transport error.
		// D-01's "returns ONE CallToolResult" holds even when zero bytes were
		// streamed. Only a non-cancellation Do error is a real transport error.
		if errors.Is(err, context.Canceled) {
			return emptySubscribeEnvelope(), nil
		}
		return nil, fmt.Errorf("kamacu subscribe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) // bounded error body
		return nil, fmt.Errorf("kamacu GET /api/sessions/%s/subscribe: HTTP %d: %s", args.SessionID, resp.StatusCode, string(body))
	}

	// D-04 cap: accumulate in fixed-size 8 KiB chunks; trim-front on overflow
	// so the buffer holds the MOST RECENT subscribeTailCap bytes. Incremental
	// trim bounds memory tightly; the trim cost is amortized across reads.
	// (08-RESEARCH Open Q3 recommendation — fixed-size chunks with trim-front.)
	buf := make([]byte, 0, subscribeTailCap)
	truncated := false
	chunk := make([]byte, 8192)
	for {
		n, readErr := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if len(buf) > subscribeTailCap {
				buf = buf[len(buf)-subscribeTailCap:]
				truncated = true
			}
		}
		if readErr != nil {
			// io.EOF (stream closed by Kamacu — duration/exit/disconnect)
			// AND context.Canceled (the handler ctx was cancelled — the SDK
			// sent notifications/cancelled) both break the loop here. The
			// partial buffer accumulated so far is the SC3 result.
			break
		}
	}

	// Exit marker via a follow-up GET (08-RESEARCH Open Q2 option b). Option
	// (b) avoids introducing sentinel-byte or length-prefix framing into the
	// raw application/octet-stream (which would complicate the D-08 envelope
	// contract and risk mis-splitting PTY bytes that happen to match a
	// sentinel). The extra loopback GET is acceptable for v1.11 single-user
	// localhost.
	exited := false
	var exitCode *int
	stopRequested := false
	if info, infoErr := b.do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(args.SessionID), nil); infoErr == nil {
		defer info.Body.Close()
		if info.StatusCode == http.StatusOK {
			infoBody, _ := io.ReadAll(io.LimitReader(info.Body, 4096))
			var infoStruct struct {
				Status        string `json:"status"`
				ExitCode      *int   `json:"exitCode"`
				StopRequested bool   `json:"stopRequested"`
			}
			if json.Unmarshal(infoBody, &infoStruct) == nil {
				exited = infoStruct.Status == "exited"
				exitCode = infoStruct.ExitCode
				stopRequested = infoStruct.StopRequested
			}
		}
	}
	// A follow-up failure (404 — session reaped mid-subscribe, or transport
	// error) leaves exited=false. The session's final state is unknown; the
	// envelope still carries the accumulated bytes + truncated flag. Never
	// let a follow-up failure turn the subscribe into an error — the partial
	// stream is the load-bearing result (SC3).

	// Assemble the D-08 subscribe envelope.
	envelope := subscribeEnvelope{
		Encoding:      "base64",
		Output:        base64.StdEncoding.EncodeToString(buf),
		Bytes:         len(buf),
		Truncated:     truncated,
		Exited:        exited,
		ExitCode:      exitCode,
		StopRequested: stopRequested,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		// Marshal of a fixed struct should never fail; surface as an error so
		// the agent CLI sees something rather than a silent nil.
		return nil, fmt.Errorf("kamacu subscribe: marshal envelope: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}, nil
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

// startTaskAgent is the body of the start_task_agent tool handler. It POSTs
// {"kind":"agent","task_id":N,"resume":bool} to /api/sessions and returns the
// 201 session-row JSON verbatim via bridge.call (D-05 passthrough). This is
// the MCP-only reversal of the "no auto-start" rule: the agent (the user's
// delegate) can now spawn the agent session for a task. The browser's Start
// button is unchanged.
//
// The bridge performs NO validation. Kamacu's (*sessionHandler).create
// (internal/api/sessions.go:222) enforces server-side: task existence (404),
// the one-running-agent-per-task dedup (409 "agent session already running"),
// worktree presence (409 "task has no worktree"), and resume-presence (409
// "no session to resume"). Each surfaces as a D-05 wrapped error — the bridge
// adds NONE of that logic.
//
// resume is always sent (bool, default false) — mirrors moveTask always
// sending after_id. The handler decodes resume with a false zero value, so
// omitting it would be equivalent; sending it keeps the body self-describing.
func (b *bridge) startTaskAgent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		TaskID int64 `json:"task_id"`
		Resume bool  `json:"resume"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("start_task_agent: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"kind":    "agent",
		"task_id": args.TaskID,
		"resume":  args.Resume,
	})
	if err != nil {
		return nil, fmt.Errorf("start_task_agent: marshal body: %w", err)
	}
	return b.call(ctx, http.MethodPost, "/api/sessions", bytes.NewReader(body))
}

// sendSessionMessage is the body of the send_session_message tool handler. It
// POSTs {"message":"<string>"} to /api/sessions/{id}/input and returns
// Kamacu's 200 {"bytes_written":N} verbatim via bridge.call (D-05 passthrough).
// This is the MCP-only reversal of D-14's read-only-terminal contract: the
// agent (the user's delegate) can now write to a session's PTY stdin. The
// browser WS interactive surface is unchanged.
//
// The bridge performs NO validation and NO content sanitization. Kamacu's
// (*sessionHandler).input enforces server-side (404 unknown, 409 exited,
// newline append) — each surfaces as a D-05 wrapped error.
//
// D-14 import gate holds: this file still does NOT import internal/session.
// The write prohibition is relaxed ONLY via the HTTP bridge path landing on
// the endpoint that internally calls WriteInput. The bridge has ZERO direct
// WriteInput/FrameData refs.
func (b *bridge) sendSessionMessage(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("send_session_message: invalid arguments: %w", err)
	}
	body, err := json.Marshal(map[string]any{"message": args.Message})
	if err != nil {
		return nil, fmt.Errorf("send_session_message: marshal body: %w", err)
	}
	escaped := url.PathEscape(args.SessionID)
	return b.call(ctx, http.MethodPost, "/api/sessions/"+escaped+"/input", bytes.NewReader(body))
}

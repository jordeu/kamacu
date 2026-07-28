---
quick_id: 260728-qsf
slug: add-mcp-tool-send-session-message-sessio
description: >
  Add an MCP tool `send_session_message(session_id, message)` that lets an
  agent send a message/prompt to an agent session by writing bytes to the
  session's PTY. This REVERSES the v1.11 D-14 read-only-terminal contract for
  the agent delegate surface ONLY — the browser WebSocket stays the
  authoritative interactive surface. The browser's interactive UX is unchanged.

must_haves:
  truths:
    # Endpoint contract
    - "POST /api/sessions/{id}/input exists, accepts a JSON body {\"message\":\"<string>\"}, looks up the session via h.mgr.Get(id), returns 404 {\"error\":\"session not found\"} on an unknown id, and 200 {\"bytes_written\":N} on success where N == len(message after newline normalization)."
    - "The endpoint appends a single '\\n' to the message when its last byte is not already '\\n', so a prompt actually fires (claude/opencode wait on Enter) rather than sitting unread in the input buffer. A message already ending in '\\n' is passed through with NO double newline. An empty message becomes just '\\n' (a bare Enter — a legitimate default-prompt answer)."
    - "The endpoint calls sess.WriteInput([]byte(msg)) — the EXISTING primitive at internal/session/session.go:368, the same code path the browser WS handler calls at internal/ws/handler.go:164. No new PTY-write code is introduced; only a new caller over a new transport (HTTP instead of WS)."
    - "An EXITED in-memory session (still reachable via mgr.Get per D-12) rejects the write: WriteInput returns 'session exited', surfaced as 409 {\"error\":\"session exited\"} (mirrors the project's state-conflict-is-409 convention: 'agent session already running', 'session is still running')."
    # MCP tool contract
    - "The MCP tool send_session_message(session_id string, message string) is registered inside registerSessionTools in internal/mcp/sessions.go (alongside the existing 5 session tools), wraps its handler closure in withRecover (the file convention for EVERY session tool), and delegates to bridge.call(ctx, http.MethodPost, \"/api/sessions/\"+url.PathEscape(id)+\"/input\", body) — a verbatim D-05 passthrough."
    - "send_session_message performs NO validation, NO content sanitization, NO quoting/escaping, NO rate-limiting, NO bulk/broadcast. One session, one message per call. The agent loops if it needs to send more. The message is verbatim passthrough; newline normalization happens server-side."
    # D-14 reversal boundary (the security-critical invariant)
    - "The D-14 IMPORT gate holds: internal/mcp/sessions.go still does NOT import internal/session. The bridge speaks HTTP only; the relaxed write prohibition lands server-side in internal/api/sessions.go (which already imports internal/session), never in the bridge. The bridge still has ZERO direct references to WriteInput or FrameData."
    - "The browser WebSocket interactive surface is byte-for-byte unchanged — ZERO files under web/src/ or internal/ws/ are modified. The browser remains the authoritative interactive surface; send_session_message is an agent-delegate affordance at the SAME trust level as start_task_agent, delete_task, and delete_project (any agent Kamacu spawned, gated by the inherited KAMACU_HOOK_TOKEN envelope + loopback binding)."
    # Integrity invariants (inherited from the milestone)
    - "internal/mcp/sessions.go writes NOTHING to os.Stdout (Phase 06 D-12 intact); slog stays on stderr."
    - "internal/api/sessions.go adds ONLY the input handler + one route line in SessionRoutes(); no other handler, route, or signature changes."
  key_artifacts:
    - "internal/api/sessions.go: new method func (h *sessionHandlers) input(w, r) + one new route line `mux.HandleFunc(\"POST /api/sessions/{id}/input\", s.input)` inside SessionRoutes()."
    - "internal/mcp/sessions.go: new method func (b *bridge) sendSessionMessage(ctx, req) + one new s.AddTool block (Name \"send_session_message\") as entry #6 inside registerSessionTools."
    - "internal/api/sessions_test.go: TestInput_* (happy round-trip via output endpoint, unknown-id 404, newline-already-present no-double-append, empty-message writes bare newline)."
    - "internal/mcp/sessions_test.go: TestBridge_SendSessionMessage_* (happy POST path+body+passthrough, unknown-id 404 D-05 wrap, empty-message still posted)."
---

# Quick Task: Add `send_session_message` MCP tool (D-14 write-prohibition reversal, agent-delegate surface only)

## Objective

Ship the `send_session_message(session_id, message)` MCP tool and its backing
Kamacu endpoint `POST /api/sessions/{id}/input`. An agent running inside a
Kamacu task PTY can now send a prompt/message to ANOTHER session's PTY stdin —
the one capability v1.11's D-14 read-only-terminal contract explicitly
prohibited. The reversal is **MCP-only**: the browser WebSocket stays the
authoritative interactive surface and is byte-for-byte unchanged.

This closes the last gap between "the agent can observe sibling sessions" (v1.11
shipped: `get_session_output`, `subscribe_session_output`) and "the agent can
drive sibling sessions" (this task). The agent is the user's delegate at the
same trust level as `start_task_agent`, `delete_task`, and `delete_project`.

Output:
  - `internal/api/sessions.go`        (MODIFIED — new `input` handler + 1 route line)
  - `internal/mcp/sessions.go`        (MODIFIED — new `sendSessionMessage` + 1 AddTool block)
  - `internal/api/sessions_test.go`   (MODIFIED — 4 endpoint tests)
  - `internal/mcp/sessions_test.go`   (MODIFIED — 3 tool tests)

## Background

**Why D-14 existed, and why it is now reversed only for the MCP surface.**
D-14's read-only-terminal contract (Phase 08) enforced a scoped grep gate:
`internal/mcp/sessions.go` has ZERO references to `WriteInput`/`FrameData` and
does NOT import `internal/session`. The rationale (PROJECT.md Out-of-Scope:
"MCP write to live PTYs (keystroke injection)") was that an agent reading a
malicious tool result could be prompt-injected into driving a sibling session
to take destructive actions. That rationale is **acknowledged, not fixed here**.

The reversal is scoped and principled:
- The **trust boundary is unchanged**: "any agent Kamacu spawned", gated by the
  inherited `KAMACU_HOOK_TOKEN` envelope + loopback binding (the same gate
  `delete_task` and `delete_project` already use). A caller without the token
  cannot reach the endpoint.
- The **D-14 import gate still holds**: the bridge still does NOT import
  `internal/session`. The write lands server-side in `internal/api/sessions.go`
  (which already imports `internal/session`), reached over HTTP. The bridge has
  no new coupling to the session engine.
- The **browser WS surface is untouched** — zero files under `web/src/` or
  `internal/ws/` change. The browser remains authoritative for interactive use;
  `send_session_message` is an agent-delegate affordance.

**The PTY-write primitive already exists** — `(*Session).WriteInput` at
`internal/session/session.go:368`. The browser WS handler calls it at
`internal/ws/handler.go:164` (`_ = sess.WriteInput(data[1:])` on `FrameData`
frames). This task reuses that exact primitive via a new HTTP caller. No new
PTY-write code is written.

**Mechanical shape — repeat the milestone's bridge pattern.** The MCP tool is a
thin `bridge.call` POST (the same shape as `startTaskAgent` at
`internal/mcp/sessions.go:587-603`, the closest POST-with-body analog). The
endpoint mirrors the existing `(*sessionHandlers).getSession` /
`.getSessionOutput` lookup-then-act shape at `internal/api/sessions.go:875-937`.

**Correction to the orchestrator brief:** the brief said "Route it in
`internal/api/routes.go` next to the existing session routes." Session routes
are NOT in `routes.go` — they are registered in `SessionRoutes()` at
`internal/api/sessions.go:32-48` (confirmed: `routes.go` registers only
workspaces/agents/projects/tasks/settings; `cmd/kamacu/serve.go:261` calls
`SessionRoutes` separately). The new route goes in `SessionRoutes()`. **`routes.go`
is NOT modified.**

## Security Note (acknowledged risk, NOT fixed in this task)

The prompt-injection escalation path (agent A reads a malicious tool result →
injected → calls `send_session_message` on agent B → drives B to destructive
actions) is a REAL risk and is **intentionally not mitigated technically here**.
The mitigation is the user's authority model: the user trusts the agent they
spawned with the same authority they themselves have. This is identical to the
posture of `start_task_agent` (260728-q5k), `delete_task`, and `delete_project`.
Documenting this as an acknowledged risk is the correct posture for a quick
task; technical rate-limiting / send-quotas / inter-agent permission prompts are
out of scope.

## Tasks

<task type="auto" tdd="false">
  <name>Task 1 (feat/api): Add POST /api/sessions/{id}/input handler + route</name>
  <files>internal/api/sessions.go</files>

  <read_first>
    - internal/api/sessions.go:32-48 (SessionRoutes — the route registration site; add the new route HERE, not in routes.go)
    - internal/api/sessions.go:501-509 (the stop handler — the closest {id}-POST analog: mgr.Get → 404-on-unknown → act)
    - internal/api/sessions.go:875-890 (getSession — the read-side {id} lookup shape to mirror for the 404 path)
    - internal/api/sessions.go:539-551 (rename — the JSON-decode-body + writeJSON/writeError shape to copy)
    - internal/session/session.go:365-381 (WriteInput — the primitive being reused; note it returns "session exited" on an exited session)
    - internal/ws/handler.go:160-164 (the browser's caller of WriteInput — proves the primitive is the shared code path)
    - internal/api/respond.go (writeJSON/writeError — already in scope, no import needed)
  </read_first>

  <behavior>
    - Happy path: known live session id + {"message":"echo hi"} → 200 {"bytes_written": <len("echo hi")+1>} (newline appended); the bytes reach the PTY (verifiable via GET .../output).
    - Unknown id: → 404 {"error":"session not found"} (same copy as getSession/stop).
    - Newline already present: {"message":"hi\n"} → 200 {"bytes_written": 3} (NO double newline).
    - Empty message: {"message":""} → 200 {"bytes_written": 1} (just "\n", a bare Enter).
    - Exited session: → 409 {"error":"session exited"} (WriteInput's error verbatim).
    - Malformed JSON body: → 400 {"error":"invalid JSON body"} (same copy as rename).
  </behavior>

  <action>
    EDIT internal/api/sessions.go in two places (same file — NO changes to routes.go):

    (a) Add ONE route line inside SessionRoutes() (the func at lines 32-48),
    next to the other {id}-scoped session routes (e.g. right after the
    `POST /api/sessions/{id}/stop` line at :36, or grouped with the Phase 08
    read-only routes — either is fine; keep it adjacent to its siblings):

        mux.HandleFunc("POST /api/sessions/{id}/input", s.input)

    (b) Add the handler method on *sessionHandlers. Place it near the other
    {id} handlers (a natural home is right after the `stop` handler at
    lines 501-509, or after the Phase 08 read-only block). The handler:

        // input handles POST /api/sessions/{id}/input — writes a message to
        // the session's PTY via the existing WriteInput primitive (the same
        // code path the browser WS handler uses at internal/ws/handler.go:164).
        // This is the server-side half of the v1.11 D-14 read-only-terminal
        // REVERSAL for the agent delegate surface: the MCP bridge reaches the
        // PTY-write primitive through this HTTP endpoint, NOT through the WS
        // frame protocol. The browser WS interactive surface is unchanged.
        //
        // Newline normalization is the ONLY "smart" behavior: a trailing '\n'
        // is appended when the message does not already end in one, so a prompt
        // actually fires rather than sitting in the input buffer. No quoting,
        // escaping, or content sanitization — verbatim passthrough.
        func (h *sessionHandlers) input(w http.ResponseWriter, r *http.Request) {
            sess, ok := h.mgr.Get(r.PathValue("id"))
            if !ok {
                writeError(w, http.StatusNotFound, "session not found")
                return
            }
            var req struct {
                Message string `json:"message"`
            }
            if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeError(w, http.StatusBadRequest, "invalid JSON body")
                return
            }
            msg := req.Message
            if !strings.HasSuffix(msg, "\n") {
                msg += "\n"
            }
            if err := sess.WriteInput([]byte(msg)); err != nil {
                writeError(w, http.StatusConflict, err.Error())
                return
            }
            writeJSON(w, http.StatusOK, map[string]int{"bytes_written": len(msg)})
        }

    No new imports are needed: `strings` is already imported (sessions.go:17),
    `encoding/json` (:8), and writeJSON/writeError live in the package
    (respond.go). Verify the file compiles + vets: `go build ./internal/api/...
    && go vet ./internal/api/...`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./internal/api/... && go vet ./internal/api/...</automated>
  </verify>

  <done>
    internal/api/sessions.go compiles and vets clean; SessionRoutes registers
    POST /api/sessions/{id}/input; the input handler does mgr.Get→404, decodes
    {"message"}, appends "\n" only when absent, calls sess.WriteInput (409 on
    its error), and returns 200 {"bytes_written":N}. No other handler/route/
    signature changed. routes.go untouched.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 2 (test/api): Endpoint tests for POST /api/sessions/{id}/input</name>
  <files>internal/api/sessions_test.go</files>

  <read_first>
    - internal/api/sessions.go (the Task 1 handler — confirms exact response shape + status codes)
    - internal/api/sessions_test.go:30-56 (newSessionServer helper — REUSE; returns (srv, mgr) with real bash spawns; cleanup stops all sessions)
    - internal/api/sessions_test.go:1489-1532 (TestGetSession_Happy — the {id}-endpoint happy-path scaffold: spawn dev session via POST /api/sessions, grab sid, call the endpoint, assert)
    - internal/api/sessions_test.go:1536-1546 (TestGetSession_UnknownID_404 — the 404 scaffold: uuid.NewString id, assert status + error copy)
    - internal/api/sessions_test.go:1580-1646 (TestGetSessionOutput_Happy — the round-trip pattern: write input then read via output endpoint with a poll loop to confirm the bytes reached the PTY)
    - internal/api/sessions_test.go:1189-1228 (TestSessionStop — doJSON usage on a {id} POST route)
  </read_first>

  <behavior>
    - TestInput_Happy_WritesAndAppendsNewline: POST /api/sessions (empty body) → sid; POST .../input {"message":"echo qsf-input-marker"} → 200, bytes_written == len("echo qsf-input-marker")+1; round-trip GET .../output contains "qsf-input-marker" (proves WriteInput was actually called, not just 200 returned).
    - TestInput_UnknownID_404: POST .../<uuid>/input {"message":"hi"} → 404 {"error":"session not found"}.
    - TestInput_NewlineAlreadyPresent_NoDoubleAppend: {"message":"hi\n"} → bytes_written == 3.
    - TestInput_EmptyMessage_WritesBareNewline: {"message":""} → bytes_written == 1.
  </behavior>

  <action>
    Append FOUR tests to internal/api/sessions_test.go (same package `api`).
    Reuse newSessionServer(t) and doJSON(t, method, url, body) — do NOT
    redefine either. Model the happy-path scaffold on TestGetSession_Happy
    (:1489) and the round-trip poll on TestGetSessionOutput_Happy (:1580).

    1. TestInput_Happy_WritesAndAppendsNewline — srv, mgr := newSessionServer(t).
       Spawn a dev bash session: status, body := doJSON(t, "POST",
       srv.URL+"/api/sessions", nil); sid := body["id"].(string); grab sess via
       mgr.Get(sid); t.Cleanup(sess.Stop). Send: status2, body2 := doJSON(t,
       "POST", srv.URL+"/api/sessions/"+sid+"/input", map[string]any{"message":
       "echo qsf-input-marker"}). Assert status2==200. Compute wantBytes :=
       len("echo qsf-input-marker")+1; assert body2["bytes_written"]==float64(wantBytes).
       Round-trip (proves WriteInput fired, modeled on TestGetSessionOutput_Happy):
       poll GET srv.URL+"/api/sessions/"+sid+"/output" up to ~2s decoding the
       base64 output envelope; assert the decoded bytes contain
       "qsf-input-marker". (The PTY echoes the typed line AND the echo command
       output, so the marker appears twice — Contains is sufficient.)

    2. TestInput_UnknownID_404 — srv, _ := newSessionServer(t); status, body :=
       doJSON(t, "POST", srv.URL+"/api/sessions/"+uuid.NewString()+"/input",
       map[string]any{"message":"hi"}); assert status==404 and
       body["error"]=="session not found".

    3. TestInput_NewlineAlreadyPresent_NoDoubleAppend — spawn dev session;
       POST .../input {"message":"hi\n"}; assert 200 and bytes_written==3
       (len("hi\n"), no extra newline appended).

    4. TestInput_EmptyMessage_WritesBareNewline — spawn dev session; POST
       .../input {"message":""}; assert 200 and bytes_written==1 (just "\n").

    Run: `go test ./internal/api/ -run TestInput_ -v -count=1`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/api/ -run TestInput_ -v -count=1</automated>
  </verify>

  <done>
    Four TestInput_* tests pass: happy (with output round-trip proving WriteInput
    fired), unknown-id 404, newline-already-present no-double-append, and
    empty-message writes bare newline. Tests reuse newSessionServer + doJSON;
    no new helpers defined.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 3 (feat/mcp): Add send_session_message tool handler + registration</name>
  <files>internal/mcp/sessions.go</files>

  <read_first>
    - internal/mcp/sessions.go:93-221 (registerSessionTools — add the new AddTool block as entry #6, after start_task_agent; copy the withRecover-wrapped closure shape verbatim)
    - internal/mcp/sessions.go:587-603 (startTaskAgent — the closest POST-with-body analog: unmarshal args → json.Marshal body map → bytes.NewReader → bridge.call POST. sendSessionMessage is this shape with a different path + body)
    - internal/mcp/sessions.go:537-546 (getSession — the url.PathEscape(id) pattern to reuse for session_id path interpolation; T-08-10 path-injection mitigation)
    - internal/mcp/bridge.go:88-106 (bridge.call — the D-05 passthrough helper; sendSessionMessage delegates to it verbatim, like startTaskAgent)
  </read_first>

  <behavior>
    - Happy: send_session_message(session_id="s1", message="run tests") → bridge.call POSTs /api/sessions/s1/input with body {"message":"run tests"}, returns Kamacu's 200 {"bytes_written":N} verbatim as TextContent, IsError=false.
    - Unknown id (Kamacu 404): → non-nil error wrapping "HTTP 404" (D-05).
    - Empty message: bridge still POSTs {"message":""} verbatim (NO client-side validation); Kamacu's newline normalization handles it.
  </behavior>

  <action>
    EDIT internal/mcp/sessions.go in two places (same file — NO server.go edit;
    registerSessionTools is already wired at server.go:88):

    (a) Add a 6th s.AddTool block inside registerSessionTools (the func at
    lines 93-221), AFTER the start_task_agent block (entry #5, :198-220). Copy
    the AddTool + withRecover-wrapped-closure shape verbatim from entry #5.
    Fields:

        Name:        "send_session_message"
        Description: "Send a message/prompt to a session by writing bytes to the session's PTY stdin. Bridges POST /api/sessions/{id}/input with {\"message\":\"<string>\"} and returns Kamacu's 200 {\"bytes_written\":N} verbatim. Kamacu appends a trailing newline if absent so the prompt fires. The browser WebSocket stays the authoritative interactive surface — this is an agent-delegate affordance at the same trust level as start_task_agent and delete_task. No content sanitization (verbatim passthrough); no bulk send (one session, one message per call — loop to send more)."
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "session_id": map[string]any{"type":"string","description":"The session id to send the message to."},
                "message":    map[string]any{"type":"string","description":"The message/prompt to write to the session's PTY. A trailing newline is appended server-side if absent so the prompt fires."},
            },
            "required": []string{"session_id", "message"},
        }
        handler closure: withRecover("send_session_message", func(ctx, req) (...) { return b.sendSessionMessage(ctx, req) })

    (b) Add the handler method on *bridge, modeled on startTaskAgent (:587-603)
    but with url.PathEscape (copy from getSession :544) since session_id is a
    string path segment:

        // sendSessionMessage is the body of the send_session_message tool
        // handler. It POSTs {"message":"<string>"} to
        // /api/sessions/{id}/input and returns Kamacu's 200 {"bytes_written":N}
        // verbatim via bridge.call (D-05 passthrough). This is the MCP-only
        // reversal of D-14's read-only-terminal contract: the agent (the
        // user's delegate) can now write to a session's PTY stdin. The browser
        // WS interactive surface is unchanged.
        //
        // The bridge performs NO validation and NO content sanitization.
        // Kamacu's (*sessionHandler).input enforces server-side (404 unknown,
        // 409 exited, newline append) — each surfaces as a D-05 wrapped error.
        //
        // D-14 import gate holds: this file still does NOT import
        // internal/session. The write prohibition is relaxed ONLY via the HTTP
        // bridge path landing on the endpoint that internally calls
        // WriteInput. The bridge has ZERO direct WriteInput/FrameData refs.
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

    No new imports needed: bytes, context, encoding/json, fmt, net/http, net/url
    are ALL already imported at sessions.go:3-15. Verify: `go build
    ./internal/mcp/... && go vet ./internal/mcp/...`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./internal/mcp/... && go vet ./internal/mcp/...</automated>
  </verify>

  <done>
    send_session_message registers (entry #6 in registerSessionTools), the
    handler delegates verbatim to bridge.call POST /api/sessions/{id}/input
    with url.PathEscape + bytes.NewReader body; withRecover wraps the closure;
    the file still imports only stdlib + the SDK (no internal/session). server.go
    untouched (registerSessionTools already wired).
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 4 (test/mcp): Tool tests for send_session_message</name>
  <files>internal/mcp/sessions_test.go</files>

  <read_first>
    - internal/mcp/sessions.go (the Task 3 handler — confirms exact method name sendSessionMessage + the bridge.call delegation)
    - internal/mcp/sessions_test.go:825-870 (TestBridge_StartTaskAgent_Happy_PostsAgentBody — the POST-with-body happy scaffold to copy: capture gotPath/gotMethod/gotBody, httptest 200/201, assert path+method+body+passthrough)
    - internal/mcp/sessions_test.go:903-925 (TestBridge_StartTaskAgent_AlreadyRunning_Kamacu409 — the D-05 404/409 wrap assertion to copy: assert err != nil + "HTTP NNN" substring + Kamacu message survives)
    - internal/mcp/sessions_test.go:39 + bridge_test.go:19-21 (the canonical *bridge literal + newCallToolRequest — REUSE both, same package)
  </read_first>

  <behavior>
    - TestBridge_SendSessionMessage_Happy_PostsInputPath: fake 200 {"bytes_written":14}; assert POST /api/sessions/s1/input, body {"message":"run tests"}, TextContent passthrough, IsError=false.
    - TestBridge_SendSessionMessage_UnknownID_Kamacu404: fake 404 {"error":"session not found"}; assert non-nil err containing "HTTP 404".
    - TestBridge_SendSessionMessage_EmptyMessage_StillPosted: fake 200; args {"session_id":"s1","message":""}; assert body posted is {"message":""} (bridge does NO validation — empty message reaches Kamacu verbatim).
  </behavior>

  <action>
    Append THREE tests to internal/mcp/sessions_test.go (same package `mcp`).
    Reuse newCallToolRequest (bridge_test.go:19-21) and the canonical *bridge
    literal `&bridge{base: srv.URL, token: "t", client: &http.Client{Timeout:
    10 * time.Second}}`. Model the scaffold on TestBridge_StartTaskAgent_*
    (:825-925).

    1. TestBridge_SendSessionMessage_Happy_PostsInputPath — const canned =
       `{"bytes_written":14}`; capture gotPath/gotMethod/gotBody (decode via
       json.NewDecoder(r.Body).Decode(&gotBody)); fake httptest returns 200 +
       canned. req := newCallToolRequest(json.RawMessage(`{"session_id":"s1",
       "message":"run tests"}`)). res, err := b.sendSessionMessage(ctx, req).
       Assert err==nil; gotPath=="/api/sessions/s1/input"; gotMethod==POST;
       gotBody["message"]=="run tests"; res.Content[0].(*mcpsdk.TextContent).Text==canned.

    2. TestBridge_SendSessionMessage_UnknownID_Kamacu404 — fake returns 404 +
       `{"error":"session not found"}`. req with session_id "gone". Assert err
       != nil; err.Error() contains "HTTP 404" (D-05 wrap). Optionally assert
       "session not found" survives the wrap (Gap 2 pattern).

    3. TestBridge_SendSessionMessage_EmptyMessage_StillPosted — fake returns
       200 + `{"bytes_written":1}`. req := newCallToolRequest(`{"session_id":
       "s1","message":""}`). Assert no client-side rejection (err==nil);
       gotBody["message"]=="" (the empty message reached Kamacu verbatim —
       bridge does zero validation).

    Run: `go test ./internal/mcp/ -run TestBridge_SendSessionMessage_ -v -count=1`.
  </action>

  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/mcp/ -run TestBridge_SendSessionMessage_ -v -count=1</automated>
  </verify>

  <done>
    Three TestBridge_SendSessionMessage_* tests pass: happy POST path+body+
    passthrough, unknown-id 404 D-05 wrap, and empty-message-still-posted
    (proving the bridge does no client-side validation). Tests reuse the
    sessions_test.go scaffold + newCallToolRequest; no new helpers.
  </done>
</task>

## Out of Scope

- **Browser/WS changes** — web/src/ and internal/ws/ are untouched. The browser
  remains the authoritative interactive surface.
- **Technical rate-limiting / send quotas / inter-agent permission prompts** —
  the prompt-injection escalation risk is acknowledged in the Security Note, not
  mitigated here. The user's authority model (trust the agent you spawned) is
  the mitigation, identical to start_task_agent/delete_task/delete_project.
- **Bulk send / broadcast / iteration helpers** — one session, one message per
  call. The agent loops.
- **Content sanitization / quoting / escaping** — verbatim passthrough. The
  agent owns content responsibility.
- **PROJECT.md / STATE.md doc edits** — the D-14 Out-of-Scope line evolution is
  a separate concern (orchestrator may handle post-task).
- **routes.go** — session routes live in SessionRoutes() (internal/api/sessions.go),
  not routes.go. routes.go is NOT modified.
- **D-14 SC3 contract** (subscribe_session_output cancel-via-ctx) — unchanged.

## Open Questions

None — scope locked. The trust boundary (KAMACU_HOOK_TOKEN + loopback), the
response shape (200 {"bytes_written":N}), the newline-normalization rule, and
the D-14 import-gate preservation are all decided above.

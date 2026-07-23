---
phase: 08-sessions-terminal-read-access
plan: 02
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/mcp/sessions.go
  - internal/mcp/server.go
  - internal/mcp/sessions_test.go
autonomous: true
requirements:
  - MCPSESS-01
  - MCPSESS-02
  - MCPSESS-03
must_haves:
  truths:
    - "An MCP client can call list_sessions(project_id?, task_id?) and receive current session state (status + taskTitle/projectName/agentName) with orphaned tmux-survivor rows filtered out (D-09, D-13)"
    - "An MCP client can call get_session(session_id) and receive session.Info + the D-10 JOIN; an unknown id surfaces the Phase 07 D-05 error wrap (HTTP 404)"
    - "An MCP client can call get_session_output(session_id, bytes?) and receive the D-08 snapshot envelope (encoding=base64) as TextContent"
    - "registerSessionTools(s, b) is called once from server.go registerTools alongside the Phase 07 registrars (D-07)"
    - "A panic in any session tool handler is caught by defer recover() and surfaces as a clean error, never a stream desync (Phase 06 Open Q1 resolved, Pitfall 2)"
  artifacts:
    - internal/mcp/sessions.go (NEW): registerSessionTools + withRecover + listSessions/getSession/getSessionOutput bridge methods
    - internal/mcp/server.go: registerTools gains the registerSessionTools(s, b) call
    - internal/mcp/sessions_test.go (NEW): per-tool happy + 404 + orphan-filter tests
  key_links:
    - "list_sessions/get_session/get_session_output delegate the response-handling half to bridge.call (Phase 07 pattern) — Kamacu's JSON passes through as TextContent"
    - "withRecover wraps every session tool handler closure (Phase 06 Open Q1 / Pitfall 2)"
  prohibitions:
    - "internal/mcp/sessions.go must not import internal/session (Phase 06 split — the bridge speaks HTTP only; type-level read-only isolation, D-14)"
    - "No handler calls a PTY-write primitive or sends a WS FrameData input frame (the bridge has no access to either — it only does HTTP GETs)"
---

<objective>
Create internal/mcp/sessions.go with the three non-streaming MCP session tools (list_sessions, get_session, get_session_output) repeating the Phase 06/07 bridge pattern verbatim, wire registerSessionTools into server.go, and add the deferred panic-recovery wrapper (Phase 06 Open Question 1 — subscribe introduces the streaming panic surface in Plan 03, so the wrapper is established here and applied to all session handlers). Plan 03 will add subscribe_session_output to this same file.

Purpose: The mechanical bulk of Phase 08 — three thin HTTP-bridge tools. MCPSESS-01/02/03 delivered end-to-end at the MCP layer.
Output: new internal/mcp/sessions.go + internal/mcp/sessions_test.go; one-line addition to internal/mcp/server.go.
</objective>

<execution_context>
@/home/jordi/.config/opencode/gsd-core/workflows/execute-plan.md
@/home/jordi/.config/opencode/gsd-core/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/08-sessions-terminal-read-access/08-CONTEXT.md
@.planning/phases/08-sessions-terminal-read-access/08-RESEARCH.md
@.planning/phases/08-sessions-terminal-read-access/08-01-kamacu-session-endpoints-PLAN.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: sessions.go — registerSessionTools + 3 non-streaming bridge methods + withRecover panic wrapper (D-05/D-07/D-08/D-09/D-10/D-13, MCPSESS-01/02/03)</name>
  <files>internal/mcp/sessions.go, internal/mcp/server.go</files>
  <read_first>
    - internal/mcp/tasks.go (the per-resource template: registerTaskTools + bridge listTasks/getTask methods + map[string]any InputSchema shape — copy this structure exactly for sessions.go)
    - internal/mcp/bridge.go lines 31-106 (bridge struct, do, bridge.call — the response-handling helper the 3 methods delegate to; maxBodyBytes=1<<20; http.Client 10s timeout)
    - internal/mcp/server.go lines 82-86 (registerTools — where registerSessionTools(s, b) is added as one line)
    - internal/mcp/tasks.go lines 182-198 (listTasks — the *int64 optional-arg + path-build pattern to mirror for listSessions project_id/task_id)
    - internal/mcp/server_test.go lines 23-65 (TestSC2 — confirms the SDK does NOT recover handler panics; this is WHY withRecover exists)
  </read_first>
  <behavior>
    - list_sessions: optional project_id OR task_id -> GET /api/sessions?project_id=N or ?task_id=N or bare; Kamacu JSON array passed through as TextContent AFTER orphaned (orphaned:true) rows are filtered out
    - get_session: required session_id (string) -> GET /api/sessions/{id}; Kamacu JSON object passed through; 404 wraps as "HTTP 404" error
    - get_session_output: required session_id + optional bytes -> GET /api/sessions/{id}/output?bytes=N; D-08 envelope passed through verbatim
    - A panic in any of the three handlers is caught and converted to a returned error (never propagates to the SDK dispatch at server.go:753 which has no recover)
  </behavior>
  <action>
    Create internal/mcp/sessions.go (package mcp). Mirror tasks.go's structure: a `registerSessionTools(s *mcp.Server, b *bridge)` function holding the s.AddTool calls, plus `(b *bridge)` method bodies.

    Add a panic-recovery wrapper `withRecover(name string, h func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)) mcp.ToolHandler` that returns a closure doing `defer func() { if r := recover(); r != nil { err = fmt.Errorf("kamacu %s: panic: %v", name, r); result = nil } }()` then `return h(ctx, req)`. This resolves Phase 06 Open Question 1 (the SDK calls st.handler directly with no recover — server.go:753; verified by TestSC2). Apply withRecover to EVERY session tool handler closure passed to s.AddTool (subscribe in Plan 03 gets it too). Use named return values (result, err) so the deferred recover can overwrite them.

    list_sessions (MCPSESS-01): InputSchema map[string]any type=object with properties project_id (integer, optional) and task_id (integer, optional); no required array. Description leads with "List live Kamacu terminal sessions (bash tabs AND agent sessions)... Read-only." Handler `(b *bridge) listSessions(ctx, req)`: unmarshal args struct{ ProjectID *int64; TaskID *int64 } (lenient on empty args, like listTasks at tasks.go:189). Build the query: if ProjectID != nil -> "?project_id=N"; else if TaskID != nil -> "?task_id=N"; else "". Call `b.call(ctx, http.MethodGet, "/api/sessions"+query, nil)` to get the raw TextContent. Then D-13 orphan filter: the Kamacu list still includes orphaned tmux-survivor rows (orphaned:true) for the SPA; the bridge must NOT expose them (every listed id must be operable). Parse the TextContent text as []map[string]any, drop any entry whose "orphaned" field is truthy, re-marshal to JSON, and return a fresh CallToolResult with a single TextContent holding the filtered JSON. If parsing/filtering fails for any reason, fall back to returning the original Kamacu body unchanged (never error on a shape surprise — degrade to passthrough). This keeps Kamacu's SPA behavior intact while guaranteeing the MCP contract.

    get_session (MCPSESS-02): InputSchema type=object, properties session_id (string), required [session_id]. Handler `(b *bridge) getSession(ctx, req)`: unmarshal struct{ SessionID string }; `url.PathEscape(args.SessionID)`; `return b.call(ctx, http.MethodGet, "/api/sessions/"+escaped, nil)`. A Kamacu 404 surfaces through bridge.call's non-2xx wrap as `fmt.Errorf("kamacu %s %s: HTTP %d: %s", ...)` (Phase 07 D-05) — no extra handling needed.

    get_session_output (MCPSESS-03): InputSchema type=object, properties session_id (string, required) + bytes (integer, optional, "last N bytes of the PTY ring, default 4096, clamped to ~512KiB"), required [session_id]. Handler `(b *bridge) getSessionOutput(ctx, req)`: unmarshal struct{ SessionID string; Bytes *int }; build path "/api/sessions/"+escaped+"/output"; if Bytes != nil append "?bytes="+strconv.Itoa(*Bytes); `return b.call(...)`. The Kamacu handler (Plan 01) does the clamping and emits the D-08 envelope; the bridge passes it through as TextContent unchanged (consistent with Phase 07's raw-JSON passthrough).

    In internal/mcp/server.go registerTools (lines 82-86), add `registerSessionTools(s, b)` as a fourth line after registerWorkspaceTools(s, b). Do not modify anything else in server.go.

    Read-only (D-14): internal/mcp/sessions.go must NOT import the in-repo session engine package that owns PTY lifetimes (Phase 06 split — the bridge speaks HTTP only; the executor knows the package from the read_first references). It imports only context, encoding/json, fmt, net/http, net/url, strconv, and github.com/modelcontextprotocol/go-sdk/mcp. It performs only HTTP GETs. There is no PTY-write primitive and no WS frame type in scope anywhere in this file.
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./internal/mcp/ && go test ./internal/mcp/ -run 'TestBridge_ListSessions|TestBridge_GetSession|TestBridge_GetSessionOutput' -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `go build ./internal/mcp/` succeeds.
    - registerSessionTools is invoked exactly once from registerTools (server.go); `grep -c 'registerSessionTools' internal/mcp/server.go` == 1.
    - list_sessions with project_id=5 hits GET /api/sessions?project_id=5 ; with task_id=9 hits GET /api/sessions?task_id=9 ; with neither hits GET /api/sessions (no query).
    - list_sessions filters out orphaned:true rows: a Kamacu response `[{"id":"a","orphaned":true},{"id":"b"}]` yields a TextContent whose decoded JSON array has exactly one element with id=="b".
    - get_session with session_id "xyz" hits GET /api/sessions/xyz ; a Kamacu 404 returns an error whose message contains "HTTP 404".
    - get_session_output with bytes=8192 hits GET /api/sessions/{id}/output?bytes=8192.
    - withRecover converts a panicking handler into a returned non-nil error (testable: a handler that panic()s returns err containing "panic").
    - Read-only isolation: `grep -c 'internal/session' internal/mcp/sessions.go` == 0 (no import of the session package).
    - `go vet ./internal/mcp/` clean.
  </acceptance_criteria>
  <done>
    list_sessions/get_session/get_session_output are registered, bridge to the Plan 01 Kamacu endpoints via bridge.call, filter orphaned rows (D-13), and pass Kamacu JSON through as TextContent (D-08). withRecover protects all three. registerSessionTools wired in server.go. MCPSESS-01/02/03 delivered at the MCP layer.
  </done>
</task>

<task type="auto">
  <name>Task 2: per-tool happy + error tests for the 3 non-streaming tools (Phase 07 D-08 one-representative-test-per-tool)</name>
  <files>internal/mcp/sessions_test.go</files>
  <read_first>
    - internal/mcp/tasks_test.go (the exact test shape to mirror: httptest.NewServer + &bridge{base:srv.URL,...} + newCallToolRequest(json.RawMessage) + path/method/passthrough/"HTTP NNN" assertions)
    - internal/mcp/bridge_test.go lines 15-21 (newCallToolRequest helper — already defined, reuse it)
    - internal/mcp/sessions.go (the file just created in Task 1 — the methods under test)
  </read_first>
  <behavior>
    - One happy + one error case per tool, matching tasks_test.go's density (Phase 07 D-08)
    - The orphan-filter test proves D-13 (the only behavior unique to list_sessions vs the Phase 07 template)
    - The panic-recovery test proves withRecover converts a panic to a clean error (SC2 stream-desync invariant holds)
  </behavior>
  <action>
    Create internal/mcp/sessions_test.go (package mcp). Reuse the existing `newCallToolRequest` helper from bridge_test.go (do not redefine it). Mirror tasks_test.go's httptest.Server + &bridge{base: srv.URL, token:"t", client: &http.Client{Timeout: 10*time.Second}} construction.

    Add these tests:
    (a) TestBridge_ListSessions_NoArgs_HitsBareEndpoint — no args -> asserts r.URL.Path=="/api/sessions", method GET, no query.
    (b) TestBridge_ListSessions_ProjectID_HitsFilteredEndpoint — {"project_id":5} -> path "/api/sessions", query "project_id=5".
    (c) TestBridge_ListSessions_FiltersOrphanedRows (D-13) — Kamacu returns `[{"id":"a","orphaned":true,"tmuxName":"kamacu-1-1"},{"id":"b","status":"running"}]`; assert the returned TextContent decodes to a one-element array whose sole element has id=="b" and no orphaned field.
    (d) TestBridge_GetSession_Happy_PassesThroughJSON — Kamacu returns `{"id":"xyz","status":"running","taskTitle":"t","projectName":"p","agentName":"a"}`; assert path "/api/sessions/xyz", method GET, and verbatim TextContent passthrough.
    (e) TestBridge_GetSession_NotFound404_ReturnsError — Kamacu 404 `{"error":"session not found"}`; assert error contains "HTTP 404".
    (f) TestBridge_GetSessionOutput_WithBytes_HitsOutputEndpoint — {"session_id":"xyz","bytes":8192} -> path "/api/sessions/xyz/output", query "bytes=8192"; Kamacu returns the D-08 envelope `{"encoding":"base64","output":"AA==","bytes":1,"clamped":false}` passed through verbatim.
    (g) TestWithRecover_ConvertsPanicToError — call withRecover("test", func(...) {...; panic("boom")})(ctx, req); assert err != nil and err.Error() contains "panic" and "boom"; assert result is nil. This is the unit proof for Phase 06 Open Q1 / Pitfall 2.
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/mcp/ -run 'TestBridge_(ListSessions|GetSession|GetSessionOutput)|TestWithRecover' -count=1</automated>
  </verify>
  <acceptance_criteria>
    - All seven tests pass.
    - The orphan-filter test asserts exactly one element survives with id=="b" (D-13 proven).
    - The get_session 404 test asserts the error message contains "HTTP 404" (Phase 07 D-05 wrap proven).
    - The withRecover test asserts a panicking handler yields err containing "panic" (SC2 stream-desync protection proven).
    - `go vet ./internal/mcp/` clean.
  </acceptance_criteria>
  <done>
    list_sessions/get_session/get_session_output each have a happy + error representative test; orphan filtering (D-13) and panic recovery (Pitfall 2) are regression-gated.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Agent CLI -> MCP subcommand (stdio) | Untrusted tool-call arguments arrive here; the bridge must PathEscape/validate before interpolating into URLs. |
| MCP bridge -> Kamacu HTTP (loopback) | Same boundary as Plan 01; X-Kamacu-Token sent, loopback-binding is the real auth. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-08-08 | Tampering / EoP | session tool handlers (PTY-write reach) | critical | mitigate | Type-level isolation (D-14): internal/mcp/sessions.go does not import internal/session and performs only HTTP GETs — no PTY-write primitive and no WS FrameData type are in scope. Scoped grep gate (`internal/session` absent) in acceptance criteria. |
| T-08-09 | Tampering | JSON-RPC stream (handler panic) | high | mitigate | withRecover wraps every session handler; a panic becomes a returned error, never an unwound stack through the SDK's unrecovering dispatch (server.go:753). TestWithRecover is the gate. |
| T-08-10 | Spoofing | session_id path interpolation | low | mitigate | url.PathEscape(args.SessionID) before path build (V5 input validation); a malformed id cannot inject path segments. |

Package legitimacy: zero new packages (Plan 02 adds no imports beyond stdlib + the already-installed go-sdk).
</threat_model>

<verification>
- `go build ./internal/mcp/` compiles.
- `go test ./internal/mcp/ -count=1` passes (existing Phase 06/07 tests unchanged + new sessions tests green).
- `go vet ./internal/mcp/` clean.
- Scoped isolation grep: `grep -c 'internal/session' internal/mcp/sessions.go` == 0.
</verification>

<success_criteria>
- list_sessions/get_session/get_session_output callable over MCP, returning current session state + snapshot (SC1/SC2/SC3-snapshot at the MCP layer; MCPSESS-01/02/03).
- Orphaned tmux rows never reach the agent (D-13).
- Panic in a session handler cannot desync the JSON-RPC stream (Phase 06 Open Q1 closed).
- Read-only at the type level: the bridge file has no access to any PTY-write primitive (SC4).
</success_criteria>

<output>
Create `.planning/phases/08-sessions-terminal-read-access/08-02-SUMMARY.md` when done.
</output>

## Artifacts this phase produces (Plan 02)

**internal/mcp/sessions.go (NEW symbols):**
- Function `registerSessionTools(s *mcp.Server, b *bridge)` — registers list_sessions, get_session, get_session_output (subscribe_session_output is ADDED here by Plan 03).
- Function `withRecover(name string, h mcp.ToolHandler-style) mcp.ToolHandler` — panic->error wrapper (Phase 06 Open Q1).
- Bridge methods `(b *bridge) listSessions`, `(b *bridge) getSession`, `(b *bridge) getSessionOutput`.
- Tool registrations: list_sessions, get_session, get_session_output (3 of the 4 MCPSESS tools).

**internal/mcp/server.go (modified):** registerTools gains `registerSessionTools(s, b)` (one line).

**internal/mcp/sessions_test.go (NEW):** TestBridge_ListSessions_*, TestBridge_GetSession_*, TestBridge_GetSessionOutput_*, TestWithRecover_*.

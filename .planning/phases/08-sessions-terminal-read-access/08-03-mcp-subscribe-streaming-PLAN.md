---
phase: 08-sessions-terminal-read-access
plan: 03
type: execute
wave: 2
depends_on:
  - 08-02
files_modified:
  - internal/mcp/sessions.go
  - internal/mcp/sessions_test.go
autonomous: true
requirements:
  - MCPSESS-04
must_haves:
  truths:
    - "An MCP client can call subscribe_session_output(session_id, duration_seconds?, include_history?) and receive ONE CallToolResult whose TextContent is a D-08 JSON envelope {encoding:base64, output, bytes, truncated, exited, exitCode?, stopRequested?}"
    - "subscribe uses a dedicated *http.Client with NO 10s timeout — it does NOT reuse b.client (bridge.go:53) or bridge.call (bridge.go:88) (D-11, Pitfall 1)"
    - "On ctx cancellation (the agent CLI sent notifications/cancelled) the handler returns the partial stream collected so far as a normal result — NOT a transport error (D-01, SC3)"
    - "On ctx cancellation BEFORE subscribeClient.Do succeeds (cancel-before-attach), the handler returns an empty D-08 envelope (output=='', bytes==0) as a normal result — NOT a transport error (D-01 strict-edge: 'returns ONE CallToolResult' holds even when zero bytes were streamed)"
    - "The accumulated buffer is capped at ~1 MiB keeping the MOST RECENT bytes with a truncated:true flag when the cap is hit (D-04)"
    - "Goroutine count is stable across N subscribe/cancel cycles — no leak (SC3)"
  artifacts:
    - internal/mcp/sessions.go: subscribe_session_output tool registration (added to registerSessionTools) + (b *bridge) subscribeSessionOutput method + dedicated streaming *http.Client + subscribeTailCap constant + emptySubscribeEnvelope() helper (D-01 cancel-before-attach guard)
    - internal/mcp/sessions_test.go: SC3 cancellation/leak test (mcp.NewInMemoryTransports + fake streaming httptest.Server) + goroutine-stability-across-N-cycles + subscribe happy/error cases + TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError (D-01 strict-edge)
  key_links:
    - "subscribeSessionOutput accumulates raw octets from GET /api/sessions/{id}/subscribe (Plan 01), then does a follow-up GET /api/sessions/{id} (Plan 01) via b.do for the exit marker, then base64-encodes and assembles the D-08 envelope"
    - "select on ctx.Done() (SDK cancellation -> handler ctx) is the SC3 cancellation primitive (D-01/D-03)"
  prohibitions:
    - "subscribeSessionOutput never calls a PTY-write primitive and never sends a WS FrameData input frame (D-14) — it only issues HTTP GETs"
    - "subscribeSessionOutput never spawns a goroutine that outlives the tool call (no new long-lived/background goroutine — milestone rule)"
---

<objective>
Add the fourth and riskiest MCP session tool — subscribe_session_output — which diverges from the Phase 06/07 bridge pattern: it cannot use bridge.call (10s client timeout, body-buffered) and instead runs a dedicated streaming HTTP read with no short timeout, accumulates raw PTY octets under a 1 MiB most-recent cap, selects on the SDK's ctx for cancellation (SC3), and assembles its own D-08 envelope with an exit marker. This plan also delivers the SC3 cancellation/leak proof — the milestone's central regression gate — using the SDK's in-memory transport plus a fake streaming server.

**Research divergence (08-RESEARCH Open Question 2):** This plan deliberately adopts option (b) — a follow-up `GET /api/sessions/{id}` for the exit marker — over the research's recommended option (a) (a trailing JSON line on the streaming response). Rationale: option (b) avoids introducing sentinel-byte or length-prefix framing into the raw `application/octet-stream` (which would complicate the D-08 envelope contract and risk mis-splitting PTY bytes that happen to match a sentinel), and it reuses the existing `GET /api/sessions/{id}` endpoint delivered by Plan 08-01 rather than adding new Kamacu-side trailer logic. The cost is one extra loopback GET per subscribe call — acceptable for v1.11 single-user localhost. Both options are within D-03 scope; the divergence is documented here for transparency.

Purpose: Deliver MCPSESS-04 (bounded live tail, cancel-safe, read-only) and prove SC3 (partial result on cancel + stable goroutine count). This is the streaming panic surface that motivated deferring the withRecover wrapper (Plan 02) to Phase 08.
Output: subscribe_session_output added to internal/mcp/sessions.go; SC3 + goroutine-stability tests in internal/mcp/sessions_test.go.
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
@.planning/phases/08-sessions-terminal-read-access/08-02-mcp-session-tools-PLAN.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: subscribeSessionOutput bridge method + subscribe_session_output registration (D-01..D-04, D-08, D-11, D-14, MCPSESS-04)</name>
  <files>internal/mcp/sessions.go</files>
  <read_first>
    - internal/mcp/sessions.go (after Plan 02 — registerSessionTools + withRecover + the 3 non-streaming methods already exist; this task ADDS the 4th)
    - internal/mcp/bridge.go lines 31-67 (bridge struct, b.do — use b.do for the follow-up GET, NOT b.call; b.base/b.token fields; the 10s client at line 53 that subscribe MUST NOT reuse)
    - internal/mcp/server_test.go lines 23-65 (TestSC2 — the in-memory transport + Connect shape the SC3 test reuses)
    - internal/session/session.go lines 342-397 (Attach/Detach/Done/Snapshot — the Kamacu-side primitives the Plan 01 streaming endpoint consumes; for understanding the wire contract, not for import)
    - 08-RESEARCH.md "Pattern 3" (the subscribe handler skeleton) and "Open Question 2" (exit-info via follow-up GET, option b)
  </read_first>
  <behavior>
    - subscribe_session_output(session_id, duration_seconds?=30, include_history?=false) returns ONE CallToolResult with a D-08 subscribe envelope
    - duration_seconds clamped to [1, 300] (MCPSESS-04); include_history default false
    - A dedicated *http.Client (no Timeout, or Timeout > 300s+grace) issues GET /api/sessions/{id}/subscribe?duration_seconds=N&include_history=0|1 with X-Kamacu-Token; raw octets are accumulated with a 1 MiB most-recent cap
    - On ctx.Done (cancel), stream EOF, or accumulation complete, the handler does a follow-up GET /api/sessions/{id} for the exit marker, base64-encodes the accumulated buffer, assembles the envelope, returns (result, nil) — never a transport error on cancel
    - Cancel-before-attach (strict-edge, D-01): if ctx is cancelled before subscribeClient.Do succeeds, the handler returns an empty envelope as a normal result (emptySubscribeEnvelope, nil) — NOT a transport error. D-01's "returns ONE CallToolResult" holds even when zero bytes were streamed
    - A panic is caught by withRecover (already applied in Plan 02's registration pattern — apply it to this handler too)
  </behavior>
  <action>
    Add constant `subscribeTailCap = 1 << 20` (1 MiB, D-04) and `defaultSubscribeSeconds = 30`, `maxSubscribeSeconds = 300`. Add a package-level or bridge-field dedicated streaming client: `var subscribeClient = &http.Client{}` (NO Timeout field — the SDK handler ctx is the cancellation mechanism; a 10s/any-short timeout would silently kill every >10s tail per Pitfall 1). Do NOT reuse b.client.

    Add `(b *bridge) subscribeSessionOutput(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error)`. Wrap the whole body via withRecover when registering (see registration step). Unmarshal args struct{ SessionID string; DurationSeconds *int; IncludeHistory *bool } (lenient on empty args). Validate SessionID non-empty (else return nil, fmt.Errorf("subscribe_session_output: session_id is required")). Clamp duration: effective := defaultSubscribeSeconds; if DurationSeconds != nil { effective = *DurationSeconds }; if effective < 1 { effective = 1 }; if effective > maxSubscribeSeconds { effective = maxSubscribeSeconds }. includeHistory := IncludeHistory != nil && *IncludeHistory.

    Build the streaming request: `httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, b.base+"/api/sessions/"+url.PathEscape(args.SessionID)+fmt.Sprintf("/subscribe?duration_seconds=%d&include_history=%v", effective, includeHistory), nil)`; `httpReq.Header.Set("X-Kamacu-Token", b.token)`. `resp, err := subscribeClient.Do(httpReq)`. **Cancel-before-attach guard (D-01 — strict-edge):** if `err != nil`, BEFORE the generic error wrap, check `if errors.Is(err, context.Canceled) { return emptySubscribeEnvelope(), nil }` — this covers the case where the agent CLI's ctx was cancelled before/during `subscribeClient.Do` succeeded (the stream never attached; there is no partial buffer to return, but D-01's "returns ONE CallToolResult" still holds — an empty envelope is a valid result, not a transport error). Only if the error is NOT a context.Cancellation do you fall through to `return nil, fmt.Errorf("kamacu subscribe: %w", err)`. Add a package-level helper `emptySubscribeEnvelope() *mcp.CallToolResult` that returns a CallToolResult whose TextContent is the JSON of `{encoding:"base64", output:"", bytes:0, truncated:false, exited:false}` (the same D-08 struct with a zero-length buffer). Import `errors`. After the guard: `defer resp.Body.Close()`. If resp.StatusCode != 200 { read a bounded body; return nil, fmt.Errorf("kamacu GET /api/sessions/%s/subscribe: HTTP %d: %s", args.SessionID, resp.StatusCode, body) } — the Phase 07 D-05 wrap shape.

    Accumulate under the cap (D-04, Open Q3 recommendation — fixed-size chunks, trim-front on overflow): `buf := make([]byte, 0, subscribeTailCap)`; `truncated := false`; `chunk := make([]byte, 8192)`; loop { `n, readErr := resp.Body.Read(chunk)`; if n > 0 { buf = append(buf, chunk[:n]...); if len(buf) > subscribeTailCap { buf = buf[len(buf)-subscribeTailCap:]; truncated = true } }; if readErr != nil { break } }. resp.Body.Read returns io.EOF when the Kamacu stream closes (duration/exit/disconnect) and returns a context-cancellation error when ctx is cancelled — both break the loop and fall through to envelope assembly. There is NO separate select needed: Read on a request whose ctx was cancelled returns promptly with an error, so the loop exits on cancel just as on EOF (this is why NewRequestWithContext carries the handler ctx).

    Exit marker via follow-up GET (08-RESEARCH Open Q2 option b — unambiguous, avoids sentinel-framing): `exited := false; var exitCode *int; stopRequested := false`. Use `b.do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(args.SessionID), nil)` (b.do uses the 10s client — fine for a quick GET; non-2xx is tolerated). If it returns a 200, read a bounded body and json.Unmarshal into struct{ Status string `json:"status"`; ExitCode *int `json:"exitCode"`; StopRequested bool `json:"stopRequested"` }; set exited = (Status == "exited"), exitCode, stopRequested from it. If the follow-up fails (404 — session reaped mid-subscribe, or transport error), leave exited=false (the session's final state is unknown; the envelope still carries the accumulated bytes + truncated flag). Never let a follow-up failure turn the subscribe into an error — the partial stream is the load-bearing result (SC3: "returns the partial stream collected so far").

    Assemble the D-08 subscribe envelope: struct{ Encoding string `json:"encoding"`; Output string `json:"output"`; Bytes int `json:"bytes"`; Truncated bool `json:"truncated"`; Exited bool `json:"exited"`; ExitCode *int `json:"exitCode,omitempty"`; StopRequested bool `json:"stopRequested,omitempty"` }{ Encoding:"base64", Output:base64.StdEncoding.EncodeToString(buf), Bytes:len(buf), Truncated:truncated, Exited:exited, ExitCode:exitCode, StopRequested:stopRequested }. Marshal to JSON; return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: jsonString}}}, nil. Import encoding/base64.

    Register subscribe_session_output inside registerSessionTools (created by Plan 02): add an s.AddTool call with Name "subscribe_session_output", a Description leading with "Tail live PTY output for a session for a bounded duration (default 30s, cap 300s). Read-only — collect-and-return; cancels cleanly." InputSchema map[string]any type=object, properties session_id (string), duration_seconds (integer, optional), include_history (boolean, optional), required [session_id]. Pass the handler through withRecover("subscribe_session_output", b.subscribeSessionOutput) exactly as the other three session tools are wrapped.

    Read-only (D-14): this method issues only HTTP GETs (the streaming subscribe + the follow-up info). It does not import the in-repo session engine package that owns PTY lifetimes, does not reference any PTY-write primitive, and does not touch the WS frame protocol. It spawns no goroutine — the handler body is the reader (request-scoped; dies with the tool call).
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go build ./internal/mcp/ && go test ./internal/mcp/ -run 'TestSubscribe|TestSubscribeCancelled' -count=1 -timeout 60s</automated>
  </verify>
  <acceptance_criteria>
    - `go build ./internal/mcp/` succeeds.
    - subscribe_session_output is registered (4th tool in registerSessionTools); the handler is wrapped by withRecover.
    - subscribeSessionOutput uses subscribeClient (a dedicated *http.Client) — `grep -c 'b.client' internal/mcp/sessions.go` reflects subscribe does NOT use b.client for the streaming request (the streaming Do uses subscribeClient). It does NOT call bridge.call for the streaming read.
    - duration_seconds=999 is clamped: the assembled request query contains duration_seconds=300.
    - The returned TextContent decodes to JSON with keys encoding=="base64", bytes (int), truncated (bool), exited (bool). When the Kamacu stream returns raw bytes "abc", output decodes (base64) to "abc".
    - On ctx cancellation the handler returns a non-nil *CallToolResult and a nil error (SC3 — partial result, not a transport error).
    - Cancel-before-attach (strict-edge, D-01): when the handler ctx is cancelled before subscribeClient.Do returns a response, `errors.Is(err, context.Canceled)` is true and the handler returns (emptySubscribeEnvelope, nil) — a valid empty D-08 envelope (output=="", bytes==0), NOT an error. D-01's "returns ONE CallToolResult" holds for this edge.
    - When the follow-up GET reports status "exited" + exitCode 0, the envelope has exited==true and exitCode==0.
    - Read-only: `grep -c 'internal/session' internal/mcp/sessions.go` == 0 ; `grep -c 'WriteInput' internal/mcp/sessions.go` == 0 ; `grep -c 'FrameData' internal/mcp/sessions.go` == 0.
    - No `go func` in subscribeSessionOutput (request-scoped; no goroutine launched).
    - `go vet ./internal/mcp/` clean.
  </acceptance_criteria>
  <done>
    subscribe_session_output tails bounded live PTY output (default 30s, cap 300s), accumulates under a 1 MiB most-recent cap (D-04), returns a partial result on ctx cancel (D-01/SC3), carries an exit marker from a follow-up GET (D-03), and assembles the D-08 base64 envelope. Dedicated streaming client; no bridge.call/b.client for the stream (D-11). Read-only enforced (D-14). Delivers MCPSESS-04 at the MCP layer.
  </done>
</task>

<task type="auto">
  <name>Task 2: SC3 cancellation/leak test + goroutine-stability-across-N-cycles + subscribe happy/error (D-01, SC3, Phase 07 D-08)</name>
  <files>internal/mcp/sessions_test.go</files>
  <read_first>
    - internal/mcp/sessions_test.go (after Plan 02 — the 3 non-streaming tool tests + newCallToolRequest reuse already exist)
    - internal/mcp/server_test.go lines 23-65 (TestSC2 — mcp.NewInMemoryTransports + srv.Connect + client.Connect + clientSession.CallTool shape; the SC3 test mirrors this end-to-end-driving structure)
    - internal/mcp/sessions.go (after Task 1 — subscribeSessionOutput + subscribeClient under test)
    - 08-RESEARCH.md "Code Examples / Example 2" (the SC3 test target shape)
  </read_first>
  <behavior>
    - SC3 proof: cancelling the caller's context mid-subscribe returns a partial CallToolResult (NOT an error) and the fake streaming server observes the connection close promptly
    - Goroutine stability: runtime.NumGoroutine() does not grow across N=5 subscribe/cancel cycles (no leak)
    - Happy path: a short duration subscribe returns an envelope with the streamed bytes
    - Error path: a non-200 streaming response surfaces the D-05 wrap (HTTP NNN)
    - Cancel-before-attach (strict-edge, D-01): a ctx already cancelled BEFORE subscribeSessionOutput is called returns (emptyEnvelope, nil) — no transport error, zero bytes
  </behavior>
  <action>
    Add tests to internal/mcp/sessions_test.go (package mcp). Use httptest.NewServer for the fake Kamacu and the existing newCallToolRequest / &bridge{base,token,client} construction for the bridge-method-level tests, and mcp.NewInMemoryTransports for the end-to-end SDK-level cancellation test.

    (a) TestBridge_Subscribe_Happy_ReturnsEnvelope — fake server: on GET /api/sessions/{id}/subscribe write "hello" then close; on GET /api/sessions/{id} (the follow-up) return 200 `{"status":"running"}`. Call b.subscribeSessionOutput(ctx, newCallToolRequest(`{"session_id":"xyz","duration_seconds":1}`)). Assert err==nil; decode the TextContent JSON; assert encoding=="base64", base64-decoded output contains "hello", exited==false, truncated==false.

    (b) TestBridge_Subscribe_DurationClamp_QueryReflects300 — fake server records the subscribe query. Call with duration_seconds=999. Assert the recorded query string is "duration_seconds=300&include_history=false".

    (c) TestBridge_Subscribe_Non200_ReturnsWrappedError — fake server returns 500 `{"error":"boom"}` on subscribe. Assert err != nil and err.Error() contains "HTTP 500".

    (d) TestBridge_Subscribe_TruncationCap (D-04) — fake server streams >subscribeTailCap bytes (e.g. 1.5 MiB of 'x') then closes; follow-up returns running. Assert truncated==true and bytes <= subscribeTailCap (the buffer kept the most-recent 1 MiB).

    (e) TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches (SC3 — the central gate). Use mcp.NewInMemoryTransports(): create srv, register ONLY subscribe_session_output via registerSessionTools(srv, b) where b points at a fake streaming httptest.Server whose handler blocks forever after attaching (write one chunk, then `<-r.Context().Done()` so it holds the conn until the client disconnects) and records the disconnect time; on the follow-up GET return running. Connect a client over the in-memory transport. Launch `clientSession.CallTool(ctx, subscribe_session_output, {session_id, duration_seconds:60})` in a goroutine. After a short sleep (so the stream is attached), cancel the caller context (simulating notifications/cancelled — the SDK cancels the handler ctx per transport.go:205-218). Assert: CallTool returns a non-nil result with NO transport error (res non-nil, the envelope decodes, contains the one chunk that streamed before cancel); and the fake server observed its request context Done() fire within a generous bound (e.g. 2s — proves the bridge closed the HTTP body on ctx cancel, propagating to the Kamacu-side r.Context().Done() which is what triggers Session.Detach in Plan 01). This is the MCP half of SC3; Plan 01's TestSubscribe_ClientCancel_DetachesPromptly is the Kamacu half.

    (f) TestSubscribe_GoroutineStabilityAcrossCycles (SC3 leak gate) — capture runtime.NumGoroutine() before; run N=5 cycles of: create a fresh cancellable ctx, call b.subscribeSessionOutput against a fake blocking streaming server in a goroutine, cancel after a short sleep, wait for the call to return. After all cycles + a short settle (e.g. 200ms), assert runtime.NumGoroutine() is not meaningfully higher than the baseline (allow a small delta like +2 for GC/timer goroutines; the assertion is "no growth proportional to N" — i.e. delta < N). This proves no goroutine/conn leak across subscribe/cancel cycles.

    (g) TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError (strict-edge, D-01) — create a context and cancel it IMMEDIATELY (ctx, cancel := context.WithCancel(background()); cancel()) before calling b.subscribeSessionOutput(ctx, newCallToolRequest(`{"session_id":"xyz","duration_seconds":1}`)). The fake streaming httptest.Server is irrelevant here (the request never reaches it successfully because the ctx is already cancelled). Assert: err == nil (NOT a transport error); res != nil; decode the TextContent JSON; assert encoding=="base64", output=="" (base64 of zero bytes is the empty string), bytes==0, truncated==false, exited==false. This proves the cancel-before-Do guard returns a valid empty D-08 envelope per D-01's "returns ONE CallToolResult" — the strict-edge the checker flagged.
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/mcp/ -run 'TestBridge_Subscribe|TestSubscribe_Cancel' -count=1 -timeout 120s</automated>
  </verify>
  <acceptance_criteria>
    - All seven tests pass (a–g).
    - The SC3 test asserts CallTool returns a non-nil partial result (not a transport error) on context cancellation, AND the fake server's request context Done fired (bridge closed the stream -> Kamacu-side r.Context().Done() -> Detach).
    - The goroutine-stability test asserts runtime.NumGoroutine() after 5 cycles is within a small delta of the baseline (no leak proportional to N).
    - The truncation test asserts truncated==true and bytes <= 1<<20 when the stream exceeded the cap.
    - The duration-clamp test asserts the subscribe query reflects duration_seconds=300 for an input of 999.
    - The cancel-before-attach test (g) asserts err==nil and a non-nil result whose envelope has bytes==0 / output=="" / exited==false when the ctx is cancelled before subscribeSessionOutput runs (D-01 strict-edge).
    - `go vet ./internal/mcp/` clean.
  </acceptance_criteria>
  <done>
    SC3 is regression-gated: subscribe returns a partial result on cancel (not an error) and the stream detaches promptly; goroutine count is stable across N cycles (no leak); cancel-before-attach returns an empty envelope per D-01 (strict-edge covered). Happy/error/truncation/duration-clamp paths all covered. MCPSESS-04 proven end-to-end.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Agent CLI -> MCP subcommand (stdio) | subscribe call + notifications/cancelled arrive here; the handler ctx is the cancellation channel. |
| MCP bridge -> Kamacu streaming HTTP (loopback) | The dedicated subscribeClient holds a long-lived connection for up to 300s; ctx cancel must propagate to a closed connection (SC3). |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-08-11 | Denial of Service | subscribe memory (unbounded accumulation) | medium | mitigate | subscribeTailCap (1MiB) with most-recent retention + truncated flag (D-04); incremental trim-front on each read. TestBridge_Subscribe_TruncationCap is the gate. |
| T-08-12 | Denial of Service | subscribe goroutine/conn leak on cancel | high | mitigate | ctx-carried http request (NewRequestWithContext) so Read unblocks on cancel; defer resp.Body.Close(); handler is request-scoped (no `go func`). TestSubscribe_GoroutineStabilityAcrossCycles is the gate. |
| T-08-13 | Tampering | subscribe handler panic desyncs JSON-RPC stream | high | mitigate | withRecover wraps subscribeSessionOutput (Pitfall 2 / Phase 06 Open Q1); panic -> error, never an unwound stack through the SDK dispatch. |
| T-08-14 | Tampering / EoP | subscribe reaching a PTY-write path | critical | mitigate | Type-level read-only (D-14): subscribeSessionOutput issues only HTTP GETs; no internal/session import, no PTY-write primitive, no WS FrameData. Scoped grep gates in acceptance criteria. |
| T-08-15 | Denial of Service | subscribe duration_seconds huge | medium | mitigate | Server-side clamp to maxSubscribeSeconds (300) before building the request (MCPSESS-04); TestBridge_Subscribe_DurationClamp is the gate. |

Package legitimacy: zero new packages (subscribeClient is a bare &http.Client{}; base64/strconv are stdlib).
</threat_model>

<verification>
- `go build ./internal/mcp/` compiles.
- `go test ./internal/mcp/ -count=1 -timeout 120s` passes (Phase 06/07/08-Plan02 tests unchanged + all subscribe/SC3 tests green).
- `go vet ./internal/mcp/` clean.
- Scoped read-only grep: `grep -c 'WriteInput' internal/mcp/sessions.go` == 0 ; `grep -c 'FrameData' internal/mcp/sessions.go` == 0 ; `grep -c 'internal/session' internal/mcp/sessions.go` == 0.
</verification>

<success_criteria>
- subscribe_session_output delivers a bounded live tail (default 30s, cap 300s) returning one D-08 envelope (MCPSESS-04).
- Cancellation returns the partial stream as a normal result and detaches promptly; goroutine count stable across N cycles (SC3).
- No tool in Phase 08 exposes PTY-input — read-only at the type level (SC4): the subscribe path is HTTP GETs only, no PTY-write primitive, no WS FrameData input frame.
- No new long-lived goroutine (milestone rule): subscribe's reader is the request-scoped handler body.
</success_criteria>

<output>
Create `.planning/phases/08-sessions-terminal-read-access/08-03-SUMMARY.md` when done.
</output>

## Artifacts this phase produces (Plan 03)

**internal/mcp/sessions.go (NEW/MODIFIED symbols):**
- Constants `subscribeTailCap = 1<<20`, `defaultSubscribeSeconds = 30`, `maxSubscribeSeconds = 300`.
- Var `subscribeClient = &http.Client{}` (dedicated no-timeout streaming client).
- Package helper `emptySubscribeEnvelope() *mcp.CallToolResult` (D-01 strict-edge: zero-byte D-08 envelope returned when ctx is cancelled before subscribeClient.Do succeeds).
- Bridge method `(b *bridge) subscribeSessionOutput(ctx, req) (result, err)`.
- The subscribe D-08 envelope struct (Encoding/Output/Bytes/Truncated/Exited/ExitCode/StopRequested).
- Tool registration `subscribe_session_output` added to `registerSessionTools` (4th session tool), wrapped by withRecover.

**internal/mcp/sessions_test.go (NEW tests):** TestBridge_Subscribe_Happy_ReturnsEnvelope, TestBridge_Subscribe_DurationClamp_QueryReflects300, TestBridge_Subscribe_Non200_ReturnsWrappedError, TestBridge_Subscribe_TruncationCap, TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches (SC3, uses mcp.NewInMemoryTransports), TestSubscribe_GoroutineStabilityAcrossCycles (SC3 leak gate), TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError (D-01 strict-edge: pre-cancelled ctx returns empty envelope, not an error).

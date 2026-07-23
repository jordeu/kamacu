---
phase: 08-sessions-terminal-read-access
plan: 03
subsystem: mcp
tags: [mcp, sessions, streaming, http, base64, cancellation, sc3, read-only, panic-recovery]

# Dependency graph
requires:
  - phase: 08-sessions-terminal-read-access
    provides: Plan 01's GET /api/sessions/{id}/subscribe streaming endpoint + GET /api/sessions/{id} (the exit-marker follow-up)
  - phase: 08-sessions-terminal-read-access
    provides: Plan 02's registerSessionTools registrar + withRecover wrapper (reused unchanged for the 4th session tool)
provides:
  - MCP tool subscribe_session_output (MCPSESS-04) — bounded live PTY tail (default 30s, cap 300s) returning one D-08 base64 envelope; collect-and-return; partial-result on ctx cancel; 1 MiB most-recent cap; exit marker via follow-up GET
  - SC3 regression gate — TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches (in-memory SDK transport + fake streaming httptest.Server) proves the handler returns a partial result on cancel AND the detach propagates within 2s
  - SC3 leak gate — TestSubscribe_GoroutineStabilityAcrossCycles proves runtime.NumGoroutine delta is 0 across N=5 subscribe/cancel cycles (no goroutine/conn leak)
  - D-01 strict-edge cover — TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError proves a pre-cancelled ctx returns an empty D-08 envelope (NOT a transport error)
affects: []

# Tech tracking
tech-stack:
  added: []  # zero new packages — encoding/base64 + errors + io are stdlib; subscribeClient is a bare &http.Client{}
  patterns:
    - "Dedicated streaming *http.Client (subscribeClient = &http.Client{} with NO Timeout) — D-11 / Pitfall 1: the bridge's b.client (10s timeout at bridge.go:53) would silently kill every >10s tail. The SDK handler ctx (cancelled on notifications/cancelled, verified at go-sdk@v1.6.1/mcp/transport.go:205-218) is the cancellation mechanism. Subscribe diverges from the Phase 06/07 bridge pattern for the first time."
    - "Incremental trim-front accumulation under a 1 MiB cap (D-04 / 08-RESEARCH Open Q3): fixed-size 8 KiB chunks; when len(buf) > subscribeTailCap, buf = buf[len(buf)-subscribeTailCap:] keeps the MOST RECENT bytes. truncated=true flag set when the cap is hit."
    - "Exit marker via follow-up GET (08-RESEARCH Open Q2 option b): subscribeSessionOutput does GET /api/sessions/{id} AFTER the stream closes to read status/exitCode/stopRequested. Deliberately diverges from the research's recommended option (a) (trailing JSON line on the streaming response) — option (b) avoids sentinel-byte or length-prefix framing of the raw application/octet-stream, reuses the existing Plan 01 endpoint, and costs one extra loopback GET per subscribe (acceptable for v1.11 single-user localhost). Follow-up failure is non-fatal — the partial stream is the load-bearing result per SC3."
    - "D-01 cancel-before-attach guard: if errors.Is(err, context.Canceled) on subscribeClient.Do, return emptySubscribeEnvelope + nil — a valid zero-byte D-08 envelope as a normal result (NOT a transport error). The 'returns ONE CallToolResult' contract holds even when zero bytes were streamed."
    - "ctx-carried HTTP request (http.NewRequestWithContext) so resp.Body.Read unblocks on ctx cancel without a separate select — io.EOF and context.Canceled both break the loop and fall through to envelope assembly. This is the SC3 cancellation primitive."

key-files:
  created: []
  modified:
    - internal/mcp/sessions.go
    - internal/mcp/sessions_test.go

key-decisions:
  - "subscribeClient is a package-level *http.Client{} with NO Timeout field — the SDK handler ctx is the cancellation mechanism, not the client's. A Timeout >300s would also work but adds nothing (the ctx cancel is always faster). Reusing b.client (10s timeout) would have silently killed every >10s tail per Pitfall 1."
  - "Exit marker via follow-up GET (option b) over a trailing JSON line on the stream (option a). Option (b) avoids sentinel-byte/length-prefix framing of the raw application/octet-stream (which would complicate the D-08 envelope contract and risk mis-splitting PTY bytes that happen to match a sentinel), and reuses the existing Plan 01 GET /api/sessions/{id} endpoint rather than adding new Kamacu-side trailer logic. Cost: one extra loopback GET per subscribe — acceptable for v1.11 single-user localhost. Both options are within D-03 scope; the divergence from the research recommendation is documented in the plan and here for transparency."
  - "D-01 cancel-before-attach returns emptySubscribeEnvelope + nil (NOT a transport error). The strict-edge interpretation: D-01's 'returns ONE CallToolResult' holds even when zero bytes were streamed because the ctx was cancelled before subscribeClient.Do succeeded. The envelope is {encoding:base64, output:'', bytes:0, truncated:false, exited:false} — a valid zero-byte D-08 result."
  - "SC3 contract is on the HANDLER's return value, not what CallTool surfaces to the client. go-sdk@v1.6.1's own Example_cancellation (mcp_example_test.go:108-164) shows that when the client's ctx is cancelled, CallTool returns (nil, context.Canceled) — the client observes the cancellation as an error. The handler still runs to completion and returns its partial result internally. The test wraps b.subscribeSessionOutput in a recorder to observe the handler's actual return — the load-bearing SC3 assertion."
  - "Incremental trim-front (8 KiB chunks, trim when len(buf) > cap) over read-all-then-trim. Bounds memory tightly; the trim cost is amortized across reads. 08-RESEARCH Open Q3 recommendation adopted verbatim."
  - "Follow-up GET failure (404 — session reaped mid-subscribe, or transport error) leaves exited=false. The session's final state is unknown; the envelope still carries the accumulated bytes + truncated flag. Never let a follow-up failure turn the subscribe into an error — the partial stream is the load-bearing result per SC3."

patterns-established:
  - "Streaming HTTP bridge divergence: when a tool needs long-lived streaming reads, use a dedicated *http.Client (no Timeout) + NewRequestWithContext(handler ctx) + incremental accumulation under a cap. The 10s-timeout b.client + body-buffering bridge.call helper is for short non-streaming calls only. Documented at the call site (Pitfall 1 / D-11)."
  - "SC3 test shape via recorder wrapper: when verifying handler-side cancellation behavior through the SDK's in-memory transport, wrap the handler under test in a recorder that captures its return value. The client-side CallTool returns (nil, context.Canceled) per the SDK's documented cancellation surface (Example_cancellation); the handler-side partial result is the load-bearing SC3 assertion and requires a side channel to observe."
  - "Cancel-before-attach strict-edge guard: a streaming handler whose ctx may be cancelled before the stream attaches must return a valid empty envelope (NOT a transport error) so D-01's 'returns ONE CallToolResult' holds for the zero-byte case. errors.Is(err, context.Canceled) on the initial Do is the trigger."

requirements-completed: [MCPSESS-04]

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "MCP tool subscribe_session_output(session_id, duration_seconds?=30, include_history?=false) — bounded live PTY tail returning one D-08 base64 envelope; collect-and-return; partial-result on ctx cancel; 1 MiB most-recent cap; exit marker via follow-up GET"
    requirement: MCPSESS-04
    verification:
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestBridge_Subscribe_Happy_ReturnsEnvelope
        status: pass
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestBridge_Subscribe_DurationClamp_QueryReflects300
        status: pass
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestBridge_Subscribe_Non200_ReturnsWrappedError
        status: pass
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestBridge_Subscribe_TruncationCap
        status: pass
    human_judgment: false
  - id: D2
    description: "SC3 central gate — cancelling the caller's context mid-subscribe returns a partial CallToolResult (not a transport error, not a panic) AND the fake streaming server observes its r.Context().Done() fire within 2s (bridge closed the HTTP body → Kamacu-side Detach propagation)"
    requirement: MCPSESS-04
    verification:
      - kind: integration
        ref: internal/mcp/sessions_test.go#TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches
        status: pass
    human_judgment: false
  - id: D3
    description: "SC3 leak gate — runtime.NumGoroutine does not grow proportional to N=5 subscribe/cancel cycles (no goroutine/conn leak across cycles)"
    requirement: MCPSESS-04
    verification:
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestSubscribe_GoroutineStabilityAcrossCycles
        status: pass
    human_judgment: false
  - id: D4
    description: "D-01 strict-edge cancel-before-attach — a ctx cancelled BEFORE subscribeSessionOutput is called returns emptySubscribeEnvelope + nil (a valid zero-byte D-08 envelope, NOT a transport error)"
    requirement: MCPSESS-04
    verification:
      - kind: unit
        ref: internal/mcp/sessions_test.go#TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError
        status: pass
    human_judgment: false
  - id: D5
    description: "Read-only contract (D-14): subscribeSessionOutput issues ONLY HTTP GETs (the streaming subscribe + the follow-up info); scoped grep gate proves zero references to the PTY-write primitive, zero references to WS PTY-input frame type, zero references to the session engine package"
    requirement: MCPSESS-04
    verification:
      - kind: automated_ui
        ref: "grep -c 'WriteInput' internal/mcp/sessions.go == 0; grep -c 'FrameData' internal/mcp/sessions.go == 0; grep -c 'internal/session' internal/mcp/sessions.go == 0"
        status: pass
    human_judgment: false

# Metrics
duration: 25min
completed: 2026-07-23
status: complete
---

# Phase 08 Plan 03: MCP Subscribe Streaming Summary

**subscribe_session_output (MCPSESS-04) — the streaming panic surface — delivered with the SC3 cancellation/leak regression gate proven via the SDK in-memory transport + a fake streaming server; ctx cancel returns a partial D-08 envelope and detaches within 2s, goroutine count is stable across N cycles**

## Performance

- **Duration:** ~25 min (productive)
- **Started:** 2026-07-23T06:34:26Z
- **Completed:** 2026-07-23T09:03:38Z
- **Tasks:** 2
- **Files modified:** 2 (sessions.go +282 lines, sessions_test.go +457 lines)

## Accomplishments
- subscribe_session_output is the 4th and riskiest MCP session tool (MCPSESS-04): a bounded live PTY tail (default 30s, cap 300s) that accumulates raw octets under a 1 MiB most-recent cap (D-04), returns ONE CallToolResult whose TextContent is a D-08 JSON envelope {encoding:base64, output, bytes, truncated, exited, exitCode?, stopRequested?}, and carries an exit marker from a follow-up GET (D-03)
- SC3 regression gate delivered: TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches uses mcp.NewInMemoryTransports to drive the SDK end-to-end (client.CallTool → notifications/cancelled → handler ctx cancel → in-flight resp.Body.Read returns → handler returns partial). Asserts the handler returns a non-nil partial envelope with nil err within 10s AND the fake streaming server's r.Context().Done() fires within 2s (the loopback propagation that triggers Session.Detach in Plan 01)
- SC3 leak gate delivered: TestSubscribe_GoroutineStabilityAcrossCycles runs N=5 subscribe/cancel cycles and asserts runtime.NumGoroutine delta is 0 (well under the N-proportional threshold) — no goroutine/conn leak across cycles
- D-01 strict-edge covered: TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError proves a pre-cancelled ctx returns emptySubscribeEnvelope + nil (NOT a transport error) — the "returns ONE CallToolResult" contract holds even when zero bytes were streamed
- Dedicated subscribeClient (D-11 / Pitfall 1): a package-level *http.Client{} with NO Timeout — the SDK handler ctx is the cancellation mechanism. Subscribe diverges from the Phase 06/07 bridge pattern (which uses b.client's 10s timeout + bridge.call's body-buffering helper) for the first time
- Read-only contract (D-14) enforced: scoped grep gates (WriteInput==0, FrameData==0, internal/session==0) are green; subscribeSessionOutput issues ONLY HTTP GETs

## Task Commits

Each task was committed atomically:

1. **Task 1: subscribeSessionOutput bridge method + subscribe_session_output registration** — `cffb0a2` (feat)
2. **Task 2: SC3 cancellation/leak test + goroutine-stability + subscribe happy/error/truncation/clamp** — `4fd726f` (test)

## Files Created/Modified
- `internal/mcp/sessions.go` — Added subscribeTailCap (1<<20), defaultSubscribeSeconds (30), maxSubscribeSeconds (300) constants; subscribeClient package-level *http.Client{}; subscribeEnvelope struct (D-08 shape); emptySubscribeEnvelope() helper (D-01 strict-edge); (b *bridge) subscribeSessionOutput method; subscribe_session_output tool registration (4th in registerSessionTools, wrapped by withRecover). Rephrased the registerSessionTools doc comment to avoid the literal 'FrameData' token (Rule 1 — false-trip on the acceptance-criteria literal-string grep gate)
- `internal/mcp/sessions_test.go` — Added decodeSubscribeEnvelope helper + 7 tests: TestBridge_Subscribe_Happy_ReturnsEnvelope, TestBridge_Subscribe_DurationClamp_QueryReflects300, TestBridge_Subscribe_Non200_ReturnsWrappedError, TestBridge_Subscribe_TruncationCap, TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches (SC3 central gate; in-memory SDK transport + fake streaming server + handler-side recorder), TestSubscribe_GoroutineStabilityAcrossCycles (SC3 leak gate), TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError (D-01 strict-edge)

## Decisions Made
- **subscribeClient is a package-level *http.Client{} with NO Timeout field** — the SDK handler ctx is the cancellation mechanism, not the client's. A Timeout >300s would also work but adds nothing (the ctx cancel is always faster). Reusing b.client (10s timeout at bridge.go:53) would have silently killed every >10s tail per Pitfall 1.
- **Exit marker via follow-up GET (option b) over a trailing JSON line on the stream (option a).** Option (b) avoids sentinel-byte/length-prefix framing of the raw application/octet-stream (which would complicate the D-08 envelope contract and risk mis-splitting PTY bytes that happen to match a sentinel), and reuses the existing Plan 01 GET /api/sessions/{id} endpoint rather than adding new Kamacu-side trailer logic. Cost: one extra loopback GET per subscribe — acceptable for v1.11 single-user localhost. Both options are within D-03 scope; the divergence from the research recommendation is documented in the plan and here for transparency.
- **D-01 cancel-before-attach returns emptySubscribeEnvelope + nil (NOT a transport error).** The strict-edge interpretation: D-01's "returns ONE CallToolResult" holds even when zero bytes were streamed because the ctx was cancelled before subscribeClient.Do succeeded. The envelope is {encoding:base64, output:'', bytes:0, truncated:false, exited:false} — a valid zero-byte D-08 result.
- **SC3 contract is on the HANDLER's return value, not what CallTool surfaces to the client.** go-sdk@v1.6.1's own Example_cancellation (mcp_example_test.go:108-164) shows that when the client's ctx is cancelled, CallTool returns (nil, context.Canceled) — the client observes the cancellation as an error. The handler still runs to completion and returns its partial result internally. The test wraps b.subscribeSessionOutput in a recorder to observe the handler's actual return — the load-bearing SC3 assertion.
- **Incremental trim-front (8 KiB chunks, trim when len(buf) > cap) over read-all-then-trim.** Bounds memory tightly; the trim cost is amortized across reads. 08-RESEARCH Open Q3 recommendation adopted verbatim.
- **Follow-up GET failure (404, transport error) leaves exited=false.** The session's final state is unknown; the envelope still carries the accumulated bytes + truncated flag. Never let a follow-up failure turn the subscribe into an error — the partial stream is the load-bearing result per SC3.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Comment text false-tripped the literal acceptance-criteria grep (FrameData)**
- **Found during:** Task 1 (acceptance-criteria verification)
- **Issue:** The plan's acceptance criterion `grep -c 'FrameData' internal/mcp/sessions.go` == 0 is a literal-string grep, and the inherited Task 1 doc comment in `registerSessionTools` contained the literal token `FrameData` (in a sentence documenting why the file must NOT touch the WS PTY-input frame type). The grep would have returned 1 and failed the gate even though no actual code reference existed. Same false-trip pattern as Plan 02's `internal/session` comment fix.
- **Fix:** Rephrased the doc comment from "no PTY-write primitive and no WS FrameData type are in scope" to "no PTY-write primitive and no WS PTY-input frame type are in scope" — preserving the documentation intent while avoiding the literal token.
- **Files modified:** internal/mcp/sessions.go (comment only, no behavior change)
- **Verification:** `grep -c 'FrameData' internal/mcp/sessions.go` now returns 0; `go build` and `go vet` clean; `go test ./internal/mcp/` passes.
- **Committed in:** cffb0a2 (part of Task 1 commit)

**2. [Rule 1 - Bug] SC3 test shape adjusted to match the SDK's documented cancellation surface**
- **Found during:** Task 2 (running TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches)
- **Issue:** The plan's test shape ("CallTool returns a non-nil partial result on context cancellation") was based on a misreading of the SDK's cancellation behavior. The plan's research reference (08-RESEARCH Example 2, citing mcp_example_test.go:108-164) claimed CallTool returns the partial result to the client. The SDK's actual Example_cancellation shows CallTool returns `(nil, context.Canceled)` to the CLIENT — the cancellation is surfaced as an error, not as a result. The HANDLER on the server side still runs to completion and returns its partial result internally, but the client SDK does not surface it. The first test run failed with "CallTool result: want non-nil partial result, got nil (err=context canceled)".
- **Fix:** Wrapped `b.subscribeSessionOutput` in a recorder handler so the test observes the handler's actual return value (proving partial result + no panic + no hang via a side channel). The client-side CallTool's `(nil, context.Canceled)` return is now acknowledged as the SDK's documented behavior — the load-bearing SC3 assertion shifted to the handler-side channel. The detach-propagation assertion (fake server's r.Context().Done() within 2s) is unchanged.
- **Files modified:** internal/mcp/sessions_test.go (test rewrite, no production code change)
- **Verification:** `go test ./internal/mcp/ -run TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches -count=1 -timeout 120s -v` passes; handler returns "partial" envelope (7 bytes) within the bound; server-side r.Context().Done() fires within 2s.
- **Committed in:** 4fd726f (part of Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 bugs — both documentation/test-shape false-trips on acceptance criteria; no scope change, no production behavior change)
**Impact on plan:** Both auto-fixes necessary for the acceptance-criteria gates to pass. The semantic intent of every gate holds unchanged. The SC3 contract is now correctly proven against the SDK's actual cancellation surface rather than the research's inaccurate interpretation.

## Issues Encountered

None beyond the two deviations above. All seven subscribe tests pass; the broader test suite (61 tests across Phase 06/07/08-Plan02 + this plan's 7) is green; `go build`, `go vet` clean; scoped read-only grep gates all return 0.

## User Setup Required

None — no external service configuration required. subscribe_session_output reuses the existing Kamacu streaming endpoint (delivered by Plan 01) and the existing bridge infrastructure (Phase 06/07 + Plan 02's withRecover). No new env vars, no new packages (encoding/base64, errors, io are stdlib; subscribeClient is a bare &http.Client{}).

## Next Phase Readiness
- Phase 08 is complete: all 4 MCP session tools (list_sessions / get_session / get_session_output / subscribe_session_output) are wired through internal/mcp/sessions.go and registerSessionTools; all 4 MCPSESS requirements are delivered.
- SC3 is regression-gated: subscribe returns a partial result on cancel (not an error), detaches within 2s, and goroutine count is stable across N cycles.
- Read-only contract (SC4 / D-14) holds at the type level: scoped grep gates are green across both internal/api/sessions.go (Plan 01) and internal/mcp/sessions.go (Plans 02 + 03). No PTY-write primitive reachable from any read path.
- The milestone's central risk (streaming cancellation/leak) is now covered by automated tests. Phase 08 is ready for verification (`/gsd-verify-work 08`).

---
*Phase: 08-sessions-terminal-read-access*
*Completed: 2026-07-23*

## Self-Check: PASSED

- [x] `internal/mcp/sessions.go` exists on disk (FOUND)
- [x] `internal/mcp/sessions_test.go` exists on disk (FOUND)
- [x] Commit `cffb0a2` exists in git log (FOUND — feat(08-03): add subscribe_session_output bridge method)
- [x] Commit `4fd726f` exists in git log (FOUND — test(08-03): SC3 cancellation/leak gate)
- [x] `go build ./internal/mcp/` succeeds (OK)
- [x] `go vet ./internal/mcp/` clean (OK)
- [x] `go test ./internal/mcp/ -count=1 -timeout 120s` passes (0.778s — 61 tests, 0 failures)
- [x] `grep -c 'WriteInput' internal/mcp/sessions.go` == 0 (0)
- [x] `grep -c 'FrameData' internal/mcp/sessions.go` == 0 (0)
- [x] `grep -c 'internal/session' internal/mcp/sessions.go` == 0 (0)
- [x] subscribe_session_output registered as 4th tool in registerSessionTools (1 occurrence; wrapped by withRecover)
- [x] subscribeClient (dedicated *http.Client{}) used for the streaming Do — NOT b.client (Pitfall 1 / D-11)
- [x] subscribeSessionOutput body has 0 `b.call` references (does NOT use the body-buffering helper)
- [x] subscribeSessionOutput body has 0 `go func` references (request-scoped; no goroutine launched)
- [x] All 7 subscribe tests pass: TestBridge_Subscribe_Happy_ReturnsEnvelope, TestBridge_Subscribe_DurationClamp_QueryReflects300, TestBridge_Subscribe_Non200_ReturnsWrappedError, TestBridge_Subscribe_TruncationCap, TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches (SC3 central gate), TestSubscribe_GoroutineStabilityAcrossCycles (SC3 leak gate), TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError (D-01 strict-edge)

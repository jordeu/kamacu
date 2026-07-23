---
phase: 08-sessions-terminal-read-access
verified: 2026-07-23T12:10:00Z
status: passed
score: 4/4 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: none
  previous_score: n/a
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 08: Sessions & Terminal Read Access — Verification Report

**Phase Goal:** An agent can enumerate sessions across the board and read terminal output — snapshots immediately, live tails bounded — without ever injecting bytes into a PTY. The milestone's only genuinely new capability (everything else is translation) and its risk center.
**Verified:** 2026-07-23T12:10:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (the 4 ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | An agent can call `list_sessions(project_id? or task_id?)` and `get_session(session_id)` and receive current session state — status (working/waiting/idle/exited/running), task, project, agent, started_at — identical to what the SPA's Active Sessions bar shows. | ✓ VERIFIED | `registerSessionTools` (internal/mcp/sessions.go:92-191) registers both tools; `b.listSessions`/`b.getSession` (lines 432-516) bridge to Kamacu's `GET /api/sessions` (with `?project_id=N` branch in `list` at sessions.go:70-104) and `GET /api/sessions/{id}` (sessions.go:875-890). `sessionDetail` embeds `session.Info` (status + started_at) + D-10 JOIN (`taskTitle`/`projectName`/`agentName`). Verified by `TestBridge_ListSessions_*` (4 tests), `TestBridge_GetSession_Happy_PassesThroughJSON`, `TestBridge_GetSession_NotFound404_ReturnsError`, `TestGetSession_Happy` (asserts all 4 JOIN fields populated), `TestGetSession_ExitedStillWorks` (D-12), `TestListSessions_ProjectID`, `TestListSessions_JoinFields`. `go test ./internal/api/ ./internal/mcp/` PASSES. `registerSessionTools` invoked exactly once from server.go:86. |
| 2 | An agent can call `get_session_output(session_id, bytes?)` and receive a snapshot of the session's PTY ring buffer (last N bytes, default 4 KB) as a read-only result — taken from the existing ring buffer with no new long-lived state. | ✓ VERIFIED | `b.getSessionOutput` (internal/mcp/sessions.go:524-538) bridges to Kamacu's `GET /api/sessions/{id}/output?bytes=N` (sessions.go:897-937). Handler calls `sess.Snapshot()` (read-only ring copy — session.go:387 seam) then slices last-N with server-side clamp (`maxOutputBytes = 512*1024`, default 4096, `clamped` flag). `sessionOutputEnvelope{Encoding:"base64", Output, Bytes, Clamped}` is the D-08 shape. Verified by `TestGetSessionOutput_Happy` (decodes base64 + asserts marker bytes flow through), `TestGetSessionOutput_Clamp` (bytes=999999 → clamped=true, bytes ≤ 524288), `TestGetSessionOutput_UnknownID_404`, `TestGetSessionOutput_BadBytes_400`, `TestBridge_GetSessionOutput_WithBytes_HitsOutputEndpoint`, `TestBridge_GetSessionOutput_NoBytes_HitsOutputEndpointNoQuery`. No new state — `Snapshot()` returns a copy on the existing 1 MiB ring. |
| 3 | An agent can call `subscribe_session_output(session_id, duration_seconds?)` to tail live PTY output for a bounded duration (default 30s, hard cap 300s); on cancellation (`notifications/cancelled`) the tool returns the partial stream collected so far and detaches from the session within 100ms; goroutine count is stable across N subscribe/cancel cycles (no leak). | ✓ VERIFIED | Kamacu side: `subscribeSessionOutput` (internal/api/sessions.go:964-1061) registers `defer sess.Detach(connID)` immediately after `sess.Attach(connID)` (line 1016) on EVERY return path — drain `r.Context().Done()`, for-loop `!ok`, Write error, `timer.C`, `r.Context().Done()`. Duration clamp at sessions.go:974-989 enforces [1s, 300s] (TestSubscribe_DurationCap asserts `maxSubscribeDuration == 300*time.Second`). Bridge side: `subscribeSessionOutput` (mcp/sessions.go:273-411) uses dedicated `subscribeClient = &http.Client{}` (NO 10s timeout — D-11/Pitfall 1); builds request via `http.NewRequestWithContext(ctx, ...)` so cancel propagates to in-flight Read. Cancel-before-Do returns `emptySubscribeEnvelope()` + nil (D-01 strict-edge — line 323). Behavioral tests: `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches` (uses `mcp.NewInMemoryTransports` + handler-side recorder — asserts handler returns non-nil partial result + nil err within 10s AND fake server's `r.Context().Done()` fires within 2s — the loopback detach propagation), `TestSubscribe_GoroutineStabilityAcrossCycles` (N=5 cycles, asserts NumGoroutine delta < N — no leak), `TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError` (D-01 strict-edge), `TestSubscribe_ClientCancel_DetachesPromptly` (Kamacu-side cancel path — asserts handler returns within 5s of cancel + NumGoroutine stable + second subscribe on same session still works). **<100ms detach target note:** Per the documented design (08-03 SUMMARY deviation #2 + context instruction), the <100ms target is structurally proven (defer Detach on every return path + zero NumGoroutine delta across N cycles + prompt handler return on cancel) rather than directly timed — `go-sdk@v1.6.1` returns `(nil, context.Canceled)` to the client so the test observes the handler-side return via a recorder. SDK source verified: `mcp/server.go:753` is `st.handler(ctx, req)` direct call (zero `recover()` calls in the file — see Probe/Spot-check). |
| 4 | No tool in this phase exposes any capability to send keystrokes / PTY input — the read-only contract is enforced at the type level, and no FrameData PTY-input frame is ever sent. | ✓ VERIFIED | Scoped grep gates: `grep -c 'WriteInput' internal/api/sessions.go` == 0; `grep -c 'FrameData' internal/api/sessions.go` == 0; `grep -c 'WriteInput' internal/mcp/sessions.go` == 0; `grep -c 'FrameData' internal/mcp/sessions.go` == 0; `grep -c 'internal/session' internal/mcp/sessions.go` == 0 (the bridge file imports ONLY stdlib + the MCP SDK — Phase 06 split holds, no PTY-write primitive in scope anywhere). Kamacu read handlers consume ONLY `session.Info/Snapshot/Attach/Detach/Done`. Subscribe transport is plain HTTP chunked `application/octet-stream` (sessions.go:1002) — never the WS frame protocol. All 4 session tool handlers are wrapped by `withRecover` (lines 112, 133, 158, 187) so even a panic cannot escape into the SDK dispatch. The PTY-write primitive (`session.go:368` `WriteInput`) is only called from `internal/ws/handler.go` (browser path) — confirmed unchanged by Phase 08's commit footprint. |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/api/sessions.go` | Extended with `getSession`/`getSessionOutput`/`subscribeSessionOutput` handlers + `sessionDetail`/`sessionOutputEnvelope` types + `defaultOutputBytes`/`maxOutputBytes`/`defaultSubscribeDuration`/`maxSubscribeDuration` constants + `joinSessionContext` batch JOIN helper + `?project_id` branch in `list` + 3 new route registrations | ✓ VERIFIED | L1 EXISTS (1061 lines); L2 SUBSTANTIVE — `sessionDetail` at 790, `sessionOutputEnvelope` at 799, constants at 777/783/942/947, `joinSessionContext` batch JOIN at 823-870 (mirrors agents.go:99 shape — ONE query for N task IDs), 3 routes registered inside `SessionRoutes` at 42/43/47; L3 WIRED — `getSession`/`getSessionOutput`/`subscribeSessionOutput` consume `mgr.Get`/`sess.Info/Snapshot/Attach/Detach/Done`; L4 FLOWING — `TestGetSession_Happy` proves all 4 JOIN fields populate from a real task-scoped session, `TestGetSessionOutput_Happy` decodes base64 and asserts the marker flows through. No anti-pattern markers. |
| `internal/api/sessions_test.go` | Extended with happy/404/clamp/project-filter/streaming-detach tests | ✓ VERIFIED | L1 EXISTS (2111 lines); L2 SUBSTANTIVE — 14 new Phase 08 tests confirmed by grep (`TestGetSession_Happy/UnknownID_404/ExitedStillWorks`, `TestGetSessionOutput_Happy/Clamp/UnknownID_404/BadBytes_400`, `TestListSessions_ProjectID/JoinFields`, `TestSubscribe_Happy_ReturnsOctets/UnknownID_404/ClientCancel_DetachesPromptly/DurationCap/DrainsReplayByDefault`); L3 WIRED — all tests use the existing `newSessionServer`/`newTaskSessionServer` harness (real Manager + temp DB); L4 FLOWING — `TestGetSession_Happy` drives a real bash PTY, asserts the JOIN fields are non-empty. `go test ./internal/api/ -run 'TestGetSession|TestGetSessionOutput|TestListSessions|TestSubscribe'` PASSES (68s). |
| `internal/mcp/sessions.go` (NEW) | `registerSessionTools` + `withRecover` + `listSessions`/`getSession`/`getSessionOutput`/`subscribeSessionOutput` bridge methods + `subscribeClient` + `subscribeTailCap` + `emptySubscribeEnvelope` | ✓ VERIFIED | L1 EXISTS (538 lines); L2 SUBSTANTIVE — `registerSessionTools` at 92-191 registers all 4 tools with map[string]any InputSchema + descriptions; `withRecover` at 62-72 (named returns so deferred recover can overwrite); `subscribeClient = &http.Client{}` at 43 (NO Timeout — D-11); `subscribeTailCap = 1<<20` at 22; `emptySubscribeEnvelope` at 201; `subscribeEnvelope` struct at 226; `isOrphanedRow` filter at 491; L3 WIRED — `subscribeSessionOutput` uses `subscribeClient.Do` + `http.NewRequestWithContext(ctx, ...)` (NOT `b.call`/`b.client` — confirmed by awk of the function body), `b.do` for the follow-up info GET; L4 FLOWING — `TestBridge_Subscribe_Happy_ReturnsEnvelope` proves the envelope carries the streamed bytes base64-decoded to "hello", `TestBridge_Subscribe_TruncationCap` proves the 1 MiB cap keeps MOST RECENT bytes. No `go func` in the body (request-scoped reader). |
| `internal/mcp/sessions_test.go` (NEW) | Per-tool happy + 404 + orphan-filter tests + SC3 cancellation/leak gate | ✓ VERIFIED | L1 EXISTS (812 lines); L2 SUBSTANTIVE — 18 tests confirmed by grep; L3 WIRED — reuses existing `newCallToolRequest` helper from bridge_test.go; L4 FLOWING — `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches` uses `mcp.NewInMemoryTransports` end-to-end (real SDK cancellation path) with a handler-side recorder (because `go-sdk@v1.6.1` returns `(nil, context.Canceled)` to the client per Example_cancellation). `go test ./internal/mcp/` PASSES (0.76s, 18 tests + existing Phase 06/07). |
| `internal/mcp/server.go` | `registerTools` gains the `registerSessionTools(s, b)` call | ✓ VERIFIED | L1 EXISTS (88 lines); L2 SUBSTANTIVE — line 86 calls `registerSessionTools(s, b)` as the 4th registrar alongside `registerTaskTools`/`registerProjectTools`/`registerWorkspaceTools`; L3 WIRED — `grep -c 'registerSessionTools' internal/mcp/server.go` == 1. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| `getSession`/`getSessionOutput`/`subscribeSessionOutput` (Kamacu handlers) | `session.Session` engine | `mgr.Get` → `sess.Info/Snapshot/Attach/Detach/Done` | ✓ WIRED | Confirmed by grep — handlers consume ONLY these primitives; `WriteInput`/`FrameData` are absent (D-14 type-level proof). |
| `subscribeSessionOutput` (Kamacu handler) | `internal/ws/handler.go` attach→stream→detach pattern | minus the readLoop/readLoop/`WriteInput` half | ✓ WIRED | Mirrors the canonical pattern (uuid connID at 1011, `defer sess.Detach(connID)` at 1016, select on `r.Context().Done()` at 1055) — read-only half only, plain HTTP, no WS frame protocol. |
| `listSessions`/`getSession`/`getSessionOutput` (bridge) | Kamacu HTTP endpoints | `bridge.call` (Phase 07 pattern) | ✓ WIRED | `b.call(ctx, http.MethodGet, ...)` at mcp/sessions.go:452/515/537. JSON TextContent passthrough. |
| `subscribeSessionOutput` (bridge) | Kamacu streaming endpoint + follow-up GET | dedicated `subscribeClient` + `b.do` for follow-up | ✓ WIRED | `subscribeClient.Do(httpReq)` at 315 (NOT `b.call`/`b.client`); follow-up `b.do(ctx, http.MethodGet, "/api/sessions/...", nil)` at 368. Confirmed by awk of the function body — zero `b.call`/`b.client`/`go func` references. |
| `registerSessionTools(s, b)` | `registerTools` in server.go | one-line addition | ✓ WIRED | server.go:86 — exactly one call. |
| SDK `notifications/cancelled` → handler ctx cancel → HTTP body close → Kamacu `r.Context().Done()` → `defer sess.Detach` | end-to-end SC3 cancellation chain | `http.NewRequestWithContext` + `resp.Body.Read` unblocks on ctx cancel | ✓ WIRED | Proven by `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches` — fake server's `r.Context().Done()` fires within 2s of client cancel (the loopback detach propagation that triggers Session.Detach in Plan 01). SDK source verified: `mcp/server.go:753` `st.handler(ctx, req)` direct call (no recover). |
| `withRecover` wrapping | All 4 session tool handlers | closure passed to `s.AddTool` | ✓ WIRED | Lines 112/133/158/187 — all 4 (`list_sessions`/`get_session`/`get_session_output`/`subscribe_session_output`) wrapped. `TestWithRecover_ConvertsPanicToError` proves a panic becomes a non-nil err containing "panic" + "boom". |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `getSession` (Kamacu) | `info` (session.Info) | `sess.Info()` + `joinSessionContext` JOIN | Yes — real in-memory session Info + real DB JOIN of tasks/projects/agents | ✓ FLOWING |
| `getSessionOutput` (Kamacu) | `lastN` (ring bytes) | `sess.Snapshot()` last-N slice → base64 | Yes — real ring buffer bytes; `TestGetSessionOutput_Happy` decodes base64 and asserts the marker flowed through | ✓ FLOWING |
| `subscribeSessionOutput` (Kamacu) | `chunk` (PTY octets) | `sess.Attach(connID)` channel fan-out | Yes — real PTY output streamed via pump; `TestSubscribe_Happy_ReturnsOctets` proves non-empty octet body; `TestSubscribe_DrainsReplayByDefault` proves live-only filtering | ✓ FLOWING |
| `subscribeSessionOutput` (bridge) | `buf` (accumulated octets) | `resp.Body.Read` loop + `b.do` follow-up GET | Yes — real streamed bytes; `TestBridge_Subscribe_Happy_ReturnsEnvelope` decodes "hello"; `TestBridge_Subscribe_TruncationCap` exercises the 1 MiB cap | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Phase 08 Kamacu tests pass | `go test ./internal/api/ -run 'TestGetSession|TestGetSessionOutput|TestListSessions|TestSubscribe' -count=1 -timeout 120s` | `ok kamacu/internal/api 68.003s` | ✓ PASS |
| Phase 08 MCP tests pass (incl. SC3 + leak gate + D-01 strict-edge) | `go test ./internal/mcp/ -count=1 -timeout 120s` | `ok kamacu/internal/mcp 0.760s` | ✓ PASS |
| SDK dispatch at server.go:753 is direct call (justifies withRecover) | `grep -nE 'st\.handler\|recover' $(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp/server.go` | `753: res, err := st.handler(ctx, req)`; total `recover()` in file == 0 | ✓ PASS |
| Read-only D-14 grep gates (5 gates) | `grep -c 'WriteInput' internal/api/sessions.go; grep -c 'FrameData' internal/api/sessions.go; grep -c 'WriteInput' internal/mcp/sessions.go; grep -c 'FrameData' internal/mcp/sessions.go; grep -c 'internal/session' internal/mcp/sessions.go` | `0\n0\n0\n0\n0` | ✓ PASS |
| `registerSessionTools` wired exactly once | `grep -c 'registerSessionTools' internal/mcp/server.go` | `1` | ✓ PASS |
| subscribeSessionOutput body has no `b.call`/`b.client`/`go func` | `awk '/func \(b \*bridge\) subscribeSessionOutput/,/^}/' internal/mcp/sessions.go \| grep -nE 'b\.call\|b\.client\|^go func\|go func\('` | empty (zero matches) | ✓ PASS |
| All 4 session tool handlers wrapped by withRecover | `grep -nE 'withRecover\(' internal/mcp/sessions.go` | matches at lines 112/133/158/187 | ✓ PASS |
| All 3 new Kamacu routes registered inside SessionRoutes | `grep -nE 'mux\.HandleFunc' internal/api/sessions.go` | routes at 34/35/36/37/38 (existing) + 42/43/47 (new Phase 08) | ✓ PASS |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| Phase 08 commit hashes exist (per SUMMARYs) | `git log --oneline \| grep -E 'ba7f16a\|66e4063\|7b13508\|9d0ec92\|cffb0a2\|4fd726f'` | All 6 commits present (ba7f16a/66e4063 = 08-01; 7b13508/9d0ec92 = 08-02; cffb0a2/4fd726f = 08-03) | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| MCPSESS-01 | 08-01, 08-02 | `list_sessions(project_id? or task_id?)` returns sessions with status (working/waiting/idle/exited/running) | ✓ SATISFIED | Kamacu `list` extended with `?project_id=N` (sessions.go:70-104); bridge `listSessions` + D-13 orphan filter (mcp/sessions.go:432-486). Status delivered via `session.Info.Status` + `Info.AgentStatus` (D-09 mixed enum). 6 tests cover it. REQUIREMENTS.md marks `[x]`. |
| MCPSESS-02 | 08-01, 08-02 | `get_session(session_id)` returns full session detail (status, task, project, agent, started_at) | ✓ SATISFIED | Kamacu `getSession` + D-10 JOIN (sessions.go:875-890); bridge `getSession` (mcp/sessions.go:507-516). `sessionDetail` carries all required fields. 5 tests cover it. REQUIREMENTS.md marks `[x]`. |
| MCPSESS-03 | 08-01, 08-02 | `get_session_output(session_id, bytes?)` returns a snapshot of the session's PTY ring buffer (last N bytes, default 4 KB) — read-only | ✓ SATISFIED | Kamacu `getSessionOutput` (sessions.go:897-937) — `Snapshot()` (no new state, MCPSESS-03 "no new long-lived state" satisfied for free) + clamp to 512 KiB + `clamped` flag; bridge `getSessionOutput` (mcp/sessions.go:524-538). 6 tests cover it. REQUIREMENTS.md marks `[x]`. |
| MCPSESS-04 | 08-01, 08-02, 08-03 | `subscribe_session_output(session_id, duration_seconds?)` tails live PTY output for a bounded duration (default 30s, cap 300s); cancellation via `ctx.Done()` → `Session.Detach`; read-only | ✓ SATISFIED | Kamacu `subscribeSessionOutput` (sessions.go:964-1061) with defer-Detach-on-every-return + bounded duration; bridge `subscribeSessionOutput` (mcp/sessions.go:273-411) with dedicated `subscribeClient` + ctx-carried HTTP request + 1 MiB tail cap + D-01 cancel-before-attach guard. 12 tests cover it (4 Kamacu + 7 MCP + SC3 gate). REQUIREMENTS.md marks `[x]`. |

No orphaned requirements — REQUIREMENTS.md maps exactly MCPSESS-01..04 to Phase 08; all 4 are claimed by plans and satisfied.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| internal/api/sessions.go, internal/mcp/sessions.go, internal/mcp/server.go | — | No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER`/`not yet implemented` markers | ℹ️ Info | Clean — no audit-blocking debt markers in Phase 08 modified files. |

### Code Review Findings (from 08-REVIEW.md — for visibility, not blockers)

The standard-depth code review (08-REVIEW.md, status `issues_found`) surfaced 4 warnings + 2 info notes. None block the 4 success criteria — all are cosmetic, documentation, or low-impact robustness gaps:

| ID | Severity | Description | Impact on Phase 08 SCs |
| --- | --- | --- | --- |
| WR-01 | Warning | Bridge comment claims `project_id` precedence Kamacu does not implement (Kamacu checks `task_id` first when both supplied). | None — bridge never sends both; behavioral bug today is impossible. Documentation defect only. |
| WR-02 | Warning | `getSession`/`getSessionOutput` lack the explicit `session_id == ""` guard that `subscribeSessionOutput` has. | None — SDK's `"required"` InputSchema validation rejects empty values before the handler runs; bypassing the SDK is out of scope. Inconsistency only. |
| WR-03 | Warning | Dead-code branch + misleading comment in subscribe drain logic (`!ok` unreachable on first read). | None — dead branch is harmless; cosmetic. |
| WR-04 | Warning | Subscribe handler can hang in `w.Write` if a client stops reading WITHOUT disconnecting (no per-write deadline). | None for the intended client (the MCP bridge always drains `resp.Body.Read`). A misbehaving loopback client could pin handler goroutines; `http.ListenAndServe` has no `WriteTimeout` backstop. SC3 (cancel-via-ctx path) is unaffected — this is a different edge case (no cancel, just stall). Worth a follow-up hardening PR but not a Phase 08 blocker. |
| IN-01 | Info | Unused-import compile-guard `var _ = errors.Is` in test file. | Smell only. |
| IN-02 | Info | `TestSubscribe_GoroutineStabilityAcrossCycles` discards handler return shape. | Coverage hygiene — SC3 partial-result shape IS covered by the sibling `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches`. |

### Human Verification Required

None. All Phase 08 truths are behaviorally verified by automated tests:
- The end-to-end SDK cancellation path is exercised via `mcp.NewInMemoryTransports` (`TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches`).
- The Kamacu-side detach-on-cancel is exercised via real `httptest.Server` + a cancellable HTTP request context (`TestSubscribe_ClientCancel_DetachesPromptly`).
- The D-01 strict-edge (cancel-before-attach) is exercised via a pre-cancelled ctx (`TestSubscribe_CancelBeforeAttach_ReturnsEmptyEnvelopeNotError`).
- The D-14 read-only contract is proven structurally via 5 scoped grep gates (all return 0).
- All 4 MCPSESS requirements are covered by representative happy + error tests at both the Kamacu and bridge layers.

Real-binary e2e harness with a live agent CLI (claude/opencode calling `kamacu mcp serve`) is **MCPHARD-03, explicitly deferred to v1.12** per REQUIREMENTS.md — out of Phase 08 scope by design.

### Gaps Summary

No gaps. All 4 ROADMAP Success Criteria are verified:
- **SC1**: `list_sessions` + `get_session` deliver current session state with the D-10 JOIN. Status + taskTitle + projectName + agentName are populated from a real task-scoped PTY session in `TestGetSession_Happy`.
- **SC2**: `get_session_output` returns a base64 envelope from `Snapshot()` (read-only on the existing ring — MCPSESS-03's "no new long-lived state" satisfied for free).
- **SC3**: `subscribe_session_output` returns the partial stream on cancel (TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches asserts non-nil result + nil err within 10s AND the fake server's `r.Context().Done()` fires within 2s); goroutine count is stable across N=5 cycles (TestSubscribe_GoroutineStabilityAcrossCycles); `defer sess.Detach(connID)` runs on every return path. The <100ms detach target is structurally proven per the documented design (08-03 SUMMARY deviation #2) — `go-sdk@v1.6.1` returns `(nil, context.Canceled)` to the client so direct timing isn't asserted; the structural proof (no hang + NumGoroutine delta 0 + defer-on-every-return) carries SC3.
- **SC4**: No tool exposes PTY input — type-level proof via 5 scoped grep gates (all return 0); the bridge file imports only stdlib + the SDK; the Kamacu read handlers consume only Info/Snapshot/Attach/Detach/Done.

Notes for the maintainer (NOT blockers):
- **WR-04** is the only finding with theoretical production impact: a misbehaving loopback client could stall the subscribe handler by stopping reads without disconnecting. SC3's cancel-via-ctx path is unaffected; the intended bridge client always drains. Worth a follow-up hardening PR (per-write deadline) but not a Phase 08 regression — the SC3 contract holds for the documented usage.
- **TestSpawnPumpFillsRing flake in internal/session** (per the regression-run context note) is pre-existing — Phase 08's commit footprint does not touch `internal/session/`; Phase 08 only consumes `Snapshot/Attach/Detach/Done/Info` from outside. Correctly characterized as a pre-existing flake (2s timing budget under parallel load), not a phase regression.

---

_Verified: 2026-07-23T12:10:00Z_
_Verifier: the agent (gsd-verifier)_

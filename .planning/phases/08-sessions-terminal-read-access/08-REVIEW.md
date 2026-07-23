---
phase: 08-sessions-terminal-read-access
reviewed: 2026-07-23T11:30:00Z
depth: standard
files_reviewed: 5
files_reviewed_list:
  - internal/api/sessions.go
  - internal/api/sessions_test.go
  - internal/mcp/server.go
  - internal/mcp/sessions.go
  - internal/mcp/sessions_test.go
findings:
  critical: 0
  warning: 4
  info: 2
  total: 6
status: issues_found
---

# Phase 08: Code Review Report

**Reviewed:** 2026-07-23T11:30:00Z
**Depth:** standard
**Files Reviewed:** 5
**Status:** issues_found

## Summary

The Phase 08 read-only session surface (three Kamacu HTTP read endpoints + their `?project_id` list filter, three non-streaming MCP bridge tools + the `withRecover` panic wrapper, and the streaming `subscribe_session_output` tool) is **largely correct**. Both load-bearing contracts the brief asked me to verify are **held**:

- **D-14 (read-only):** `internal/api/sessions.go`'s `getSession` / `getSessionOutput` / `subscribeSessionOutput` consume only `session.Info` / `Snapshot` / `Attach` / `Detach` / `Done`. Zero references to the PTY-write primitive (`WriteInput` / `ptmx.Write` / WS `FrameData` input frames) on these paths — confirmed by grep and by reading session.go (the write primitive is `s.ptmx.Write` at line 379, reached only via `WriteInput`, which none of the read handlers call). `internal/mcp/sessions.go` imports only stdlib + the MCP SDK — no in-repo session-engine import at all.
- **SC3 (detach-on-cancel):** `subscribeSessionOutput` registers `defer sess.Detach(connID)` immediately after `sess.Attach(connID)`, and every subsequent return path (drain `!ok`, drain `r.Context().Done()`, for-loop `!ok`, for-loop Write error, `timer.C`, for-loop `r.Context().Done()`, and any panic) triggers the defer. `session.Detach` only removes the conn from the fan-out map — it never touches the PTY. The API-side `TestSubscribe_ClientCancel_DetachesPromptly` and the MCP-side `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches` + `TestSubscribe_GoroutineStabilityAcrossCycles` verify this end-to-end.

The streaming tool's divergences from the Phase 06/07 bridge pattern are **deliberate and correct**: `subscribeClient` (no `Timeout`) is required because `b.client`'s 10s timeout would kill every >10s tail; the SDK handler ctx (cancelled on `notifications/cancelled`) is the documented cancellation mechanism and propagates via `http.NewRequestWithContext` → transport-level Read unblock → `resp.Body.Read` returns a ctx error → loop breaks → partial envelope assembled. The 1 MiB tail cap with trim-front is correct (keeps the MOST RECENT `subscribeTailCap` bytes; `truncated=true` when hit).

No BLOCKER-level defects were found. The six findings below are documentation defects, a minor validation inconsistency, dead code with a misleading comment, and a robustness gap with low practical impact for the intended (always-reading) bridge client.

## Warnings

### WR-01: Bridge comment claims project_id precedence that Kamacu does not implement

**File:** `internal/mcp/sessions.go:429-431`
**Issue:** The `listSessions` doc comment asserts:

> "project_id takes precedence over task_id when both are supplied (matches the existing Kamacu list_sessions handler's filter precedence; supplying both is rare and either resolution is fine)."

This is **factually wrong** about the Kamacu side. `internal/api/sessions.go:64-119` checks `task_id` FIRST (outer `if q := ...Get("task_id"); q == ""`), so when both query params are supplied, **task_id wins** on the server, not project_id. The bridge builds only one query param so there is no behavioral bug today, but the comment misrepresents the server contract and would mislead a future maintainer who relies on it (e.g., if they add client-side coalescing of both filters, they will assume Kamacu agrees with the bridge's precedence when it does not).

**Fix:** Either correct the comment to match reality, or make the precedence explicit on both sides. Minimal fix is the comment:
```go
// project_id takes precedence over task_id ON THE BRIDGE SIDE when both are
// supplied; note Kamacu's handler (sessions.go:64) checks task_id first, so
// the server actually prefers task_id. The bridge never sends both, so the
// divergence is harmless in practice but the two sides disagree on paper.
```

### WR-02: Inconsistent `session_id` empty-check across the three MCP session tools

**File:** `internal/mcp/sessions.go:284-286` vs `:511-515` and `:529-533`
**Issue:** `subscribeSessionOutput` explicitly guards against an empty session_id:
```go
if args.SessionID == "" {
    return nil, errors.New("subscribe_session_output: session_id is required")
}
```
But `getSession` (line 507) and `getSessionOutput` (line 524) do NOT — they `url.PathEscape` whatever arrives and call Kamacu. The InputSchema marks `session_id` as `"required"`, so the SDK's own validation should reject empty values before the handler runs; but if a caller bypasses the SDK (e.g., an in-process test, a future JSON-RPC client that does not enforce `required`), `getSession("")` builds `GET /api/sessions/` (empty path segment) which the ServeMux will not route to `{id}` — the caller sees a confusing 404/405 instead of a crisp "session_id is required" error. The inconsistency also means the three sibling tools have different validation postures for the same required field.

**Fix:** Hoist the empty-check into a shared helper, or replicate the guard in `getSession` and `getSessionOutput`:
```go
func (b *bridge) getSession(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    var args struct {
        SessionID string `json:"session_id"`
    }
    if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
        return nil, fmt.Errorf("get_session: invalid arguments: %w", err)
    }
    if args.SessionID == "" {
        return nil, errors.New("get_session: session_id is required")
    }
    ...
}
```

### WR-03: Dead-code branch + misleading comment in subscribe drain logic

**File:** `internal/api/sessions.go:1023-1030`
**Issue:** The `include_history=false` drain block:
```go
select {
case _, ok := <-q:
    if !ok {
        return // session exited before any replay — queue already closed
    }
case <-r.Context().Done():
    return
}
```
The `if !ok` branch is **unreachable** and the comment is **wrong**. `session.Attach` (session.go:342-353) ALWAYS buffers the ring replay as the first queue message BEFORE any close:
```go
q <- append([]byte(nil), s.ring.Bytes()...)
if s.status == StatusExited {
    close(q)
    return q
}
```
So the first `<-q` always returns `(replayBytes, true)` — the channel can only be observed as closed on a SUBSEQUENT read. There is no "session exited before any replay" state: an exited session still delivers its replay first, then closes. The comment will mislead anyone debugging an exited-session subscribe flow into thinking the empty-body case is handled here (it is not — the for-loop handles it via its own `!ok` at line 1038).

This is not a correctness bug (the dead branch is harmless) but it is a misleading-invariant comment in a concurrency-sensitive code path, which is exactly where misleading comments cause future regressions.

**Fix:** Remove the dead branch and fix the comment, OR keep the branch but document why it cannot fire:
```go
select {
case _, ok := <-q:
    // Attach always buffers the ring replay before any close, so ok is
    // always true on this first read; discard the replay unseen (D-02).
    _ = ok
case <-r.Context().Done():
    return
}
```

### WR-04: Subscribe handler can hang in `w.Write` if a client stops reading without disconnecting

**File:** `internal/api/sessions.go:1035-1060` (compounded by `cmd/kamacu/serve.go:335`)
**Issue:** The streaming for-loop calls `w.Write(chunk)` OUTSIDE the `select`:
```go
for {
    select {
    case chunk, ok := <-q:
        ...
        if _, err := w.Write(chunk); err != nil {  // ← blocking syscall, no select guard
            return
        }
        ...
    case <-timer.C:
        return
    case <-r.Context().Done():
        return
    }
}
```
If a client opens the subscribe connection and stops reading WITHOUT disconnecting (TCP RST/FIN), the kernel write buffer fills (~64–256 KiB on localhost) and `w.Write` blocks on the syscall. While blocked, the goroutine is NOT in the `select`, so `timer.C` (duration cap) and `r.Context().Done()` cannot fire. The handler hangs until the client either reads, disconnects, or the connection errors at the TCP layer.

The production server (`cmd/kamacu/serve.go:335`) uses `http.ListenAndServe(c.addr, handler)` with NO `WriteTimeout` / `ReadHeaderTimeout`, so there is no server-level backstop. The intended client (the MCP bridge) always drains its `resp.Body.Read` loop, and on cancel it `defer resp.Body.Close()`s which disconnects, so this gap is not practically reachable from the bridge. But a misbehaving or adversarial loopback client (and this is a localhost app, so "the local user" is the threat model) could pin handler goroutines indefinitely. Note: adding `WriteTimeout` to the server is NOT a correct fix because it would also kill legitimate >N-second subscribe streams — the fix must be per-write.

**Fix:** Set a per-write deadline so a stalled Write errors out and the handler returns (defer Detach still runs):
```go
// At the top of the for-loop body, before Write:
if rc, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
    _ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
}
if _, err := w.Write(chunk); err != nil {
    return // stalled or disconnected client — defer Detach cleans up
}
```
Alternatively, push chunks to the response writer from a separate goroutine and select on `<-r.Context().Done()` while it works — but the per-write deadline is the minimal change.

## Info

### IN-01: Unused-import compile-guard `var _ = errors.Is` in test file

**File:** `internal/mcp/sessions_test.go:808-812`
**Issue:** The `errors` package is imported but referenced only via `var _ = errors.Is` at the bottom of the file, with a comment explaining it exists so `go vet` / unused-import checks pass when only some tests reference it. In fact, NO test in the file calls `errors.Is` — the cancellation test (`TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches`) inspects `r.err` via `!= nil`, not `errors.Is`. The guard is masking a genuinely unused import. This is a code smell that makes the import graph harder to audit.

**Fix:** Remove the `errors` import and the `var _ = errors.Is` line.

### IN-02: `TestSubscribe_GoroutineStabilityAcrossCycles` discards handler return shape

**File:** `internal/mcp/sessions_test.go:726-745`
**Issue:** The leak-stability test runs 5 subscribe/cancel cycles but explicitly discards the handler return values:
```go
_ = res // not asserted — the contract is "returns promptly", not "returns a specific shape"
_ = err
```
The test only asserts `runtime.NumGoroutine()` does not grow proportional to N. If the handler were ever refactored to return a transport error on cancel (a real SC3 regression — partial buffer is the contract, not an error), this test would still pass. SC3 correctness IS covered by the sibling `TestSubscribe_CancelledViaContext_ReturnsPartialAndDetaches`, so this is a coverage-hygiene note rather than a gap. Consider asserting `err == nil || errors.Is(err, context.Canceled)` here as well so the leak gate doubles as a partial-result gate.

**Fix:**
```go
// After wg.Wait():
if err != nil && !errors.Is(err, context.Canceled) {
    t.Errorf("cycle %d: handler returned non-cancel err=%v — want partial result or ctx.Canceled", i, err)
}
```

---

_Reviewed: 2026-07-23T11:30:00Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_

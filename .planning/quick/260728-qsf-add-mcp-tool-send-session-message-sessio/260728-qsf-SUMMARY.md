---
quick_id: 260728-qsf
slug: add-mcp-tool-send-session-message-sessio
status: complete
---

# Summary: Add `send_session_message` MCP tool (D-14 write-prohibition reversal, agent-delegate surface only)

## Outcome

Shipped the `send_session_message(session_id, message)` MCP tool and its
backing Kamacu endpoint `POST /api/sessions/{id}/input`. An agent running
inside a Kamacu task PTY can now send a prompt/message to ANOTHER session's
PTY stdin — the one capability v1.11's D-14 read-only-terminal contract
explicitly prohibited. The reversal is **MCP-only**: the browser WebSocket
stays the authoritative interactive surface and is byte-for-byte unchanged.

This closes the last gap between "the agent can observe sibling sessions"
(v1.11 shipped `get_session_output`, `subscribe_session_output`) and "the
agent can drive sibling sessions" (this task). The agent is the user's
delegate at the same trust level as `start_task_agent`, `delete_task`, and
`delete_project`.

## Commits

| # | Commit | Scope |
|---|--------|-------|
| 1 | `feat(api): add POST /api/sessions/{id}/input endpoint (WriteInput bridge)` | Server-side half of the reversal — reuses the existing `(*Session).WriteInput` primitive over HTTP. Newline normalization is the only smart behavior. |
| 2 | `test(api): add happy + 404 + newline-normalization tests for sessions input` | 4 `TestInput_*` tests: happy round-trip, unknown-id 404, newline-already-present no-double-append, empty-message bare-newline. |
| 3 | `feat(mcp): add send_session_message tool` | Entry #6 in `registerSessionTools`, thin `bridge.call` POST passthrough, wrapped in `withRecover`. D-14 import gate preserved. |
| 4 | `test(mcp): add happy + 404 coverage for send_session_message` | 3 `TestBridge_SendSessionMessage_*` tests: happy POST path+body+passthrough, unknown-id 404 D-05 wrap, empty-message still posted (no client-side validation). |

## Files Modified

- `internal/api/sessions.go` — 1 new route line (`POST /api/sessions/{id}/input`)
  + 1 new handler method (`input`). No other handler/route/signature changed.
- `internal/api/sessions_test.go` — 4 appended tests (reused `newSessionServer`,
  `doJSON`, `getJSON`; no new helpers).
- `internal/mcp/sessions.go` — 1 new `AddTool` block (entry #6 in
  `registerSessionTools`) + 1 new handler method (`sendSessionMessage`).
- `internal/mcp/sessions_test.go` — 3 appended tests (reused
  `newCallToolRequest` + canonical `*bridge` literal; no new helpers).

## Verification

- `go build ./...` — passes.
- `go vet ./internal/api/... ./internal/mcp/...` — passes.
- `go test ./internal/api/...` — passes (incl. the 4 new `TestInput_*` tests).
- `go test ./internal/mcp/...` — passes (incl. the 3 new
  `TestBridge_SendSessionMessage_*` tests).

## Invariants Preserved

- **D-14 import gate holds**: `internal/mcp/sessions.go` still does NOT import
  `internal/session`. The relaxed write prohibition lands server-side in
  `internal/api/sessions.go` (which already imports `internal/session`),
  reached over HTTP only. The bridge has ZERO direct references to `WriteInput`
  or `FrameData` (verified via scoped grep — every textual match is in an
  explanatory comment, never a call or import).
- **Browser WS surface untouched**: zero files under `web/src/` or
  `internal/ws/` modified. The browser remains the authoritative interactive
  surface.
- **PTY-write primitive reused, not duplicated**: the endpoint calls
  `sess.WriteInput([]byte(msg))` — the exact primitive at
  `internal/session/session.go:368`, the same code path the browser WS handler
  calls at `internal/ws/handler.go:164`. No new PTY-write code introduced.
- **`routes.go` untouched**: session routes live in `SessionRoutes()` at
  `internal/api/sessions.go`; the new route went there. `cmd/` untouched too.
- **`withRecover` wraps the new closure**: every session tool handler in
  `internal/mcp/sessions.go` is wrapped, including `send_session_message`.
- **No new long-lived goroutines**: the `input` handler is a synchronous
  decode-write-respond — the milestone rule is intact.

## Security Posture (acknowledged risk, NOT fixed here)

The prompt-injection escalation path (agent A reads a malicious tool result →
injected → calls `send_session_message` on agent B → drives B to destructive
actions) is a REAL risk and is intentionally not mitigated technically in
this task. The mitigation is the user's authority model: the user trusts the
agent they spawned with the same authority they themselves have. This is
identical to the posture of `start_task_agent`, `delete_task`, and
`delete_project`. Technical rate-limiting / send-quotas / inter-agent
permission prompts are out of scope for this quick task.

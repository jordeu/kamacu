---
quick_id: 260728-sm5
slug: fix-send-session-message-split-the-pty-w
status: complete
commits: 2
files_modified:
  - internal/api/sessions.go
  - internal/api/sessions_test.go
---

# Quick Task 260728-sm5: Split text/CR write in send_session_message

## Outcome

The POST `/api/sessions/{id}/input` handler now performs **two separate
`sess.WriteInput` calls in sequence** — the message body first (any
request-supplied terminator stripped), then a single `"\r"` submit key —
instead of one combined `(body + "\r")` write.

## Why

A single combined write trips the paste-detection heuristic in raw-mode TUIs
(Claude Code via Ink): the chunk "looks pasted", so the embedded CR is
treated as paste content rather than the Enter key. The prompt text lands in
the input box but the agent never acts on it — it stays idle. Multi-byte
UTF-8 and longer messages triggered this reliably; short ASCII sometimes
slipped through. Splitting the write mirrors how human typing reaches the PTY
(text in one stdin chunk, Enter in the next) and is robust regardless of
content or size. The xterm.js browser WS path was never affected — it
produces per-keystroke chunks naturally; only the HTTP bridge produced one
big chunk per request.

## What changed

### `internal/api/sessions.go`
- `normalizeInputTerminator(msg) string` → `splitInputForWrite(msg) (body, submit string)`. The helper now returns the ordered pair of payloads the handler writes, expressing the two-write contract directly. The terminator-stripping logic (`TrimSuffix` of `"\n"` then `"\r"`) is preserved verbatim — only the return shape changed from a single combined string to a pair.
- The `input` handler writes `body` and `submit` as two sequential `sess.WriteInput` calls. If the body write fails, it returns 409 immediately and never sends the submit key (no half-state). If the submit write fails, it returns 409 — the body landed but the prompt is staged unsubmitted, and the caller knows.
- 200 response reports `bytes_written: len(body) + len(submit)` (submit is always 1 byte, so this equals the pre-fix value — `len(stripped body) + 1`).
- Empty body is still legal: a 0-byte body write followed by a 1-byte `"\r"` write submits whatever was already staged.
- Comment blocks above both the handler and the helper document the split-write rationale and the paste-detection root cause.

### `internal/api/sessions_test.go`
- `TestNormalizeInputTerminator` → `TestSplitInputForWrite`. The regression guard now asserts the ordered `(body, submit)` pair returned by the helper:
  - Exactly two payloads (body, then `"\r"`).
  - The submit key is always exactly one byte `0x0d` — never empty, never `"\n"`, never doubled.
  - The body never carries a trailing terminator byte (guards against the combined-write shape returning).
  - Cases cover all four terminator shapes (none / `"\n"` / `"\r"` / `"\r\n"`), empty message, **and a multi-byte UTF-8 case** (`"ping — test"`) — the regression guard for the original failing symptom.
- Doc comments on the three HTTP round-trip tests (`TestInput_Happy_WritesAndAppendsCR`, `TestInput_TrailingLF_TranslatedToCR`, `TestInput_EmptyMessage_WritesBareCR`) updated to describe the two-write contract and reference `TestSplitInputForWrite`. Their `bytes_written` assertions are unchanged — the count is identical before and after the split (`len(body) + 1`).

## Verification

- `go build ./...` — clean.
- `go test ./internal/api/... -count=1 -run "Input|SessionInput|SendInput"` — pass (includes `TestSplitInputForWrite` and the three HTTP round-trip tests).

## Out of scope (per plan)

- Per-character typing simulation (one WriteInput per body byte) — overkill.
- Backpressure / write-queue management — the two writes are synchronous.
- MCP tool (`internal/mcp/`) — bridges verbatim, contract unchanged.
- PROJECT.md / ROADMAP.md — untouched.

## Commits

1. `fix(api): split send_session_message write into body + CR (defeat raw-mode TUI paste-detection)` — `internal/api/sessions.go` only.
2. `test(api): assert two ordered WriteInput calls (body, then \r) across all input shapes` — `internal/api/sessions_test.go` only.

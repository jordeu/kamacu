---
quick_id: 260728-t4c
slug: fix-send-session-message-3rd-attempt-wra
status: complete
---

# Quick Task 260728-t4c: Bracketed paste wrap for send_session_message

## Summary

The third and final fix to `send_session_message`. The input handler now wraps
the request body in ANSI bracketed paste markers (`ESC[2004<body>ESC[2014`) as
ONE `WriteInput` call, then writes `"\r"` as a SECOND `WriteInput` call after
the closing bracket. The explicit brackets tell raw-mode TUIs (Claude Code via
Ink, opencode, etc.) "this is one paste event", bypassing the heuristic paste
detector that was absorbing programmatic writes (and their embedded CR) as
paste content rather than submitting them. The trailing CR lands outside the
bracket as a standalone Enter keystroke that submits the captured paste.

## What changed

**`internal/api/sessions.go`** (fix commit):
- `splitInputForWrite(msg) (body, submit)` → `wrapInputForWrite(msg) (body, submit)`.
  The helper now returns the trimmed body wrapped in `\x1b[2004...\x1b[2014`
  as the first payload, plus `"\r"` as the second. The handler's two-write
  structure and `bytes_written: len(body) + len(submit)` formula are unchanged
  — only the body payload's content changed (now bracketed).
- Doc comments on the handler and helper updated to describe the three-layer
  iterative diagnosis (260728-s5a set the `\r` submit key; 260728-sm5 split the
  two writes; 260728-t4c added the bracketed wrap — the deterministic fix).

**`internal/api/sessions_test.go`** (test commit):
- `TestSplitInputForWrite` → `TestWrapInputForWrite`. The pure-function guard
  now asserts the EXACT first-write payload (`"\x1b[2004" + expectedBody +
  "\x1b[2014"`) and second-write payload (`"\r"`) across all four terminator
  shapes (none / `\n` / `\r` / `\r\n`) + multi-byte UTF-8 (`ping — test`) +
  empty body + internal-LF cases. Added structural guards: body must start
  with `\x1b[2004` and end with `\x1b[2014` (catches a dropped bracket).
- The three HTTP round-trip tests updated for the new `bytes_written` totals
  (see arithmetic note below).

## Byte-count arithmetic correction (deviation from plan prose)

The plan's prose states the bracket pair is "10 bytes" and `bytes_written ==
len(body) + 11`. **This miscounted `ESC[2004` as 5 bytes — it is 6 bytes.**

`"\x1b[2004"` = ESC (0x1b) + `[` + `2` + `0` + `0` + `4` = **6 bytes**.
`"\x1b[2014"` = ESC (0x1b) + `[` + `2` + `0` + `1` + `4` = **6 bytes**.

So the bracket pair is **12 bytes** (not 10), and `bytes_written ==
len(trimmed_body) + 13` (not `+ 11`):
- 6 (prefix) + len(body) + 6 (suffix) + 1 (CR) = len(body) + 13.

The plan's own pseudocode (`writeJSON(... {"bytes_written": len(body) + 1})`
where `body` is the bracketed string) produces the **honest** count of 13 —
the "11" in the prose was the arithmetic slip, not the code blueprint. The
implementation reports the actual bytes written via the unchanged
`len(body) + len(submit)` formula, and the tests assert the real values
(happy `len+13`, trailing-LF `15`, empty `13`). Reporting a knowingly-wrong
`bytes_written` to match the prose's "11" would have been a defect — the count
is a value callers can rely on.

Verified live (see below) with the plan's exact `\x1b[2004` / `\x1b[2014`
markers; the markers are taken as specified by the plan, which asserts live
verification against Claude Code v2.1.22 / Opus 5.

## Verification

### Build + targeted tests

```
go build ./...
go test ./internal/api/... -count=1 -run "Input|SessionInput|SendInput"
```

Both pass. Targeted run covers 5 top-level input tests + `TestWrapInputForWrite`
(8 subtests: 4 terminator shapes + empty + multi-byte UTF-8 + 2 internal-LF
cases), all green.

### Live verification (from plan diagnosis, pre-commit)

Tested against Kamacu session `70e5cae0-cb84-41f2-8c14-3f2897e7ac61` (task 221,
sched project, Claude Code Opus 5):

1. **Before:** `agentStatus=idle`, prompt showing leftover state from prior
   user tests.
2. Cleared the prompt with `\u001b` (Esc) + `\u0015` (Ctrl+U) + `\u001b` (Esc).
3. Sent raw body `\u001b[2004ACK-BRACKET-TEST-9\u001b[2014` via the existing
   endpoint — the endpoint appended `\r` as a separate write (post-260728-sm5
   behavior). This is the exact sequence the new `wrapInputForWrite` produces.
4. **After 5s:** `agentStatus=working`, tail shows "thinking with xhigh effort".
   **Submitted.** The idle→working transition confirms the bracketed body +
   standalone CR deterministically submits against the real Claude Code TUI.

The previous split-write fix (260728-sm5) was necessary (the `\r` must be its
own write) but insufficient (the body chunk still tripped the heuristic without
explicit brackets). The bracketed wrap is the deterministic resolution.

## Out of scope (per plan)

- Per-character typing simulation (the brackets are deterministic; per-char is
  the wrong abstraction).
- Bracket-escape for messages containing literal `ESC[2004`/`ESC[2014`
  sequences (documented known limitation; agent prompts rarely contain these).
- Conditional bracketing by session kind (always bracket for now).
- MCP tool changes (`internal/mcp/`) — the bridge passes verbatim.
- PROJECT.md / ROADMAP.md updates.

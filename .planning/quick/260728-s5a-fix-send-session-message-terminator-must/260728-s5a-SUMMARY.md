---
quick_id: 260728-s5a
slug: fix-send-session-message-terminator-must
status: complete
---

# Quick Task 260728-s5a — Summary

## Outcome

Fixed the one-character terminator bug in `POST /api/sessions/{id}/input`
(the `send_session_message` MCP tool's server-side half): the handler now
writes a single trailing `\r` (CR, 0x0d) to the PTY regardless of the
request-supplied terminator shape, instead of `\n`.

**Why it mattered:** raw-mode TUIs (Claude Code via Ink, opencode,
readline/bubbletea-based agents) read `\r` as the Enter key. A `\n` is a
literal line feed that does not submit the prompt — so `send_session_message`
delivered the text but the agent never acted on it. The browser WS path was
unaffected because xterm.js already sends `\r` on Enter.

## Changes

### Commit 1 — `fix(api): normalize send_session_message terminator to \r (CR) for raw-mode TUIs`
- `internal/api/sessions.go`: extracted `normalizeInputTerminator(msg) string`
  which strips any trailing `\n` then any trailing `\r`, then appends a single
  `\r`. This collapses all four input shapes (none / `\n` / `\r` / `\r\n`) to
  a single trailing `\r`. Internal `\n`s in a multi-line body are preserved
  verbatim. The `input` handler now calls the helper; its doc comment was
  updated ("Newline normalization" → "CR-terminator normalization").

### Commit 2 — `test(api): assert \r terminator reaches WriteInput across all input shapes`
- `internal/api/sessions_test.go`:
  - Added `TestNormalizeInputTerminator` — the **load-bearing byte-exact
    regression guard**. Because the handler writes the helper's return value
    verbatim to `WriteInput`, asserting the function's output (last byte ==
    `\r`) IS asserting the last byte written to the PTY. Covers all four
    terminator shapes plus internal-`\n`-preservation cases.
  - Renamed + updated comments for the three existing HTTP round-trip tests
    to reflect CR semantics (`TestInput_Happy_WritesAndAppendsNewline` →
    `...AppendsCR`; `..._NoDoubleAppend` → `...TrailingLF_TranslatedToCR`;
    `..._WritesBareNewline` → `..._WritesBareCR`). Byte-count assertions are
    unchanged (`\r` is also 1 byte).

## Why a pure-function guard (not WriteInput capture)

The handler holds `mgr *session.Manager` (concrete) and `mgr.Get` returns
`*session.Session` (concrete) whose `WriteInput` writes to the unexported
`s.ptmx`. There is no interface seam for a fake Manager, and refactoring the
handler/manager to interfaces was out of scope for a one-character fix.

The HTTP round-trip tests (which drive real bash sessions) cannot distinguish
`\r` from `\n`: bash's cooked-mode line discipline (ICRNL) translates both
identically on input, so the echoed output is the same. This is exactly why
the bug was invisible to the existing test suite.

Extracting the normalization into a pure function makes it precisely testable:
its return value is the exact byte stream handed to `WriteInput`. A table test
over all four shapes asserting the last byte is `0x0d` fails loudly if the
terminator ever reverts to `\n`. This is the strongest load-bearing guard
available without an interface refactor.

## Verification

- `go build ./...` — clean.
- `go test ./internal/api/... -count=1 -run "Input|SessionInput|SendInput|NormalizeInputTerminator"` — `ok kamacu/internal/api 16.383s`.
- `TestNormalizeInputTerminator` subtests all PASS: no_terminator,
  trailing_LF, trailing_CR, trailing_CRLF, empty_message,
  internal_LFs_preserved, internal_LFs_preserved_with_trailing_LF.
- Renamed HTTP round-trip tests all PASS (byte counts coincidentally
  unchanged: `\r` is 1 byte, same as the previous `\n`).

## Scope

No modifications to: `internal/mcp/`, `internal/session/`, `internal/ws/`,
`web/src/`, `cmd/`, `.planning/PROJECT.md`, `.planning/ROADMAP.md`. The MCP
tool's InputSchema and docs string are unchanged (the user-facing contract is
unchanged: "message" still means "a prompt to send to the agent").

## Commits

1. `29e8114` — `fix(api): normalize send_session_message terminator to \r (CR) for raw-mode TUIs`
2. `9c3bf0d` — `test(api): assert \r terminator reaches WriteInput across all input shapes`

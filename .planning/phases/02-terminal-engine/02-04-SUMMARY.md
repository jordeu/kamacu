---
phase: 02-terminal-engine
plan: 04
subsystem: ui
tags: [xterm, websocket, react, terminal, reconnect]

# Dependency graph
requires:
  - phase: 02-terminal-engine (plan 02-02)
    provides: "xterm 6 package set, zincTheme/terminalFontFamily/terminalFontSize, useStopSession + TermSession hooks, WS-capable Vite proxy"
provides:
  - "web/src/components/terminal/useTerminalSocket.ts — WS lifecycle + locked wire protocol (0x30/0x31/0x78) + reconnect state machine (connecting|connected|reconnecting|lost|exited|not-found)"
  - "web/src/components/terminal/TerminalPane.tsx — self-contained pane (header, banners, xterm viewport); props = sessionId + label + status + callbacks only"
  - "TerminalPaneProps interface incl. onNewTerminal/onClosed/onSessionExit for 02-05 TerminalPage composition"
affects: [02-05, 03-task-view]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Socket lifecycle separated from Terminal lifecycle: hook owns WS, pane owns Terminal — instance survives reconnects so scrollback stays usable"
    - "Imperative ref escape hatches (sendResizeRef/retryRef/fitAndSendRef) bridge effect-scoped closures to stable useCallback returns"
    - "Forced fit+resize send on every `connected` transition triggers the server-side SIGWINCH jiggle even when dimensions are unchanged"

key-files:
  created:
    - web/src/components/terminal/useTerminalSocket.ts
    - web/src/components/terminal/TerminalPane.tsx
  modified: []

key-decisions:
  - "retry() emits reconnecting attempt 0 and connects immediately, then the normal 5-step backoff cycle applies to subsequent failures"
  - "Keyboard-only focus ring approximated with has-[.xterm-helper-textarea:focus-visible] — browsers treat textareas as always focus-visible, so pointer clicks may still show the ring (flagged for 02-05 live verification)"
  - "Post-connect resize is force-sent (ignoring change detection) so the server jiggle fires on reattach with unchanged dimensions"

patterns-established:
  - "ConnState discriminated union is the single source of truth for banners, header status, stdin gating, and Stop visibility"
  - "Pane is attach-only: spawning/deleting always delegated to the parent via callbacks (D-12 portability into Phase 3 TaskTabs)"

requirements-completed: [TERM-02, TERM-03, TERM-05]

# Metrics
duration: 8min
completed: 2026-06-10
---

# Phase 2 Plan 04: Terminal Frontend Pane Summary

**useTerminalSocket hook implementing the locked 0x30/0x31/0x78 wire protocol with 0.5–8s ×5 backoff and 4404 short-circuit, plus a self-contained TerminalPane with WebGL/DOM xterm rendering, debounced fit/resize, D-17 clipboard, and the full UI-SPEC banner contract**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-10T10:33:04Z
- **Completed:** 2026-06-10T10:41:17Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments

- `useTerminalSocket` owns only the socket: binary arraybuffer frames, stdin framed as 0x30 (bracketed-paste wrappers untouched per D-17), 0x31 resize frames, 0x78 exit parsing; onclose decision table covers disposed/exited/4404/transient; backoff [500, 1000, 2000, 4000, 8000] with `lost` after 5 failures and `retry()` restarting from attempt 0; `term.reset()` runs on re-connect before any replay frame so the surviving Terminal instance redraws cleanly
- `TerminalPane` mounts xterm 6 with zincTheme, literal 13px mono stack, 10000-line scrollback, cursorBlink, WebGL renderer with `onContextLoss → dispose()` DOM fallback; manual copy-on-select via `onSelectionChange` (no copyOnSelect option exists in xterm 6) and best-effort Ctrl+Shift+C/V via `attachCustomKeyEventHandler`
- ResizeObserver → 75ms debounce → hidden-tab guard (`clientWidth/Height > 0`, ships now for Phase 3 TaskTabs) → `fit.fit()` → resize sent only when cols/rows changed; every `connected` transition force-sends one resize to trigger the server-side jiggle
- Full UI-SPEC connection-state contract with character-exact copy: 150ms-delayed `Connecting…`, `Connection lost. Reconnecting…`, `Couldn't reconnect.` + `Retry connection`, `Session not found.`, `Session exited (code N)` — one docked banner at a time, canvas never dimmed, exited scrollback stays copyable (D-15)
- Zero route coupling: no react-router or pages/ imports; spawn/close always delegated via `onNewTerminal`/`onClosed` callbacks (StrictMode double-mount can never double-spawn)

## Task Commits

Each task was committed atomically:

1. **Task 1: useTerminalSocket — protocol glue + reconnect state machine** - `041a30f` (feat)
2. **Task 2: TerminalPane — xterm mount, renderer, resize, clipboard** - `5a592c1` (feat)
3. **Task 3: Connection-state banners and exited UX** - `64cd813` (feat)

## Files Created/Modified

- `web/src/components/terminal/useTerminalSocket.ts` - WS lifecycle, wire protocol framing, reconnect state machine, ConnState union (182 lines)
- `web/src/components/terminal/TerminalPane.tsx` - Self-contained pane: header bar, banner strip, xterm viewport, Stop wiring (302 lines)

## Decisions Made

- `retry()` connects immediately (emitting `reconnecting` attempt 0) rather than waiting out a backoff step — the user explicitly asked for a retry, and the full 5-attempt cycle still applies to subsequent failures
- Keyboard-only focus ring implemented as `has-[.xterm-helper-textarea:focus-visible]:ring-2` scoped to xterm's helper textarea; browsers apply `:focus-visible` to textareas even on pointer focus, so this is an approximation — flagged for live check in 02-05
- Stop button disabled state tracked with local `stopping` flag (not just `mutation.isPending`) so the D-14 ~5s SIGTERM grace window keeps showing `Stopping…` until the `x` frame lands; reset on mutation error

## Deviations from Plan

None - plan executed exactly as written. (One clarifying comment was added so the pane's stdin-gating comment references the hook's onData forwarding explicitly, matching the plan's key_links pattern; no behavior change.)

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 02-05 (TerminalPage) can compose `TerminalPane` with only `sessionId`/`label`/`status` props plus the three callbacks; live end-to-end verification (replay, reconnect, jiggle, clipboard) happens there
- Focus-ring pointer-click behavior and Ctrl+Shift+C DevTools collision are best-effort items to eyeball during 02-05 human verification

---
*Phase: 02-terminal-engine*
*Completed: 2026-06-10*

## Self-Check: PASSED

- web/src/components/terminal/useTerminalSocket.ts: FOUND
- web/src/components/terminal/TerminalPane.tsx: FOUND
- Commit 041a30f: FOUND
- Commit 5a592c1: FOUND
- Commit 64cd813: FOUND

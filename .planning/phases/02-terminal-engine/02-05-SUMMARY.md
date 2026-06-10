---
phase: 02-terminal-engine
plan: 05
subsystem: terminal
tags: [react, xterm, websocket, go, httptest, integration-test]

# Dependency graph
requires:
  - phase: 02-terminal-engine (plan 02-02)
    provides: "useSessions/useSpawnSession/useStopSession/useDeleteSession hooks + TermSession type, WS-capable Vite proxy"
  - phase: 02-terminal-engine (plan 02-03)
    provides: "api.SessionRoutes REST table, ws.NewHandler WS bridge, loopback/Host/Origin security"
  - phase: 02-terminal-engine (plan 02-04)
    provides: "TerminalPane (sessionId/label/status props + onClosed/onSessionExit/onNewTerminal callbacks)"
provides:
  - "web/src/pages/TerminalPage.tsx — /terminal dev surface (D-12): page header + New terminal CTA, 220px session rail (live-above-exited sort), attach-by-click via keyed TerminalPane remount (D-13)"
  - "/terminal route registered under AppLayout in web/src/App.tsx"
  - "internal/ws/integration_test.go — TestIntegration drives spawn→attach→type→detach→replay-reattach→stop→exited→delete through the SAME mux wiring as cmd/kangent/main.go; TestIntegrationEvilOriginRejected asserts the CSWSH 403 end-to-end"
  - "Human-verified phase behavior: htop TUI fidelity, resize/scrollback/copy-paste/bracketed paste, detach/reattach with clean redraw, zero-orphan Stop, security rejections, reconnect banner flow"
affects: [03-task-view]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Route-level composition only: TerminalPage owns spawn/select/close; TerminalPane stays attach-only (portable into Phase 3 TaskTabs)"
    - "key={sessionId} on TerminalPane forces a clean remount per session — switching rail rows detaches the old WS and reattaches with replay"
    - "Integration tests build the mux exactly as main.go does (api.SessionRoutes + ws.NewHandler on the same ServeMux) so route-registration drift fails CI"

key-files:
  created:
    - web/src/pages/TerminalPage.tsx
    - internal/ws/integration_test.go
  modified:
    - web/src/App.tsx

key-decisions:
  - "Spawn-select race closed by falling back to the mutation result: the pane attaches from spawn.data when the new session isn't yet in the invalidated sessions list"
  - "Integration test marker uses shell quote-splitting (mar''ker) so the PTY echo of the typed command can never satisfy the assertion — only real command output matches"
  - "Host-header 403 not re-asserted in the integration test: hostCheck is unexported middleware in cmd/kangent (covered by main_test.go table tests); the Origin CSWSH rejection is asserted end-to-end instead (plan's stated fallback)"
  - "Reattach replay asserted on the FIRST '0' frame specifically — replay must precede any live output"

patterns-established:
  - "Dev surfaces compose engine components at the route level; engine components never import router/pages"
  - "End-to-end Go tests via httptest.NewServer over the production route table, with t.Cleanup stopping every session so no bash outlives the run"

requirements-completed: [TERM-02, TERM-03, TERM-05, TERM-06, TERM-07]

# Metrics
duration: 48min
completed: 2026-06-10
---

# Phase 2 Plan 05: Terminal Dev Surface + Integration Proof Summary

**/terminal dev route composing a 220px session rail with attach-by-click TerminalPane remounts, plus a Go integration test driving spawn→echo→detach→replay-reattach→stop→delete through the production mux — all five phase success criteria human-verified**

## Performance

- **Duration:** 48 min (includes the human-verify checkpoint wait)
- **Started:** 2026-06-10T10:54:26Z
- **Completed:** 2026-06-10T11:42:04Z
- **Tasks:** 3 (2 auto + 1 human-verify checkpoint)
- **Files modified:** 3

## Accomplishments

- `TerminalPage` (153 lines) implements the full UI-SPEC layout contract: 16px/500 `Terminal` heading with the page's only inverted-contrast `New terminal` button, fixed `w-[220px]` rail with two-line rows (label 14px/500 + 12px muted status), live-above-exited newest-first sort, `bg-[#27272a]` selection matching the sidebar, skeleton loading rows, and all three pane-area states with character-exact copy (`No sessions`, `Start a shell that keeps running even when you close this tab.`, `Select a session to attach.`)
- Spawn only ever fires from click handlers (StrictMode-safe), button disabled while in flight, inline destructive error text on failure; Close delegates to `useDeleteSession` and clears selection; session exit invalidates the sessions query so rail status flips without waiting for the 5s poll
- `TestIntegration` machine-verifies the phase's server-side criteria in one flow through the real route table: 201 spawn, binary `0`-frame echo round-trip, detach-then-list-still-`running` (TERM-05), first-frame-on-reattach contains prior output (ring replay), 202 stop, bounded poll to `exited`, 204 delete, empty list — finishing well under the 30s budget
- `TestIntegrationEvilOriginRejected` proves the CSWSH rejection end-to-end: a WS upgrade with `Origin: http://evil.example` gets HTTP 403, never 101
- Human verification (Task 3) approved against all five phase criteria: htop renders and resizes correctly, scrollback/copy/bracketed-paste work, tab-close detach + reattach redraws cleanly with `done-while-away` output preserved, Stop kills `sleep 300 &` background jobs with zero orphans while exited scrollback stays copyable, `--addr 0.0.0.0` refuses to start, evil Host/Origin both 403, and the reconnect banner flow survives a server kill/restart

## Task Commits

Each task was committed atomically:

1. **Task 1: TerminalPage dev route + App.tsx wiring (D-12/D-13)** - `f08b864` (feat)
2. **Task 2: End-to-end Go integration test through the real mux** - `e520d73` (test)
3. **Task 3: Human verification checkpoint** - no commit (verification only; user approved)

## Files Created/Modified

- `web/src/pages/TerminalPage.tsx` - Dev route: header + CTA, session rail, keyed TerminalPane composition (153 lines)
- `web/src/App.tsx` - `/terminal` route added inside the existing AppLayout route block
- `internal/ws/integration_test.go` - End-to-end lifecycle + Origin-rejection tests over the production mux shape (224 lines)

## Decisions Made

- Freshly spawned sessions attach immediately by falling back to the spawn mutation result while the list invalidation is still in flight — no flash of "Select a session to attach." after clicking New terminal
- Integration test marker written as `echo integration-mar''ker` so the terminal's echo of the typed command never matches `integration-marker`; only genuine PTY output satisfies `collectUntil`
- Host-header rejection is asserted in `cmd/kangent`'s own table tests (hostCheck is unexported); the integration test covers the Origin case end-to-end, per the plan's explicit fallback
- Replay correctness asserted on the first binary frame after re-dial, enforcing the replay-before-live-stream ordering contract

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- During the human-verify checkpoint, the executor's own verification server instance was still holding port 7333, so the user's `./bin/kangent` failed with "address already in use". The orchestrator killed the stray instances and verification proceeded. Operational hiccup only — no code change required.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 2 complete: TERM-02/03/05/06/07 all demonstrably true on the running app and enforced by `go test ./...`
- Phase 3 (task view) can promote `TerminalPane` into TaskTabs as-is — it has zero route coupling; TerminalPage's rail/selection logic is the reference composition
- The integration test pins the production route shape; any Phase 3 route refactor that drifts from main.go's wiring will fail it

---
*Phase: 02-terminal-engine*
*Completed: 2026-06-10*

## Self-Check: PASSED

- web/src/pages/TerminalPage.tsx: FOUND
- internal/ws/integration_test.go: FOUND
- web/src/App.tsx (`path="/terminal"`): FOUND
- Commit f08b864: FOUND
- Commit e520d73: FOUND

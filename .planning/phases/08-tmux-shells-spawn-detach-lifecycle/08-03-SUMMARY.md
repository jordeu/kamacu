---
phase: 08-tmux-shells-spawn-detach-lifecycle
plan: 03
subsystem: session
tags: [go, tmux, pty, creack-pty, lifecycle, kill-session, has-session]

# Dependency graph
requires:
  - phase: 08-tmux-shells-spawn-detach-lifecycle (plan 01)
    provides: internal/tmux Client (NewSessionArgs, HasSession, KillSession, KillServer) on the dedicated -L socket
provides:
  - SpawnOpts.TmuxName + Manager.SetTmuxClient + tmux spawn branch (LookPath-before-PTY, ErrTmuxNotFound sentinel)
  - Per-session killer strategy assigned at Spawn — Stop on tmux tabs runs kill-session first, signals as fallback
  - waitExit has-session probe discriminating inner-exit vs manual detach; detachedAlive recorded for Phase 9
  - TMUX-07 regression guard + real-tmux lifecycle integration tests on per-test sockets
affects: [08-04, phase-09 resume reconcile, api session handlers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-session lifecycle strategy: killer func() error assigned ONCE in Spawn; nil = byte-identical pre-Phase-8 signal path (never kind-branching at stop time)"
    - "has-session is the only exit-vs-detach discriminator (attach client exits 0 in all cases); probe errors fall through to honest exited"

key-files:
  created:
    - internal/session/tmux_lifecycle_test.go
  modified:
    - internal/session/manager.go
    - internal/session/session.go

key-decisions:
  - "Stop killer-first: kill-session ends inner shell + tmux session atomically; on killer error or undead client, fall through to SIGTERM/grace/SIGKILL so the attach client dies at minimum"
  - "Manual detach (has-session alive after client exit) recorded as detachedAlive and shown as honest exited in Phase 8 — no auto-reattach (Open Q1)"
  - "Info() wire shape untouched — no tmux marker; frontend keeps seeing kind:bash (D-77)"
  - "Shell==\"tmux\" rejected in the plain-shell arm so a raw settings value can never exec tmux on the user's default socket"

patterns-established:
  - "tmux integration tests: per-test ktest-s-* sockets, KillServer cleanup registered before spawn (LIFO), skip when tmux off PATH"
  - "Readiness via poll-for-any-snapshot-bytes before WriteInput, never blind sleeps"

requirements-completed: [TMUX-03, TMUX-04, TMUX-06, TMUX-07]

# Metrics
duration: 9min
completed: 2026-06-12
---

# Phase 8 Plan 03: tmux Session Lifecycle Summary

**tmux threaded through the session package as a spawn-time lifecycle property: SpawnOpts.TmuxName spawns `tmux new-session -A` under the existing PTY pipeline, Stop is killer-first (kill-session, signal fallback), and waitExit discriminates exit-vs-detach via has-session — with bash/agent stop paths proven byte-identical**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-12T22:05:31Z
- **Completed:** 2026-06-12T22:14:30Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- TMUX-04: Stop on a tmux-backed session removes the tmux session itself — `has-session -t =name` returns dead after Stop, never the "Stop doesn't stop" trap where only the attach client dies
- TMUX-06: inner-shell exit flows through the unchanged waitExit→markExited path; manual detach (client exits, has-session alive) is recorded as `detachedAlive` for Phase 9's resume reconcile while showing the honest exited state
- TMUX-07: bash and agent sessions get a nil killer — their Stop instructions are identical to pre-Phase-8, proven both by explicit nil-killer assertions and by the entire pre-existing test suite passing with zero modifications
- TMUX-03: satisfied by adding nothing — Detach never touches the PTY (TERM-05) and the once-per-attach SIGWINCH jiggle already repaints tmux on reattach; transport diff for this plan is zero
- D-84: `ErrTmuxNotFound` fires before any PTY allocation when tmux is off PATH; no session registered, never a silent bash fallback

## Task Commits

Each task was committed atomically:

1. **Task 1: SpawnOpts.TmuxName, SetTmuxClient, and the tmux spawn branch** - `71dcd8c` (feat)
2. **Task 2: Killer-first Stop and has-session discrimination in waitExit** - `2f78dfa` (feat)
3. **Task 3: TMUX-07 regression guard + tmux lifecycle integration tests** - `dc7448d` (test)

_Task 3 was tdd="true"; since Tasks 1–2 had already landed the implementation, the test commit serves as the GREEN verification + permanent regression guard (no separate RED commit — behavior pre-existed by plan ordering)._

## Files Created/Modified

- `internal/session/manager.go` - ErrTmuxNotFound sentinel, SpawnOpts.TmuxName, Manager.tmuxClient + SetTmuxClient (SetAgentConfig pattern), tmux spawn arm (LookPath-before-PTY, explicit env allow-list scrubbing TMUX/TMUX_PANE), Shell=="tmux" guard, killer/tmuxClient assignment after Session construction
- `internal/session/session.go` - Session fields tmuxName/tmuxClient/killer (immutable) + detachedAlive (mutex-guarded); TmuxName()/DetachedAlive() accessors; killer-first Stop inside stopOnce.Do; has-session probe in waitExit before markExited; Info() untouched
- `internal/session/tmux_lifecycle_test.go` - nil-killer regression guard (bash default/settings-shell/agent, runs without tmux), kill/inner-exit/detach-alive lifecycle tests against real tmux on per-test sockets, tmux-off-PATH D-84 case

## Decisions Made

- Reworded a manager.go comment that contained the literal token `isTmux` (acceptance criterion requires zero matches in the package — the locked no-kind-branching decision is enforceable by grep)
- Task 3's TDD RED phase collapsed into GREEN: implementation legitimately preceded tests per the plan's own task ordering (Tasks 1–2 are `type="auto"` implementation tasks)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. All five integration behaviors passed against real tmux on first run; full `go test ./... -count=1` green; production `kangent` socket reports no server after tests (only inert leftover socket files in /tmp/tmux-*, same pattern as plan 08-01's tests).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 08-04 can wire the HTTP handler: mint `kangent-<task>-<n>` names, call SetTmuxClient at startup, map ErrTmuxNotFound to the D-84 error copy
- Phase 9 resume reconcile has its seam: `DetachedAlive()` + `TmuxName()` accessors and the identity-only tmux_sessions table from 08-02
- Wire shape unchanged (no tmux field in Info JSON) — frontend work in this milestone stays untouched by this plan

---
*Phase: 08-tmux-shells-spawn-detach-lifecycle*
*Completed: 2026-06-12*

## Self-Check: PASSED

All claimed files exist on disk; all three task commits (71dcd8c, 2f78dfa, dc7448d) present in git history.

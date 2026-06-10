---
phase: 02-terminal-engine
plan: 01
subsystem: terminal
tags: [go, pty, creack-pty, circbuf, process-groups, tdd]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: Go module (kangent), table-driven test conventions in internal/api
provides:
  - internal/session package — Manager + Session owning bash PTY lifetimes independent of any connection
  - Public surface for plan 02-03 (WS/REST): NewManager, Spawn, Get, List, Remove, Attach, Detach, WriteInput, Resize, Snapshot, Stop, Done, Info, ErrStillRunning
  - Replay-first attach atomicity (ring snapshot queued as first item in one critical section)
  - Session-wide teardown with zero orphaned subprocesses (TERM-06 proven by test)
  - Resize with same-size SIGWINCH jiggle (once-per-attach contract documented for WS layer)
affects: [02-02, 02-03, 02-04, 02-05, phase-04-claude-sessions]

# Tech tracking
tech-stack:
  added: [github.com/creack/pty v1.1.24, github.com/armon/circbuf, github.com/google/uuid (promoted to direct)]
  patterns: [pump goroutine fan-out with non-blocking per-conn queues, cmd.Wait() as single authoritative exit event, /proc session scan for full-tree teardown, TDD red-green per task]

key-files:
  created:
    - internal/session/session.go
    - internal/session/manager.go
    - internal/session/session_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Stop signals every process group in the shell's SESSION (via /proc scan), not just the leader's pgroup — interactive bash job control puts background jobs in their own pgroups"
  - "Exit notification mechanism: Done() <-chan struct{} + Info().ExitCode after done; attached queues close in markExited just before Done fires"
  - "Added ErrNotFound to Manager.Remove (additive to the interface contract) so the REST layer can distinguish 404 from 409"
  - "List ordering tie-break: monotonic spawn counter desc when CreatedAt is equal"

patterns-established:
  - "Pump fan-out: every PTY read goes ring-first then non-blocking send to per-conn buffered queues; slow consumers are dropped, never block the pump"
  - "Pitfall 5 discipline: PTY read errors (EIO) are end-of-stream only; cmd.Wait() goroutine owns exit status; ptmx closed AFTER Wait"
  - "Phase 4 seam: WS layer must consume only Attach/Snapshot — the ring is never reached into directly"

requirements-completed: [TERM-02, TERM-05, TERM-06]

# Metrics
duration: 12min
completed: 2026-06-10
---

# Phase 2 Plan 01: Session Engine Summary

**Server-side bash PTY session engine with 1 MiB ring-buffer replay, atomic attach/detach, and session-wide SIGTERM→5s→SIGKILL teardown proven to leave zero orphaned subprocesses**

## Performance

- **Duration:** 12 min
- **Started:** 2026-06-10T10:17:26Z
- **Completed:** 2026-06-10T10:29:43Z
- **Tasks:** 2 (both TDD: red + green commits each)
- **Files modified:** 5

## Accomplishments

- `internal/session` package compiles standalone with zero api/ws/net-http imports — PTYs outlive sockets by construction (TERM-05)
- Replay-first attach: ring snapshot queued as the connection's first item inside the same critical section that subscribes the queue — no gap or duplication versus the live stream
- Zero-descendants teardown proven by test: `sleep 300 &` dies with the session, trap-protected trees die via SIGKILL escalation (exit code 137 = 128+SIGKILL asserted)
- Resize jiggle (rows−1 → rows) implemented with the once-per-attach forceRedraw contract documented for the WS handler (plan 02-03)
- Full public surface matches the plan's interfaces block exactly; 13 tests pass under `-race`

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: Session core (spawn, pump, ring, attach, exit watcher)**
   - RED: `b2fe234` (test) — failing tests for spawn/pump/attach/detach/exit/remove
   - GREEN: `61d2690` (feat) — Manager + Session implementation, deps pinned
2. **Task 2: Stop teardown (D-14), Resize with jiggle, zero-orphans proof**
   - RED: `5b46643` (test) — failing tests for Stop/escalation/Resize
   - GREEN: `d146545` (feat) — session-wide Stop, Resize + jiggle

## Files Created/Modified

- `internal/session/session.go` (359 lines) — Session: pump goroutine, 1 MiB circbuf ring, Attach/Detach, WriteInput, Resize+jiggle, Snapshot, exit watcher (128+signal convention), Stop, sessionPGIDs /proc scan
- `internal/session/manager.go` (150 lines) — Manager: Spawn ($SHELL fallback /bin/bash, cwd=home, explicit env, uuid IDs, monotonic "bash #N" labels), Get, List (newest first), Remove (exited only)
- `internal/session/session_test.go` (479 lines) — 13 table-driven tests incl. the TERM-06 zero-descendants test; polling helpers, no fixed sleeps over 100ms granularity
- `go.mod` / `go.sum` — creack/pty v1.1.24, armon/circbuf, google/uuid pinned (coder/websocket dropped by tidy as the plan anticipated; plan 02-03 re-adds it)

## Decisions Made

- **Session-wide teardown instead of leader-pgroup-only** — see Deviations; this is the load-bearing correctness decision of the plan
- **Exit notification = Done() + Info().ExitCode** (the plan's recommended option): markExited closes attached queues, then the watcher closes ptmx and Done; the WS layer sends the 'x' frame when Done fires
- **ErrNotFound added** alongside ErrStillRunning so plan 02-03's REST layer can map missing → 404 vs running → 409 without exploration
- **Late attach after exit** returns the replay then a closed channel, so a late attacher renders final output (per plan spec)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Leader-pgroup-only Stop would orphan background children — replaced with session-wide kill**
- **Found during:** Task 2 (pre-implementation verification of the planned mechanism)
- **Issue:** The plan's Stop (`syscall.Kill(-leaderPGID, ...)` only) assumed background children share the leader's process group. Empirically verified false: bash spawned on a PTY is interactive, enables job control, and runs every pipeline — including `sleep 300 &` — in its OWN process group (observed: sleep pgid ≠ bash pgid, same session id). The planned code would have silently violated the must-have truth "Stop terminates the entire process group (including background children)" and TERM-06. The plan's own `pgrep -g` wait step could never have observed the sleep.
- **Fix:** Added `sessionPGIDs(sid)` (/proc/[pid]/stat scan for processes whose session == leader pid) and `signalSession(sig)` which signals every group in the session plus the leader group. Stop keeps the exact plan shape (SIGTERM → grace → SIGKILL → wait done) plus a bounded post-exit sweep. Test waits on session groups (≥2) instead of `pgrep -g`, and asserts both ESRCH on the leader pgroup AND an empty session.
- **Files modified:** internal/session/session.go, internal/session/session_test.go
- **Verification:** TestStopZeroDescendants and TestStopEscalatesToSigkill pass (incl. under -race); exit code 137 proves the SIGKILL path
- **Committed in:** d146545 (Task 2 GREEN; groundwork in 61d2690)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential for TERM-06 correctness — without it Stop would orphan background jobs. Public surface unchanged; no scope creep.

## Issues Encountered

None — all tests passed on first GREEN run for both tasks.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 02-03 (WS/REST) can build against the exact exported surface with no exploration: Attach/Detach/WriteInput/Resize/Snapshot/Stop/Done/Info + Manager CRUD
- Resize contract for the WS handler documented in the method comment: forceRedraw=true exactly once, on the first resize frame after each attach
- coder/websocket v1.8.14 must be re-added by plan 02-03's `go mod tidy` (its import re-pins it); 02-03 declares go.mod/go.sum in files_modified
- Phase 4 seam intact: ring is opaque behind Snapshot()/Attach()

---
*Phase: 02-terminal-engine*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All 3 created files exist on disk
- All 4 task commits (b2fe234, 61d2690, 5b46643, d146545) present in git log
- `go test ./... -timeout 120s` passes repo-wide; `go vet ./internal/session/` clean

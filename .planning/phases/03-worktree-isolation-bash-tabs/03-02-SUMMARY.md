---
phase: 03-worktree-isolation-bash-tabs
plan: 02
subsystem: terminal
tags: [go, pty, sessions, worktree, creack-pty]

# Dependency graph
requires:
  - phase: 02-terminal-engine
    provides: session.Manager (Spawn/Stop/List/Attach), Session PTY lifecycle, Stop's process-tree teardown
provides:
  - SpawnOpts{Cwd, TaskID} — sessions can start in a worktree cwd and belong to a task
  - Cwd validated (os.Stat, IsDir) BEFORE PTY allocation; failed spawns register nothing
  - Per-task "Bash N" labels (monotonic, never reused) alongside the global "bash #N" dev counter
  - Info.TaskID (json taskId,omitempty) for REST filtering
  - ListByTask(taskID) — newest first, same comparator as List (shared listWhere helper)
  - StopAllForTask(taskID) — concurrent Stop of a task's running sessions, blocks until all exited
affects: [03-03 REST task wiring, 03-06 e2e integration test, phase-04 claude sessions]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Options-struct Spawn API (SpawnOpts) so future fields never break call sites again"
    - "Shared listWhere(filter) helper — single sort comparator for List and ListByTask"
    - "Concurrent Stop fan-out with sync.WaitGroup collapses N grace windows to one"

key-files:
  created: []
  modified:
    - internal/session/manager.go
    - internal/session/session.go
    - internal/session/session_test.go
    - internal/api/sessions.go
    - internal/ws/handler_test.go
    - internal/ws/integration_test.go

key-decisions:
  - "Separate global seq counter added to Manager so List ordering tiebreak stays correct across both label families (per-task counters would otherwise collide on seq)"
  - "Cwd validation also rejects a path that exists but is a file (!fi.IsDir()), same error shape as nonexistent"
  - "ListByTask(0) deliberately returns only unscoped dev sessions — taskID 0 is the dev sentinel, not a wildcard"

patterns-established:
  - "SpawnOpts options struct: extend session spawning by adding fields, never by changing the signature"
  - "listWhere(keep func(*Session) bool): all session list views share one snapshot+sort path"

requirements-completed: [TERM-04]

# Metrics
duration: 11min
completed: 2026-06-10
---

# Phase 3 Plan 02: Session Manager Task/Cwd Extension Summary

**Spawn(SpawnOpts{Cwd, TaskID}) with pre-PTY cwd validation, per-task monotonic "Bash N" labels, ListByTask filtering, and concurrent StopAllForTask — the engine half of TERM-04**

## Performance

- **Duration:** 11 min
- **Started:** 2026-06-10T14:18:03Z
- **Completed:** 2026-06-10T14:28:43Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 6

## Accomplishments

- Sessions can now run in a worktree cwd and carry a task identity; `Spawn(SpawnOpts{})` is byte-identical to Phase 2 behavior (home cwd, global "bash #N" labels) — the /terminal dev route is unchanged
- Invalid cwd (nonexistent or a file) fails cleanly with the path in the error BEFORE any PTY is allocated, and registers no session — a deleted worktree produces the clean "couldn't start a session" path
- Task-scoped labels follow the UI-SPEC contract: "Bash 1", "Bash 2"... per task, monotonic and never reused (stop "Bash 1", next spawn is "Bash 3"); counters live in a `taskCounters map[int64]int`
- `StopAllForTask` stops exactly the task's running sessions concurrently (one goroutine each + WaitGroup), collapsing the worst case to a single 5s SIGTERM grace window for the inline DELETE /worktree handler in 03-03
- All 20 session tests green including the full Phase 2 regression net (detach/reattach replay, process-tree stop, exit notification); `go build ./...`, vet, and api + ws test suites green

## Task Commits

Each TDD task produced RED + GREEN commits:

1. **Task 1: SpawnOpts, cwd validation, per-task labels, ListByTask**
   - RED: `18db0fb` (test) — failing tests for SpawnOpts, labels, ListByTask
   - GREEN: `65cf07e` (feat) — implementation, all tests green
2. **Task 2: StopAllForTask + compile-fix the REST caller**
   - RED: `91acbc6` (test) — failing tests for StopAllForTask
   - GREEN: `a8bf064` (feat) — StopAllForTask + Spawn call-site updates

No REFACTOR commits — implementations came out clean in GREEN.

## Files Created/Modified

- `internal/session/manager.go` — SpawnOpts, cwd validation before pty.StartWithSize, taskCounters + seq counters, listWhere/ListByTask, StopAllForTask
- `internal/session/session.go` — Session.taskID field; Info.TaskID (`json:"taskId,omitempty"`)
- `internal/session/session_test.go` — spawnForTestOpts helper, 5 new tests (labels, cwd, invalid cwd, ListByTask, StopAllForTask ×2)
- `internal/api/sessions.go` — create handler calls `Spawn(session.SpawnOpts{})` (task_id body parsing is plan 03-03)
- `internal/ws/handler_test.go`, `internal/ws/integration_test.go` — mechanical Spawn signature fix (deviation, see below)

## Decisions Made

- Added a separate global `seq` counter to Manager: previously `seq` doubled as the label counter, but per-task counters would produce colliding seq values and corrupt the List ordering tiebreak
- Cwd validation rejects existing-but-not-a-directory paths with the same error as nonexistent paths (tests cover both)
- `ListByTask(0)` returns only unscoped dev sessions (0 is the dev sentinel, not a wildcard) — matches the plan's behavior spec

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated internal/ws test helpers to the new Spawn signature**
- **Found during:** Task 2 (compile-fix sweep)
- **Issue:** `internal/ws/handler_test.go:35` and `internal/ws/integration_test.go:208` call `mgr.Spawn()`; the signature change breaks `go test ./...` compilation for the ws package, and no Phase 3 plan owns these files
- **Fix:** Mechanical one-token change to `mgr.Spawn(session.SpawnOpts{})` in both files
- **Files modified:** internal/ws/handler_test.go, internal/ws/integration_test.go
- **Verification:** `go test ./internal/ws/ -count=1` green (49.6s, full PTY/WS suite)
- **Committed in:** a8bf064 (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The plan's "no file outside internal/session/ + internal/api/sessions.go" verification line is technically violated by two test-only, mechanical call-site fixes that keep the whole tree's tests compiling. No behavior change, no scope creep.

## Issues Encountered

None.

## Known Stubs

None — all code paths are fully wired; no placeholders.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 03-03 can implement `POST /api/sessions {task_id}`, `GET /api/sessions?task_id=`, and the `DELETE /api/tasks/{id}/worktree {stop_sessions}` gate purely against SpawnOpts / ListByTask / StopAllForTask — the manager needs no further changes
- /terminal dev route behavior byte-identical (verified by untouched Phase 2 tests + api suite)

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

All 6 modified files and the SUMMARY exist on disk; all 4 task commits (18db0fb, 65cf07e, 91acbc6, a8bf064) present in git log.

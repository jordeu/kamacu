---
phase: 08-tmux-shells-spawn-detach-lifecycle
plan: 01
subsystem: session
tags: [tmux, go, os-exec, sqlite, goose, leaf-package]

# Dependency graph
requires:
  - phase: 03-worktrees
    provides: tasks table (FK target for tmux_sessions.task_id)
provides:
  - internal/tmux leaf package (DefaultSocket, Config, WriteConfig, Client with BaseArgs/NewSessionArgs/HasSession/KillSession/KillServer)
  - tmux_sessions identity table (migration 00005) for name minting and Phase 9 reconcile
  - encoded tmux 3.4 truth table: =name exact match, exit-1 dead vs exec-error broken, idempotent kill
affects: [08-03 session lifecycle, 08-04 API wiring, phase-09 reconcile]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Every tmux invocation carries -L <socket> -f <config> via Client.BaseArgs — config application deterministic regardless of which call starts the server"
    - "Session targets always '='+name exact match (tmux 3.4 prefix-matches bare names)"
    - "5s context.WithTimeout on every tmux exec so a hung binary never blocks Stop()/cleanup"
    - "HasSession (bool, error): exit-1 = (false,nil) dead; exec failure = (false,err) — callers never misread 'tmux missing' as 'session dead'"

key-files:
  created:
    - internal/tmux/tmux.go
    - internal/tmux/tmux_test.go
    - internal/store/migrations/00005_tmux_sessions.sql
  modified: []

key-decisions:
  - "Generated -f config file (status off, mouse on, history-limit 50000) over post-create set-option sequence — the latter raced empirically"
  - "tmux_sessions is identity-only (no status column); tmux itself is the status authority via has-session"
  - "KillSession/KillServer treat exit 1 as idempotent success (already dead / no server)"

patterns-established:
  - "internal/tmux is a leaf package: stdlib imports only, never session/api/store"
  - "Test harness: per-test ktest-<pid>-<TestName> sockets, KillServer in t.Cleanup registered before any session, -d for test-created sessions (no tty in go test)"

requirements-completed: [TMUX-02]

# Metrics
duration: 6min
completed: 2026-06-12
---

# Phase 8 Plan 01: internal/tmux Leaf Package + tmux_sessions Migration Summary

**Socket-isolated tmux Client (new-session -A / has-session / kill-session with =name exact match, 5s exec timeouts, idempotent kill) plus the identity-only tmux_sessions table — all later tmux work calls through this one package**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-06-12T21:54:35Z
- **Completed:** 2026-06-12T22:00:30Z
- **Tasks:** 3
- **Files modified:** 3 (all created)

## Accomplishments

- `internal/tmux` leaf package: `DefaultSocket="kangent"`, `Config` (status off / mouse on / history-limit 50000 per D-79/D-80), `WriteConfig`, and `Client{Socket,ConfPath}` whose every invocation injects `-L` and `-f` — the user's default tmux server/config can never be touched
- Exit-code truth table from 08-RESEARCH encoded once: `HasSession` returns `(false,nil)` only on a clean exit 1 (dead session AND dead server) and `(false,err)` on exec/binary/timeout failures; `KillSession`/`KillServer` treat exit 1 as idempotent success
- 8 tests green against real host tmux 3.4: argv equality, live/dead/dead-server probes, idempotent double-kill, PATH-scrub exec-error discrimination, and the prefix-collision guard (killing `kangent-1-1` leaves `kangent-1-10` alive; `HasSession("kangent-1")` is false with both alive)
- Migration 00005 `tmux_sessions` (task_id, n, name UNIQUE, label DEFAULT '', created_at, UNIQUE(task_id,n)) applies cleanly via the existing embedded glob — zero Go code changes

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement internal/tmux leaf package** - `e9a9f71` (feat)
2. **Task 2: Real-tmux tests for the leaf package** - `d5f0c9d` (test)
3. **Task 3: Migration 00005 — tmux_sessions identity table** - `ffdc26c` (feat)

## Files Created/Modified

- `internal/tmux/tmux.go` - Leaf package: socket/config constants, WriteConfig, Client with BaseArgs/NewSessionArgs/HasSession/KillSession/KillServer
- `internal/tmux/tmux_test.go` - Pure argv tests + real-tmux integration tests on per-test sockets
- `internal/store/migrations/00005_tmux_sessions.sql` - Identity-only tmux session table with both uniqueness constraints

## Decisions Made

- Reworded one code comment so the literal string "sh -c" doesn't appear in tmux.go (acceptance criterion greps for its absence); the implementation was always arg-array exec
- TDD task (Task 2) executed test-after by plan construction: Task 1 created the implementation first, so the RED phase was not meaningful — tests were written from the behavior spec and all passed first run

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. Parallel-agent note: `internal/settings/validate.go` was dirty in the working tree (plan 08-02's file) — left untouched; all commits staged files individually.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 08-03 (session lifecycle) can import `internal/tmux` for the spawn argv (`NewSessionArgs`, no `-d`) and the kill-session stop strategy
- 08-04 (API wiring) can mint `kangent-<task>-<n>` names from `tmux_sessions` `MAX(n)+1` — the table exists with both UNIQUE constraints as the backstop
- Phase 9 reconcile gets `HasSession` with honest dead-vs-broken discrimination and `KillServer` for the README escape hatch
- No blockers

---
*Phase: 08-tmux-shells-spawn-detach-lifecycle*
*Completed: 2026-06-12*

## Self-Check: PASSED

- All 3 created files exist on disk
- All 3 task commits (e9a9f71, d5f0c9d, ffdc26c) present in git log
- `go build ./...` and `go test ./internal/tmux/ ./internal/store/ -count=1` green

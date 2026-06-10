---
phase: 01-foundation-projects-board
plan: 01
subsystem: database
tags: [go, sqlite, modernc-sqlite, goose, migrations, net-http]

# Dependency graph
requires: []
provides:
  - Go module `kangent` (go 1.26) with modernc.org/sqlite v1.52.0 and goose v3.27.1
  - store.Open() with WAL/busy_timeout/foreign_keys/synchronous pragmas, _txlock=immediate, SetMaxOpenConns(1)
  - store.Migrate() running embedded goose migrations (dialect sqlite3) at startup
  - Initial schema: projects + tasks tables with status CHECK enum, REAL position, ON DELETE CASCADE, idx_tasks_board
  - cmd/kangent server binary: --addr/--db flags, startup migration, GET /api/healthz
affects: [01-02, 01-03, 01-07, phase-2, phase-3, phase-4]

# Tech tracking
tech-stack:
  added: [modernc.org/sqlite v1.52.0, github.com/pressly/goose/v3 v3.27.1]
  patterns:
    - "SQLite open discipline: pragma DSN + SetMaxOpenConns(1), driver name 'sqlite' not 'sqlite3'"
    - "Embedded goose migrations run at startup; schema evolution via new numbered migration files"
    - "Go 1.22+ ServeMux method patterns (GET /api/healthz)"
    - "log/slog structured logging; error + os.Exit(1) on fatal startup failures"

key-files:
  created:
    - go.mod
    - go.sum
    - internal/store/store.go
    - internal/store/migrate.go
    - internal/store/migrations/00001_init.sql
    - internal/store/store_test.go
    - cmd/kangent/main.go
  modified: []

key-decisions:
  - "Module path is `kangent` (local-only single binary), not a github.com path"
  - "goose dialect 'sqlite3' paired with modernc driver name 'sqlite' — intentionally different strings"
  - "No --open/auto-browser flag (Claude's discretion: default off, add later if wanted)"
  - "~ in --db expanded server-side via os.UserHomeDir"

patterns-established:
  - "Store package owns all DB lifecycle: Open (pragmas) + Migrate (goose embed)"
  - "Tests use filepath.Join(t.TempDir(), ...) for isolated per-test databases"
  - "TDD: failing test commit precedes implementation commit"

requirements-completed: [STOR-01]

# Metrics
duration: 4min
completed: 2026-06-10
---

# Phase 1 Plan 01: Go Backend Foundation Summary

**SQLite store with WAL/foreign_keys/MaxOpenConns(1) discipline, embedded goose migrations creating projects/tasks schema, and a runnable server binary serving /api/healthz at 127.0.0.1:7333**

## Performance

- **Duration:** 4 min
- **Started:** 2026-06-10T06:32:01Z
- **Completed:** 2026-06-10T06:36:08Z
- **Tasks:** 2 (Task 1 TDD: RED + GREEN commits)
- **Files modified:** 7

## Accomplishments

- Go module initialized (`kangent`, go 1.26 directive; Go 1.26.0 toolchain auto-downloaded via GOTOOLCHAIN=auto as predicted by research)
- `store.Open()` implements the mandatory open discipline verbatim from RESEARCH.md Pattern 2: WAL, busy_timeout(5000), foreign_keys(1), synchronous(NORMAL), _txlock=immediate, SetMaxOpenConns(1)
- Embedded goose migration creates `projects` and `tasks` tables with status CHECK enum ('todo','in_progress','in_review','done'), REAL position column, ON DELETE CASCADE, and the board index — no worktree/session columns (scope guard respected)
- 5 store tests prove: pragmas live, migration creates tables, cascade delete works, data survives close/reopen (STOR-01), CHECK constraint rejects bogus status
- `cmd/kangent` binary: `--addr` (default 127.0.0.1:7333) and `--db` (default ~/.kangent/kangent.db) flags, migrates at startup, serves `GET /api/healthz` → `{"status":"ok"}`
- Restart against the same DB confirmed idempotent ("goose: no migrations to run. current version: 1")

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): failing store tests** - `7428bb4` (test)
2. **Task 1 (GREEN): store implementation** - `32ec5f5` (feat)
3. **Task 2: server entrypoint** - `dfceb87` (feat)

_No REFACTOR commit for Task 1 — implementation is the verbatim research-verified pattern; nothing to clean up._

## Files Created/Modified

- `go.mod` / `go.sum` - Module `kangent`, go 1.26, sqlite + goose deps
- `internal/store/store.go` - `Open()` with full pragma discipline and single-writer pool
- `internal/store/migrate.go` - `Migrate()` via embedded goose FS, dialect "sqlite3"
- `internal/store/migrations/00001_init.sql` - projects/tasks schema + idx_tasks_board
- `internal/store/store_test.go` - 5 behavior tests (pragmas, migrate, cascade, restart, CHECK)
- `cmd/kangent/main.go` - flags, ~ expansion, MkdirAll, open+migrate, healthz, slog startup log

## Decisions Made

- Module path `kangent` per plan (local-only binary; imports are `kangent/internal/store`)
- No auto-open-browser flag (discretion area; default off per plan)
- `~` expansion handled in main.go via small `expandHome` helper rather than a dependency

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Restart-idempotency smoke test produced a transient "address already in use" because the first test process hadn't released the port before the second bound — a test-harness timing artifact, not a server bug. The relevant evidence (goose reporting "no migrations to run" on second start against the same DB) was captured successfully.
- `go test ./...` also discovers a Go package inside `web/node_modules/` created concurrently by a parallel executor agent (plan 01-04+ frontend work). Out of scope for this plan; `internal/store` and `cmd/kangent` both pass.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- DB discipline and project layout (cmd/kangent, internal/store) in place — plans 01-02/01-03 can add `internal/api` handlers against this store
- `/` is intentionally unhandled; plan 01-07 wires SPA embed + fallback
- Schema extension path proven: later phases add numbered goose migrations

## Self-Check: PASSED

- All 7 created files verified on disk
- Commits 7428bb4, 32ec5f5, dfceb87 verified in git log

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

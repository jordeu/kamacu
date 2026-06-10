---
phase: 01-foundation-projects-board
plan: 02
subsystem: api
tags: [go, net-http, servemux, sqlite, rest, fractional-ordering, tdd]

# Dependency graph
requires:
  - phase: 01-foundation-projects-board (plan 01-01)
    provides: store.Open/store.Migrate with pragma discipline, projects+tasks schema with CASCADE and position REAL, cmd/kangent server skeleton
provides:
  - All 10 REST endpoints of the RESEARCH.md Pattern 4 contract (projects CRUD, tasks CRUD, move)
  - validateRepoPath: ~ expansion, IsAbs, Stat, git rev-parse --git-dir via arg-array exec
  - Server-computed fractional ordering with 1e-9 renormalization guard inside one BeginTx transaction
  - writeJSON/writeError helpers establishing the {"error": msg} contract
  - api.Routes(mux, db) wired in cmd/kangent/main.go
affects: [01-03, 01-04, 01-05, 01-06, 01-07, phase-3]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Handler convention: per-resource struct {db *sql.DB} + r.PathValue + writeError; 400 bad id, 404 missing row"
    - "INSERT/UPDATE ... RETURNING to read back full rows in one round trip"
    - "Pointer fields (*string) in PATCH bodies to distinguish absent from empty"
    - "Empty list responses encode as [] (initialized slices), never null"
    - "Move endpoint: client sends {status, after_id} intent only; server owns all float math in one tx"
    - "TDD: every plan task is a test commit followed by a feat commit"

key-files:
  created:
    - internal/api/respond.go
    - internal/api/routes.go
    - internal/api/projects.go
    - internal/api/projects_test.go
    - internal/api/tasks.go
    - internal/api/tasks_test.go
  modified:
    - cmd/kangent/main.go

key-decisions:
  - "Duplicate repo_path detected via SELECT pre-check (plan-sanctioned at single-user scale), not constraint-error parsing"
  - "after_id == moving task id rejected as invalid after_id (client order math can never produce it)"
  - "Project DELETE returns 404 for missing id, consistent with the plan's general missing-row rule"

patterns-established:
  - "API error contract: every non-2xx body is {\"error\": \"human-readable message\"} with exact UI-SPEC strings"
  - "Ordering invariant: positions strictly increase within (project_id, status); renormalize to 1.0,2.0,... on midpoint exhaustion"

requirements-completed: [PROJ-01, PROJ-03, TASK-01, TASK-03, TASK-04]

# Metrics
duration: 8min
completed: 2026-06-10
---

# Phase 1 Plan 02: REST API Summary

**All 10 JSON endpoints (projects CRUD with git rev-parse path validation, tasks CRUD, move) with server-computed fractional ordering proven stable under a 200-insert renormalization stress test**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-10T06:45:46Z
- **Completed:** 2026-06-10T06:54:13Z
- **Tasks:** 3 (each TDD: RED + GREEN commits)
- **Files modified:** 7

## Accomplishments

- Projects CRUD: POST validates repo paths server-side (`~` expansion → `filepath.IsAbs` → `os.Stat` dir check → `git -C <abs> rev-parse --git-dir` via arg array), defaults name to `filepath.Base`, returns 409 `this repository is already added` on duplicates; DELETE cascades tasks while never touching the repo on disk (no `os.Remove*` anywhere in the package)
- Tasks CRUD: create always lands at the top of To Do (`COALESCE(MIN(position), 2.0) - 1.0`, D-09), deep-link GET, pointer-field PATCH that edits title/description only (status/position rejected by omission — moves go through /move), hard DELETE (D-11)
- Move endpoint: single `BeginTx` write transaction; `after_id: null` = top of column (mover excluded from the MIN scan), midpoint between after/next otherwise, `after + 1.0` at bottom; cross-column/cross-project/self anchors rejected with 400 `invalid after_id`; 1e-9 gap guard renumbers the whole column to 1.0, 2.0, 3.0... in the same transaction and recomputes
- 20 Go tests cover every behavior in the plan, including the 202-task strict-ordering stress test and a close/reopen persistence test (TASK-03)
- Live spot check: `curl -X POST /api/projects -d '{"repo_path":"$PWD"}'` → 201 with name "kangent"; `/api/nope` → 404 (mux default, SPA fallback arrives in 01-07)

## Task Commits

Each task was committed atomically (TDD: test then feat):

1. **Task 1 (RED): projects tests** - `ee19c71` (test)
2. **Task 1 (GREEN): respond/routes/projects + main wiring** - `50836a2` (feat)
3. **Task 2 (RED): tasks CRUD tests** - `b85beb4` (test)
4. **Task 2 (GREEN): tasks.go handlers, stubs removed** - `e336e34` (feat)
5. **Task 3 (RED): move ordering tests** - `49a1477` (test)
6. **Task 3 (GREEN): move endpoint with renormalization** - `ce3b69c` (feat)

_No REFACTOR commits — implementations followed the research-verified patterns directly; nothing to clean up._

## Files Created/Modified

- `internal/api/respond.go` - writeJSON/writeError helpers ({"error": msg} contract)
- `internal/api/routes.go` - Routes(mux, db) registering all 10 Go 1.22 method-pattern endpoints
- `internal/api/projects.go` - Project struct, validateRepoPath, list/create/update/delete handlers
- `internal/api/projects_test.go` - newTestServer/gitRepo/doJSON helpers + 7 project behaviors
- `internal/api/tasks.go` - Task struct, tasks CRUD + move with fractional positions and renumberColumn
- `internal/api/tasks_test.go` - 12 task/move behaviors incl. stress and reopen-persistence tests
- `cmd/kangent/main.go` - api.Routes(mux, db) wired after migrate; healthz kept

## Decisions Made

- Duplicate repo_path via SELECT pre-check rather than parsing the UNIQUE constraint error — plan offered both; pre-check is simpler and race-free at single-user scale with MaxOpenConns(1)
- `after_id` equal to the moving task's own id is rejected as `invalid after_id` (a correct client can never produce it; accepting it would compute a self-relative position)
- Temporary 501 task stubs lived in routes.go during Task 1 (per plan's sequential-session note) and were removed in Task 2

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The full Pattern 4 API contract is live and tested — frontend plans 01-04..01-06 can code hooks against it verbatim (01-03's contract layer already matches)
- Error strings match UI-SPEC exactly (`path must be absolute`, `not a directory: {path}`, `not a git repository: {path}`, `this repository is already added`, `title is required`, `invalid status`, `invalid after_id`)
- `/` remains unhandled by design; plan 01-07 adds SPA embed + fallback
- Handler conventions (resource struct + PathValue + writeError) are the template for Phase 3's worktree endpoints

## Self-Check: PASSED

- All 7 created/modified files verified on disk
- Commits ee19c71, 50836a2, b85beb4, e336e34, 49a1477, ce3b69c verified in git log

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

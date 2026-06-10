---
phase: 03-worktree-isolation-bash-tabs
plan: 03
subsystem: api
tags: [go, rest, sqlite, goose, worktree, sessions, tdd]

# Dependency graph
requires:
  - phase: 03-worktree-isolation-bash-tabs (plan 03-01)
    provides: internal/worktree Service (Slug, PathFor, ResolveBase, Create, DirtyCount, Remove, EnsureSubmodules)
  - phase: 03-worktree-isolation-bash-tabs (plan 03-02)
    provides: session.SpawnOpts{Cwd, TaskID}, ListByTask, StopAllForTask, Info.TaskID
provides:
  - Migration 00002: tasks gains nullable branch / worktree_path / worktree_error columns
  - Task JSON carries the three worktree fields on every endpoint (create/get/list/move)
  - Task creation provisions branch+worktree synchronously (30s cap); failures recorded as worktree_error on a 201 (D-25)
  - POST/GET/DELETE /api/tasks/{id}/worktree with server-enforced D-32/D-33 gates, D-34 branch survival
  - POST /api/sessions {task_id} spawns in the task's worktree; GET /api/sessions?task_id=N filters
affects: [03-04 bash tabs UI, 03-05 worktree meta/cleanup UI, 03-06 e2e integration, phase-04 claude sessions]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "provisionWorktree helper shared by task-create and the POST /worktree retry path; records outcome via simple UPDATEs after git ops (never a tx across git, Pitfall 9)"
    - "Cleanup gates re-checked AT DELETE TIME server-side (Pitfall 8); StopAllForTask ordered strictly before Remove (Pitfall 4)"
    - "Scalar-subquery JOIN (SELECT taskColumns, (SELECT repo_path ...)) keeps scanTask reusable for task+repo loads"

key-files:
  created:
    - internal/store/migrations/00002_worktrees.sql
    - internal/api/worktrees.go
    - internal/api/worktrees_test.go
  modified:
    - internal/api/tasks.go
    - internal/api/tasks_test.go
    - internal/api/sessions.go
    - internal/api/sessions_test.go
    - internal/api/routes.go
    - internal/api/projects_test.go
    - cmd/kangent/main.go
    - internal/ws/integration_test.go

key-decisions:
  - "provisionWorktree performs the outcome UPDATEs itself (success clears worktree_error) so the create() and Retry paths share one persistence flow"
  - "GET/DELETE treat a manually deleted worktree dir as clean (dirty=0): Remove self-heals bookkeeping on git 2.43, so a missing tree never blocks cleanup"
  - "Behavioral cwd proof in tests: echo \"mark:$PWD\" — PTY input echo only ever contains the literal $PWD, so a marker+path match cannot be satisfied by echo"

patterns-established:
  - "Worktree gate copy is exact API contract: 409 {\"error\":\"sessions running\"} and 409 {\"error\":\"worktree has uncommitted changes\"}"
  - "Optional JSON request bodies decode with io.EOF tolerated (empty body = zero values)"

requirements-completed: [GIT-01, GIT-02, GIT-03, TERM-04]

# Metrics
duration: 19min
completed: 2026-06-10
---

# Phase 3 Plan 03: Worktree & Session REST Wiring Summary

**Migration 00002 + task-create worktree provisioning (201 always, D-25) + POST/GET/DELETE /api/tasks/{id}/worktree with server-enforced stop/force gates and branch-preserving cleanup + task-scoped session spawn/filter — the whole phase is now exercisable with curl**

## Performance

- **Duration:** 19 min
- **Started:** 2026-06-10T14:36:05Z
- **Completed:** 2026-06-10T14:55:29Z
- **Tasks:** 3 (all TDD)
- **Files modified:** 11

## Accomplishments

- Task creation now provisions a real `task/<slug>-<id>` branch + worktree under `~/.kangent/worktrees/<repo>/<slug>-<id>` synchronously (30s timeout), with best-effort submodule init; any git failure lands in `worktree_error` on a still-successful 201 — idea capture is never blocked (D-25, test-pinned against an unborn-HEAD repo)
- Worktree endpoints cover the full lifecycle: Retry/lazy-create/idempotent POST (including kept-branch reuse after cleanup — Pitfall 1's reuse path proven over REST), fresh GET state ({branch, path, dirty_files via -uall, running_sessions}), and DELETE with both 409 gates re-checked at request time; every removal path's test asserts the branch survives (`git branch --list`, D-34)
- `StopAllForTask` runs strictly before `Remove` in the DELETE handler — the only real enforcement of GIT-03's refuse-while-running, since git happily deletes trees under live cwds (Pitfall 4)
- Sessions spawn into a task's worktree via `POST /api/sessions {"task_id":N}` (404 unknown task, 409 no worktree per D-30) with per-task "Bash N" labels; `?task_id=` filters the list including exited sessions (D-28); the no-body /terminal dev route is byte-identical (regression-tested)
- Whole module green: `go build ./... && go vet ./... && go test ./... -count=1` across api (45s), session, store, worktree, ws (50s) suites

## Task Commits

Each TDD task produced RED + GREEN commits (no REFACTOR commits needed):

1. **Task 1: Migration 00002 + provisioning hook**
   - RED: `ac1b3fc` (test) — failing tests for provisioning, D-25, task JSON fields
   - GREEN: `acb1f81` (feat) — migration, Task struct fields, provisionWorktree, Routes/main wiring
2. **Task 2: Worktree REST endpoints**
   - RED: `d00fd3b` (test) — failing tests for POST/GET/DELETE incl. gates and branch survival
   - GREEN: `c46d90e` (feat) — worktrees.go, WorktreeRoutes registered in main.go
3. **Task 3: Task-scoped session spawn/filter**
   - RED: `dedd5cb` (test) — failing tests for {task_id} spawn, filter, dev-route regression
   - GREEN: `461804c` (feat) — sessions.go extension, call-site updates

## Files Created/Modified

- `internal/store/migrations/00002_worktrees.sql` — three nullable task columns; Down drops them in reverse
- `internal/api/tasks.go` — Task gains Branch/WorktreePath/WorktreeError; `provisionWorktree` helper; create() resolves repo_path and always 201s
- `internal/api/worktrees.go` — POST/GET/DELETE `/api/tasks/{id}/worktree`; gates, 60s delete timeout, D-26 absent-state reset
- `internal/api/sessions.go` — optional {task_id} spawn body, `?task_id=` list filter; SessionRoutes gains *sql.DB
- `internal/api/routes.go` — Routes gains *worktree.Service
- `cmd/kangent/main.go` — worktree service rooted at ~/.kangent/worktrees; WorktreeRoutes registered
- `internal/api/{tasks,worktrees,sessions}_test.go` — 17 new tests over real git repos + real session.Manager
- `internal/api/projects_test.go`, `internal/ws/integration_test.go` — harness updates for new signatures

## Decisions Made

- `provisionWorktree` owns the outcome UPDATEs (success also sets `worktree_error = NULL`) so task-create and the POST retry path share one persistence flow — the plan's signature included db; centralizing avoided duplicated UPDATE logic in two handlers
- GET and DELETE compute dirty=0 when the worktree dir is missing on disk (manually deleted tree): `Remove` self-heals the bookkeeping on git 2.43, so a vanished dir must never block cleanup
- Worktree task/repo loads use a scalar subquery appended to `taskColumns` instead of a qualified JOIN, keeping the shared `scanTask` reusable

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated internal/ws/integration_test.go to the new SessionRoutes signature**
- **Found during:** Task 3 (SessionRoutes gained the *sql.DB parameter)
- **Issue:** `internal/ws/integration_test.go` calls `api.SessionRoutes(mux, mgr)`; the signature change breaks `go test ./...` compilation for the ws package, and no Phase 3 plan owns that file
- **Fix:** Test harness now opens a migrated temp SQLite DB and passes it — mirroring cmd/kangent/main.go's production wiring, which is that test's stated purpose
- **Files modified:** internal/ws/integration_test.go
- **Verification:** `go test ./internal/ws/ -count=1` green (49.7s, full PTY/WS suite)
- **Committed in:** 461804c (Task 3 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Mechanical test-harness fix keeping the whole tree compiling; no behavior change, no scope creep.

## Issues Encountered

- `web/dist/index.html` carries pre-existing uncommitted local modifications (a previous local `make build` overwrote the committed placeholder with real Vite output). Present before this plan started; out of scope — logged to `deferred-items.md`, left uncommitted per the Phase 01 "real build output never committed" decision.

## Known Stubs

None — all code paths are fully wired; no placeholders.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The entire phase backend is curl-exercisable: create task → worktree on disk → spawn task session → DELETE worktree {} = 409 → {"stop_sessions":true} = 204 → branch survives
- Frontend plans 03-04 (bash tabs) and 03-05 (worktree meta line + cleanup dialog) consume exactly the interface table this plan shipped: task JSON worktree fields, the three worktree endpoints' bodies/status codes, and `POST /api/sessions {task_id}` / `GET /api/sessions?task_id=`
- The two 409 error strings ("sessions running", "worktree has uncommitted changes") are the dialog-variant discriminators for 03-05

---
*Phase: 03-worktree-isolation-bash-tabs*
*Completed: 2026-06-10*

## Self-Check: PASSED

All created files and the SUMMARY exist on disk; all 6 task commits (ac1b3fc, acb1f81, d00fd3b, c46d90e, dedd5cb, 461804c) present in git log; `go build ./... && go vet ./... && go test ./... -count=1` green across the module.

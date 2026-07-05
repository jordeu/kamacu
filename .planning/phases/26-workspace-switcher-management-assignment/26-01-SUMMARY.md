---
phase: 26-workspace-switcher-management-assignment
plan: 01
subsystem: api
tags: [workspaces, crud, sqlite, net-http, go]

# Dependency graph
requires:
  - phase: 25-workspace-data-foundation
    provides: "migration 00012 (workspaces table + is_default flag + name COLLATE NOCASE unique index + projects.workspace_id NOT NULL … ON DELETE RESTRICT FK) and the BackfillWorkspaces startup hook"
provides:
  - "GET /api/workspaces — every workspace name-sorted (COLLATE NOCASE), JSON array"
  - "POST /api/workspaces — create-by-name with case-insensitive duplicate rejection (409)"
  - "PATCH /api/workspaces/{id} — rename any workspace including the default Personal (409 on collision)"
  - "DELETE /api/workspaces/{id} — guarded: 409 for is_default, 409 for non-empty (count in message), 204 for empty non-default"
  - "Workspace JSON struct + workspaceColumns + scanWorkspace (reusable read wire for the switcher UI)"
affects: [26-02-project-transfer, 26-sidebar-workspace-switcher, 26-navigation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "db-only handler struct (workspaceHandlers) mirroring settingsHandlers"
    - "COLLATE NOCASE duplicate pre-check before INSERT/UPDATE with the UNIQUE index as backstop"
    - "load-marker-then-branch guarded delete (mirrors projectHandlers.delete)"

key-files:
  created: []
  modified:
    - internal/api/workspaces.go
    - internal/api/routes.go
    - internal/api/workspaces_test.go

key-decisions:
  - "Rename is NEVER gated on is_default — the default Personal workspace is renamable (D-04)"
  - "Delete guards key off the is_default flag, never the literal name 'Personal' (rename-proof, D-06)"
  - "Non-empty delete uses an explicit SELECT COUNT(*) FROM projects so the 409 carries a clean count message instead of a raw ON DELETE RESTRICT FK error (D-05)"

patterns-established:
  - "Workspace read wire: Workspace struct + workspaceColumns + scanWorkspace (INTEGER→bool for is_default), mirroring the Project read wire"
  - "All /api/workspaces SQL uses ? placeholders (T-26-01); single-writer SetMaxOpenConns(1) serializes writes"

requirements-completed: [WSMGMT-01, WSMGMT-02, WSMGMT-03, WSMGMT-04, WSNAV-01]

# Metrics
duration: 6min
completed: 2026-07-05
---

# Phase 26 Plan 01: Workspaces CRUD API Summary

**The `/api/workspaces` CRUD surface — list (name-sorted), create-by-name (case-insensitive dup → 409), rename (Personal included), and a guarded delete that refuses the default and non-empty workspaces server-side — all proven green under `go test`.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-05T18:57:10Z
- **Completed:** 2026-07-05T19:03:37Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- `workspaceHandlers` CRUD (list/create/update/delete) in `internal/api/workspaces.go` on top of the shipped migration 00012 — no schema change.
- The 4 `/api/workspaces` routes registered in `routes.go` (`GET`, `POST`, `PATCH /{id}`, `DELETE /{id}`).
- Server-side enforcement: case-insensitive duplicate rejection on create + rename (409), rename never gated on `is_default` (Personal renamable), and a two-guard delete (default-protected + non-empty-protected with a count-carrying refusal).
- 9 new handler tests covering every behavior, green alongside the retained `TestBackfillWorkspaces`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Workspace struct + list/create/rename handlers + route registration** - `b038720` (feat)
2. **Task 2: Guarded delete handler (block-when-non-empty, block-when-default)** - `c747300` (feat)
3. **Task 3: Handler tests for workspace CRUD + guards** - `7603cb5` (test)

**Plan metadata:** see final `docs(26-01)` commit.

## Files Created/Modified
- `internal/api/workspaces.go` - Added `Workspace` struct, `workspaceColumns`, `scanWorkspace`, `workspaceHandlers` struct, and the `list`/`create`/`update`/`delete` handlers (kept the pre-existing `BackfillWorkspaces` hook untouched).
- `internal/api/routes.go` - Registered the 4 `/api/workspaces` routes on `wh := &workspaceHandlers{db: db}`.
- `internal/api/workspaces_test.go` - Added 9 HTTP handler tests + a `personalWorkspaceID` helper (kept `TestBackfillWorkspaces`).

## Decisions Made
- **Rename is never gated on `is_default`** (D-04): `update` contains no reference to the flag, so the default Personal workspace is renamable.
- **Delete guards key off `is_default`, never the name** (D-06): rename-proof default protection.
- **Explicit COUNT before delete** (D-05): the non-empty refusal carries `move or remove its N project(s) first` rather than surfacing the `ON DELETE RESTRICT` FK error; the FK remains the ultimate backstop.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Deferred the DELETE route registration from Task 1 to Task 2**
- **Found during:** Task 1 (route registration)
- **Issue:** The plan directs Task 1 to register all four routes including `DELETE /api/workspaces/{id}` → `wh.delete`, but the `delete` handler is only implemented in Task 2. Registering it in Task 1 references an undefined method, so `go build ./...` (Task 1's own verification) would fail — breaking the atomic-commit invariant that every task commit builds independently.
- **Fix:** Registered `GET`/`POST`/`PATCH` in the Task 1 commit; moved the single `DELETE` route registration line into the Task 2 commit alongside the handler it points at. The final tree has all four routes registered (verified: `routes.go` matches `/api/workspaces` 4×), satisfying the plan's key_links and overall verification.
- **Files modified:** internal/api/routes.go
- **Verification:** `go build ./...` + `go vet ./internal/api/` exit 0 after both Task 1 and Task 2; `grep -c '/api/workspaces' routes.go` == 4.
- **Committed in:** `b038720` (Task 1) + `c747300` (Task 2)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Only shifted one route-registration line one commit later so each commit builds standalone. Final state is byte-identical to the plan's intended surface. No scope creep.

## Issues Encountered
- **Duplicate `itoa` test helper:** my initial test file declared a local `itoa(int64) string`, which collided with an existing helper in `pullrequests_test.go` (same package). Removed my duplicate and reused the existing package-level `itoa`; dropped the now-unused `strconv` import. Resolved before the Task 3 commit.

## Known Stubs
None — every handler is fully wired to the DB and the delete guards are enforced server-side. The `DELETE` route was momentarily deferred one commit (see Deviations) but is present and tested in the final tree.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The workspace read wire (`GET /api/workspaces`) is ready for the sidebar switcher UI (WSNAV-01) to render.
- Create/rename/delete are ready for the switcher's add/rename/delete affordances (WSMGMT-01/02/03/04).
- Project transfer (plan 02, WSPROJ-01) will assign `projects.workspace_id` via the partial-PATCH path; the non-empty-delete test deliberately places a project via a direct `UPDATE` so it does not yet depend on that endpoint.

## Self-Check: PASSED

- Files exist: `internal/api/workspaces.go`, `internal/api/routes.go`, `internal/api/workspaces_test.go` — all FOUND.
- Commits exist: `b038720`, `c747300`, `7603cb5` — all FOUND.
- Key links present: `type workspaceHandlers` in workspaces.go, `COUNT(*) FROM projects` delete link, 4× `/api/workspaces` route registrations.
- `go build ./...` + `go vet ./internal/api/` exit 0; full `go test ./internal/api/` green.

---
*Phase: 26-workspace-switcher-management-assignment*
*Completed: 2026-07-05*

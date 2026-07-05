---
phase: 26-workspace-switcher-management-assignment
plan: 02
subsystem: api
tags: [go, sqlite, workspaces, projects, rest, http]

# Dependency graph
requires:
  - phase: 25-workspace-data-foundation
    provides: "workspaces table + projects.workspace_id NOT NULL FK (migration 00012), workspace_id on the projects wire, defaultWorkspaceID() resolver"
provides:
  - "PATCH /api/projects/{id} accepts an optional validated workspace_id that transfers the project (WSPROJ-01, D-17: no dedicated transfer route)"
  - "POST /api/projects accepts an optional workspace_id; a project lands in the requested (active) workspace, else falls back to the default Personal (WSPROJ-02, D-10)"
  - "resolveCreateWorkspaceID resolver: single validate-or-default seam, single defaultWorkspaceID() call site"
affects: [workspace-switcher-ui, project-move-menu, workspaces-crud]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "validate-then-append-else-reject-400: SELECT 1 FROM workspaces before mutating the row (mirrors the github_repo reject idiom)"
    - "resolve-once-thread-through: resolve workspace_id up front in create() and pass it into both create paths"

key-files:
  created: []
  modified:
    - internal/api/projects.go
    - internal/api/projects_test.go

key-decisions:
  - "Transfer is an optional workspace_id on the existing partial-PATCH update — no new route (D-17)"
  - "create resolves workspace_id once (validate supplied id, else default Personal) and threads it through folder + repo-first paths; defaultWorkspaceID keeps a single call site (D-10/D-14)"

patterns-established:
  - "validate-then-append-else-reject-400 for FK-referenced ids on a partial-PATCH"
  - "resolveCreateWorkspaceID centralizes the validate-or-default decision so both INSERT sites share one resolved id"

requirements-completed: [WSPROJ-01, WSPROJ-02]

# Metrics
duration: 6 min
completed: 2026-07-05
---

# Phase 26 Plan 02: Workspace Transfer & Create-in-Active Summary

**Projects transfer between workspaces via an optional validated `workspace_id` on the existing PATCH, and new projects land in the requested (active) workspace with a safe fall-back to Personal — no new route (D-17).**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-05T19:08:26Z
- **Completed:** 2026-07-05T19:14:00Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments

- WSPROJ-01: `PATCH /api/projects/{id}` now accepts an optional `workspace_id`; a valid target transfers the project and returns the moved row, a non-existent target is rejected 400 (`workspace not found`) with the row untouched (T-26-05 mitigation).
- WSPROJ-02: `POST /api/projects` (both the folder and repo-first paths) now honors an optional `workspace_id`; the project lands in that workspace, or in the default Personal workspace when omitted (T-26-06 mitigation via `NOT NULL DEFAULT 1` FK + the explicit resolver).
- Introduced `resolveCreateWorkspaceID`, collapsing the two prior `defaultWorkspaceID()` INSERT-site calls into a single validate-or-default seam (one direct call site remains, inside the resolver).
- Added transfer + create-in-workspace tests seeding the target workspace directly via the DB handle, keeping this wave-1 plan independent of plan 01's `/api/workspaces` handlers.

## Task Commits

Each task was committed atomically:

1. **Task 1: Transfer via optional workspace_id on PATCH /api/projects/{id}** - `bd3cb5f` (feat)
2. **Task 2: Active-workspace-at-create on both create paths** - `4fda349` (feat)
3. **Task 3: Transfer + create-in-workspace tests** - `dd42b1e` (test)

**Plan metadata:** (this commit) (docs: complete plan)

## Files Created/Modified

- `internal/api/projects.go` - Added `WorkspaceID *int64` to the `update` and `create` req structs; validate-then-append transfer block in `update`; `resolveCreateWorkspaceID` resolver; threaded the resolved `wsID` through the folder path and `createByRepo` (signature extended to `(w, r, repoInput, nameInput, wsID)`), removing the second `defaultWorkspaceID()` call.
- `internal/api/projects_test.go` - Added `seedWorkspace` helper, `TestProjectTransferWorkspace` (200 transfer + list reflection, 400 on non-existent target with row unchanged), and `TestProjectCreateInWorkspace` (create-in-workspace, Personal fallback, 400 no-row on non-existent workspace).

## Decisions Made

- None beyond the plan's pre-locked D-17/D-10/D-14. Transfer stays on the existing PATCH (D-17); create validates a supplied id and otherwise defaults to Personal (D-10/D-14).

## Deviations from Plan

None - plan executed exactly as written.

## TDD Gate Compliance

Task 3 is `tdd="true"` but the plan deliberately sequences implementation (Tasks 1-2) before the proving tests (Task 3), so a pre-implementation RED phase was not applicable — the tests were written and confirmed green against the already-landed behavior. The task's files are test-only (`projects_test.go`), so no MVP+TDD behavior-adding gate applied. The full `internal/api` suite passes (`go test ./internal/api/ -count=1`).

## Issues Encountered

- `state.update-progress` reported "Progress field not found in STATE.md" (the STATE.md progress lives in frontmatter, which `state.advance-plan` and `roadmap.update-plan-progress` handled). Non-blocking; position and plan counts were advanced correctly.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The backend for WSPROJ-01 (transfer via the `⋯` menu) and WSPROJ-02 (create-in-active) is complete and green; the switcher UI can drive both off the existing `/api/projects` routes with no transfer-specific endpoint.
- No blockers.

## Self-Check: PASSED

- Files: `internal/api/projects.go` FOUND, `internal/api/projects_test.go` FOUND
- Commits: `bd3cb5f` FOUND, `4fda349` FOUND, `dd42b1e` FOUND

---
*Phase: 26-workspace-switcher-management-assignment*
*Completed: 2026-07-05*

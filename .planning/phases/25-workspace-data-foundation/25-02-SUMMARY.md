---
phase: 25-workspace-data-foundation
plan: 02
subsystem: api
tags: [go, sqlite, workspaces, projects, foreign-key, tdd]

# Dependency graph
requires:
  - phase: 25-workspace-data-foundation (plan 01)
    provides: "migration 00012 — workspaces table + projects.workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT; Personal(id=1, is_default=1) seeded"
provides:
  - "workspace_id on the projects read wire (GET/POST/PATCH via RETURNING projectColumns) — D-10"
  - "both project-create paths (folder + repo-first) resolve is_default=1 and set workspace_id explicitly — D-09"
  - "projectHandlers.defaultWorkspaceID() helper resolving the rename-proof default workspace"
affects: [26-workspace-management, phase-26-switcher, WSPROJ-02]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Four-site column-order contract (struct / projectColumns const / scanProject Scan) extended for a new column — same slot in all three (Pitfall 4)"
    - "Explicit default-workspace resolve (SELECT id FROM workspaces WHERE is_default = 1) before each create INSERT, backed by the migration DEFAULT 1"

key-files:
  created: []
  modified:
    - internal/api/projects.go
    - internal/api/projects_test.go

key-decisions:
  - "workspace_id sits between icon_color and created_at in the const + Scan (mirrors the icon_letters/icon_color slot) — D-10"
  - "Default resolved by the is_default=1 flag, not the name 'Personal' (rename-proof, D-02); extracted to defaultWorkspaceID() and called by both create paths"
  - "Explicit resolve+set kept even though the column has DEFAULT 1 — the documented D-09 interim behavior Phase 26's WSPROJ-02 refines to the active workspace; resolve failure returns 500, never a workspace-less project"

patterns-established:
  - "Pattern 1: adding a NOT NULL non-null-on-wire column threads through struct+const+scan in one matching slot and rides RETURNING projectColumns onto every create/update response for free"
  - "Pattern 2: a black-box create-response test cannot distinguish an explicit resolve from the column DEFAULT backstop; the resolve is proven RED by a no-default-workspace 500 test instead"

requirements-completed: [WSDATA-01]

# Metrics
duration: 12min
completed: 2026-07-05
---

# Phase 25 Plan 02: Projects Workspace Wire + Default-Assign Summary

**`workspace_id` now ships on the projects read wire (GET/POST/PATCH) and both create paths resolve and set the default Personal workspace, so every new project satisfies the NOT NULL FK.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-05T08:51:00Z
- **Completed:** 2026-07-05T08:57:00Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 2

## Accomplishments
- Threaded `WorkspaceID int64 \`json:"workspace_id"\`` through the `Project` struct, `projectColumns` const, and `scanProject` Scan in one matching slot (between `icon_color` and `created_at`) — `GET /api/projects` now serializes `workspace_id` (==1 for migrated projects), and PATCH/POST carry it automatically via `RETURNING projectColumns`.
- Both create paths (`create()` folder + `createByRepo()` repo-first) now resolve the default workspace via `SELECT id FROM workspaces WHERE is_default = 1` and set `workspace_id` explicitly (D-09); resolve failure returns 500 rather than silently producing a workspace-less project.
- Added a shared `defaultWorkspaceID()` helper (D-02: rename-proof, flag-based default resolution).

## Task Commits

Each task was committed atomically (TDD RED → GREEN):

1. **Task 1 (RED): failing wire test** - `051064a` (test)
2. **Task 1 (GREEN): thread workspace_id onto the read wire** - `38718ee` (feat)
3. **Task 2 (RED): failing create-path default-workspace tests** - `6fdaddb` (test)
4. **Task 2 (GREEN): default-to-Personal in both create INSERTs** - `c624d14` (feat)

_No REFACTOR commits — the GREEN implementations were already clean (the `defaultWorkspaceID()` extraction was written directly during Task 2 GREEN)._

## Files Created/Modified
- `internal/api/projects.go` - Added `WorkspaceID` to the `Project` struct (with an extended column-order doc-comment), inserted `workspace_id` into `projectColumns` and `scanProject`, added `defaultWorkspaceID()`, and updated both create INSERTs to resolve+set `workspace_id`.
- `internal/api/projects_test.go` - Added `TestProjectsWorkspaceWire` (read-wire), `TestProjectsCreateAssignsDefaultWorkspace` (folder + repo-first subtests, wire + DB-row assertions), and `TestProjectsCreateRejectsWhenNoDefaultWorkspace` (500-when-no-default).

## Decisions Made
- **Default resolution is flag-based, not name-based** (D-02): `SELECT id FROM workspaces WHERE is_default = 1`, extracted to `defaultWorkspaceID()` so both create paths share one rename-proof resolver.
- **Explicit resolve+set retained despite `DEFAULT 1`** (D-09): the column DEFAULT is only a backstop; the explicit resolve is the documented interim behavior that Phase 26's WSPROJ-02 refines to the *active* workspace, and it converts a missing default into a hard 500 instead of a silently workspace-less project.
- **Column slot:** `workspace_id` placed between `icon_color` and `created_at` in all three sites (struct/const/scan) to mirror the existing icon-column precedent and keep Scan-arg counts aligned (Pitfall 4).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

**TDD RED nuance for Task 2 (resolved, not a defect).** A black-box test asserting the create *response* carries `workspace_id == 1` already passes after Task 1, because migration 00012's `DEFAULT 1` column assigns `workspace_id` even without the explicit resolve — and Phase 25 has no workspace-management endpoints, so the default is always id 1 (the DEFAULT and the resolve are observably indistinguishable on the happy path). Per the fail-fast rule I investigated the unexpected pass and confirmed it is the documented D-09 backstop, not a broken test. To obtain a genuine RED for Task 2's *new* behavior, I added `TestProjectsCreateRejectsWhenNoDefaultWorkspace`: it clears `is_default` on all workspace rows (FK target row intact) and asserts create returns 500 with no project persisted. That test fails before the Task 2 implementation (returns 201 via the DEFAULT backstop) and passes after — a real RED→GREEN cycle for the explicit resolve. `TestProjectsCreateAssignsDefaultWorkspace` remains as an end-state invariant regression guard.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- The projects API carries `workspace_id` on every read/write response — Phase 26's workspace switcher can filter on it directly (phase success criterion 4, D-10).
- Both create paths hold the NOT NULL FK invariant via an explicit resolve; Phase 26's WSPROJ-02 can swap `defaultWorkspaceID()` for the *active*-workspace resolver at the same two call sites without touching the wire/scan plumbing.
- No new workspace endpoints were added (D-11 honored — all workspace CRUD stays in Phase 26).

## Verification
- `go build ./...` — exit 0
- `go vet ./internal/api/...` — exit 0
- Targeted: `TestProjectList`, `TestProjectsWorkspaceWire`, `TestProjectCreate*`, `TestCreateRepo*`, `TestProjectsCreateAssignsDefaultWorkspace`, `TestProjectsCreateRejectsWhenNoDefaultWorkspace` — all PASS
- Full `go test ./internal/api/...` — ok (85.8s); the MEMORY-noted flaky `TestSessionTmuxReattach` passed this run (no isolation re-run needed)

## Self-Check: PASSED
- FOUND: internal/api/projects.go (workspace_id threaded; both INSERTs resolve is_default=1)
- FOUND: internal/api/projects_test.go (3 new tests)
- FOUND commit: 051064a, 38718ee, 6fdaddb, c624d14

---
*Phase: 25-workspace-data-foundation*
*Completed: 2026-07-05*

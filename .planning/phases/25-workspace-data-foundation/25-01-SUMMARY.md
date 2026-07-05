---
phase: 25-workspace-data-foundation
plan: 01
subsystem: database
tags: [sqlite, goose, migration, foreign-key, workspaces, modernc]

# Dependency graph
requires:
  - phase: 18-project-icon-data-foundation
    provides: "BackfillProjectIcons idempotent startup-hook pattern + projects icon columns/wire the workspace_id wire change (Phase 26) will sit beside"
provides:
  - "Migration 00012_workspaces.sql: workspaces table (is_default flag, NOCASE-unique name index) + Personal (id=1)"
  - "projects.workspace_id NOT NULL FK REFERENCES workspaces(id) ON DELETE RESTRICT, DB-enforced from 00012"
  - "In-SQL backfill: every pre-existing project assigned workspace_id=1, all tasks preserved"
  - "TestWorkspacesMigration: real staged-upgrade proof of the WSDATA-01/02 schema invariants"
affects: [26-workspace-switcher-management-assignment, workspaces, projects-wire, BackfillWorkspaces]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "NO TRANSACTION + PRAGMA foreign_keys OFF/ON migration to add a NOT NULL REFERENCES column to a populated table"
    - "Existing-row assignment via the ADD COLUMN DEFAULT (no separate UPDATE loop)"
    - "Staged-upgrade migration test: goose.UpTo(N-1) -> seed -> Migrate -> assert survival"

key-files:
  created:
    - internal/store/migrations/00012_workspaces.sql
    - internal/store/workspaces_migration_test.go
  modified: []

key-decisions:
  - "Approach A (ADD COLUMN under FK OFF) — no table rebuild; rebuild would cascade-wipe tasks"
  - "is_default flag identifies the protected default (rename-proof), not the name 'Personal'"
  - "Personal inserted first into the empty INTEGER PRIMARY KEY table => deterministic id=1, which DEFAULT 1 targets"

patterns-established:
  - "FK-column-add requires FK OFF + NO TRANSACTION; both Up and Down re-arm PRAGMA foreign_keys = ON (single pooled connection under SetMaxOpenConns(1))"
  - "Migration test stages the pre-migration schema via goose.UpTo and proves pre-existing data survives the upgrade"

requirements-completed: [WSDATA-01, WSDATA-02]

# Metrics
duration: ~10min
completed: 2026-07-05
---

# Phase 25 Plan 01: Workspace Data Foundation Summary

**Migration 00012 adds a `workspaces` table + a DB-enforced `projects.workspace_id NOT NULL … REFERENCES … ON DELETE RESTRICT` FK, creating the protected default Personal (id=1) and assigning every existing project to it in-SQL — with a real staged-upgrade test proving no project/task data is lost.**

## Performance

- **Duration:** ~10 min
- **Completed:** 2026-07-05
- **Tasks:** 2
- **Files modified:** 2 (both created)

## Accomplishments
- `00012_workspaces.sql`: `workspaces` table (minimal columns per D-01, `is_default` flag per D-02, `name COLLATE NOCASE` unique index per D-03), Personal inserted as id=1, and `projects.workspace_id` added as a `NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT` column — the `DEFAULT 1` assigns every existing project to Personal as part of the ALTER (D-04/D-05/D-06).
- The migration uses `-- +goose NO TRANSACTION` + `PRAGMA foreign_keys OFF` around explicit `BEGIN/COMMIT`, then re-arms `PRAGMA foreign_keys = ON` on both Up and Down (Pitfall 3 mitigation — the shared single connection would otherwise stay FK-disabled app-wide).
- Clean reversible Down (drop column → drop index → drop table) under the same FK-toggle discipline (D-08).
- `TestWorkspacesMigration` exercises the REAL upgrade path: stages the schema at 00011 (`goose.UpTo(db, "migrations", 11)`), seeds 2 projects + 2 tasks, applies 00012, then asserts every pre-seeded project is reassigned to Personal (id 1), all tasks intact, single is_default Personal, DEFAULT 1 on new rows, NOT NULL + RESTRICT both bite, FK re-armed ON, and a second `Migrate` creates no second Personal.

## Task Commits

Each task was committed atomically:

1. **Task 1: Write migration 00012_workspaces.sql (Approach A)** — `5a00aac` (feat)
2. **Task 2: Write TestWorkspacesMigration invariant test** — `3bc3d50` (test)

## Files Created/Modified
- `internal/store/migrations/00012_workspaces.sql` — the workspaces table + NOCASE unique name index + Personal row + `projects.workspace_id` NOT NULL RESTRICT FK, under NO TRANSACTION + FK-toggle discipline; reversible Down.
- `internal/store/workspaces_migration_test.go` — `package store` test staging the pre-00012 schema and asserting the WSDATA-01/02 invariants against the real goose runner.

## Decisions Made
None beyond the plan — implemented exactly per RESEARCH.md §1 (Approach A), which was empirically pre-verified against the pinned driver. The research-flagged Open Question 1 (the `BackfillWorkspaces` Go hook and its vestigial collect-then-update loop) is out of scope for this plan (the Go startup hook + `projects.go` wire change are Phase 25's later plans / not in 25-01's task list); this plan is the migration + its test only.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None. `go build ./...`, `go vet ./internal/store/...`, and `go test ./internal/store/...` all green on first run; `TestWorkspacesMigration` passed first attempt (staging log confirms version 11 → 12 → no-op re-run).

## Verification Results
- `go build ./...` — exit 0.
- `go vet ./internal/store/...` — exit 0.
- `go test ./internal/store/... -run 'TestWorkspacesMigration|TestMigrate|TestDeleteProjectCascadesToTasks' -v` — all PASS.
- Full `go test ./internal/store/...` — ok (no regressions).

## Threat Model Coverage
- **T-25-02** (FK left disabled on the pooled connection): mitigated — both Up and Down end with `PRAGMA foreign_keys = ON`; `TestWorkspacesMigration` asserts `PRAGMA foreign_keys == 1` after `Migrate`.
- **T-25-03** (data loss during the ALTER): mitigated — Approach A (`ADD COLUMN`), no `DROP TABLE projects`; the test asserts existing projects assigned + task count unchanged.
- **T-25-01** (injection): the migration inserts only the literal `'Personal'`; no user input, no string concatenation.

## Next Phase Readiness
- Schema foundation is landed: the `workspaces` table and DB-enforced `projects.workspace_id` FK exist from migration 00012. Ready for the remaining Phase 25 backend work (`BackfillWorkspaces` startup hook, `Project`/`projectColumns`/`scanProject` wire change, create-path `workspace_id` resolution) and the Phase 26 switcher UI.
- No blockers.

---
*Phase: 25-workspace-data-foundation*
*Completed: 2026-07-05*

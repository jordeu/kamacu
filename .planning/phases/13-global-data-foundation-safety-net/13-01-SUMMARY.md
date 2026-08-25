---
phase: 13-global-data-foundation-safety-net
plan: 1
subsystem: database
tags: [sqlite, goose, migration, schema-rebuild, singleton, constraint]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: tasks/projects schema and the goose embedded-FS migration runner (store.Open pooled DSN, Migrate)
  - phase: 03-opencode-engine-spawn-and-activity-based
    provides: agents table (00013) with the movable is_default flag the 00017 seed reads
provides:
  - global_task singleton table (id=1 DB-enforced, agent FK ON DELETE RESTRICT, root/github_repo storage contract, nullable resume-id columns)
  - tmux_sessions rebuilt with nullable task_id + scope discriminator ('task'|'global') and table-level XOR CHECK
  - Staged-upgrade proof (real goose runner, seeded v1.12-shape DB) that every pre-existing tmux row survives byte-for-byte
affects: [14-global-config-api, 15-sessions-backend-scope-spawn, 16-view-settings-bar, 17-hardening-e2e]

tech-stack:
  added: []  # zero new dependencies (locked decision, verified against go.mod)
  patterns:
    - "CREATE-copy-drop-rename table rebuild under goose NO TRANSACTION + PRAGMA foreign_keys OFF/re-arm (first full-table rebuild in the repo — sqlite.org §8 blessed ordering)"
    - "XOR CHECK spelling (task_id IS NULL) = (scope = 'global') — both rejection directions asserted in tests"
    - "Seed-from-flag (SELECT ... WHERE is_default = 1), never a hardcoded agent id"

key-files:
  created:
    - internal/store/migrations/00017_global_task.sql
    - internal/store/migrations/00018_tmux_scope.sql
    - internal/store/global_task_migration_test.go
  modified: []

key-decisions:
  - "Two-migration split (00017 plain transaction + 00018 NO TRANSACTION) per planner resolution — keeps the FK-off discipline scoped to the one migration that needs it; exact split the research rehearsal validated"
  - "Seed via SELECT 1, id FROM agents WHERE is_default = 1 (never literal 1) — the default flag is movable on real installs (4 agents observed)"
  - "00018 Down rebuilds the old NOT NULL shape copying back only task-backed rows — reverting scope support inherently drops global rows (acceptable dev-only Down)"

patterns-established:
  - "Full-table rebuild migration: PRAGMA foreign_keys=OFF outside BEGIN/COMMIT, create-new → copy → drop-old → rename-new, trailing PRAGMA foreign_key_check (non-gating) + foreign_keys=ON re-arm (load-bearing under SetMaxOpenConns(1))"
  - "Byte-for-byte survival proof: BEFORE/AFTER snapshots formatted as exact strings per row (id|task_id|n|name|label|created_at), never COUNT-only"

requirements-completed: [GDATA-01, GDATA-02]

coverage:
  - id: D1
    description: "global_task singleton migration — DB-enforced id=1 row seeded from the is_default agent with root/github_repo/resume-id storage contract (GDATA-01 schema half)"
    requirement: GDATA-01
    verification:
      - kind: integration
        ref: internal/store/global_task_migration_test.go#TestGlobalTaskMigration
        status: pass
    human_judgment: false
  - id: D2
    description: "tmux_sessions scope rebuild migration — nullable task_id + scope discriminator + XOR CHECK with byte-for-byte survival of every pre-existing row (GDATA-02)"
    requirement: GDATA-02
    verification:
      - kind: integration
        ref: internal/store/global_task_migration_test.go#TestTmuxScopeRebuildMigration
        status: pass
    human_judgment: false

duration: 5 min
completed: 2026-08-25
status: complete
---

# Phase 13 Plan 1: Global data foundation migrations Summary

**Two goose migrations — the `global_task` singleton (CHECK id=1, agent FK RESTRICT, seeded from the movable is_default flag) and the `tmux_sessions` scope rebuild (nullable task_id + XOR CHECK via CREATE-copy-drop-rename under NO TRANSACTION) — proven byte-for-byte-safe by staged-upgrade tests on the real goose runner**

## Performance

- **Duration:** 5 min
- **Started:** 2026-08-25T13:20:22Z
- **Completed:** 2026-08-25T13:25:53Z
- **Tasks:** 2
- **Files modified:** 3 (all new)

## Accomplishments
- Migration 00017 establishes the `global_task` singleton: `CHECK (id = 1)` makes a second row unrepresentable, `agent_id REFERENCES agents(id) ON DELETE RESTRICT` backs the future delete guard, and the seed reads `is_default = 1` (never a hardcoded id — real installs carry 4 agents with a movable flag)
- Migration 00018 rebuilds `tmux_sessions` (the repo's first full-table rebuild) so task-less global rows exist: nullable `task_id`, `scope TEXT NOT NULL DEFAULT 'task'`, table-level XOR CHECK `(task_id IS NULL) = (scope = 'global')`, with FK-off/NO TRANSACTION discipline and the load-bearing trailing `PRAGMA foreign_keys = ON` re-arm for the pooled single connection
- `TestGlobalTaskMigration` + `TestTmuxScopeRebuildMigration` prove the real upgrade path (goose.UpTo 16 staging → seed → Migrate): singleton invariants, byte-for-byte survival of custom labels + exact created_at literals, both XOR rejection directions, both acceptance directions, FK re-arm canary, and second-Migrate idempotence

## Task Commits

Each task was committed atomically:

1. **Task 1: Create migrations 00017_global_task.sql and 00018_tmux_scope.sql** - `aea15bb` (feat)
2. **Task 2: Staged-upgrade migration test (TestGlobalTaskMigration + TestTmuxScopeRebuildMigration)** - `246be5a` (test)

**Plan metadata:** (see final docs commit below)

## Files Created/Modified
- `internal/store/migrations/00017_global_task.sql` - global_task singleton (plain transaction; seed from is_default agent; D-02 managed-root path contract documented)
- `internal/store/migrations/00018_tmux_scope.sql` - tmux_sessions rebuild (NO TRANSACTION, FK-off/re-arm, sqlite.org §8 ordering, XOR CHECK; Down rebuilds the old shape task-backed-only)
- `internal/store/global_task_migration_test.go` - staged-upgrade proofs (stageV116 helper + the two test functions)

## Decisions Made
- Followed the planner resolutions verbatim: two-migration split, `scope TEXT` discriminator, two separate nullable resume-id columns — all rehearsal-validated spellings lifted unchanged (no redesign)
- Test seeding uses `n=7` for the ambiguous-row insert so `UNIQUE(task_id, n)` cannot mask the XOR CHECK failure (the certain-failure isolation the plan's acceptance criteria require)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Schema half of GDATA-01/GDATA-02 is proven; Plan 13-02 builds on it: BackfillGlobalTask boot wiring, scope-aware orphan sweep fix, agents delete-guard extension, and the real-install rehearsal
- The verified `scope` column and XOR CHECK are the contract Phase 15's spawn path will insert through (existing writers already safe by construction per research audit)

---
*Phase: 13-global-data-foundation-safety-net*
*Completed: 2026-08-25*

## Self-Check: PASSED

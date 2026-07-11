---
phase: 03-opencode-engine-spawn-and-activity-based
plan: "01"
subsystem: database
tags: [sqlite, migration, opencode, seed, backfill, system-agent]

requires:
  - phase: 01-agent-data-foundation-and-spawn-engine
    provides: agents table + BackfillAgents pattern (migration 00013)
provides:
  - opencode system agent seed (migration 00015 + BackfillOpenCodeAgent)
affects: [03-02, 03-03, 03-04, opencode-engine]

tech-stack:
  added: []
  patterns: [system-agent-seed-mirrors-claude-seed, non-deletable-is-system]

key-files:
  created:
    - internal/store/migrations/00015_opencode_agent.sql
    - internal/store/opencode_agent_migration_test.go
  modified:
    - internal/api/agents_backfill.go
    - internal/api/agents_backfill_test.go
    - cmd/kamacu/main.go

key-decisions:
  - "opencode seeded as a non-deletable system agent (engine='opencode', is_default=0, is_system=1) — present at startup, never API-created or deleted"
  - "BackfillOpenCodeAgent mirrors BackfillAgents: idempotent re-create of the opencode seed if missing"
  - "Three shared tests that assumed a single-system-agent baseline were regression-fixed for the new second system agent"

patterns-established:
  - "Each built-in engine gets its own system seed via migration + idempotent backfill hook"

requirements-completed: []

coverage:
  - id: D1
    description: opencode system agent seed (migration 00015 + BackfillOpenCodeAgent) with shared-test regression fixes
    requirement: ""
    verification:
      - kind: integration
        ref: "internal/store/opencode_agent_migration_test.go"
        status: pass
      - kind: unit
        ref: "internal/api/agents_backfill_test.go"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-07-07
status: complete
---

# Plan 03-01: opencode system agent seed (migration + backfill) Summary

**Migration 00015 seeds opencode as a non-deletable system agent (engine='opencode', is_system=1) alongside Claude; BackfillOpenCodeAgent re-creates it idempotently at startup. Three shared tests fixed for the new second system agent.**

## Performance

- **Completed:** 2026-07-07
- **Tasks:** 1
- **Files created:** 2
- **Files modified:** 3+

## Accomplishments
- `00015_opencode_agent.sql` — inserts the opencode system seed (engine='opencode', is_default=0, is_system=1)
- `BackfillOpenCodeAgent` startup hook — idempotent re-create if missing, wired in main.go
- Regression-fixed the 3 shared tests (agents_migration, agents_backfill, agents_crud) that assumed a single-system-agent baseline

## Task Commits

1. **T01: migration 00015 + BackfillOpenCodeAgent + regression fixes** — `77e52aa` (fix)

## Files Created/Modified
- `internal/store/migrations/00015_opencode_agent.sql` — opencode system seed insert
- `internal/store/opencode_agent_migration_test.go` — upgrade + invariant test
- `internal/api/agents_backfill.go` — BackfillOpenCodeAgent hook
- `internal/api/agents_backfill_test.go`, `internal/store/agents_migration_test.go`, `internal/api/agents_crud_test.go` — multi-system-agent baseline fixes
- `cmd/kamacu/main.go` — hook wired into startup

## Decisions Made
- opencode is a non-deletable system agent (is_system=1), never created or deleted via API
- BackfillOpenCodeAgent mirrors the existing BackfillAgents idempotent shape

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- opencode seed present; ready for the heuristic status branch (03-02) which treats opencode as a full-heuristics engine

---
*Phase: 03-opencode-engine-spawn-and-activity-based*
*Completed: 2026-07-07*

---
phase: 01-agent-data-foundation-and-spawn-engine
plan: "01"
subsystem: database
tags: [sqlite, migration, fk, agents, seed]

requires:
  - phase: 00012_workspaces
    provides: migration pattern (NO TRANSACTION + PRAGMA FK off/on)
provides:
  - agents table (name, command, engine, is_default, is_system)
  - Claude Code seed agent (id=1, engine=claude, is_default=1, is_system=1)
  - projects.agent_id NOT NULL DEFAULT 1 FK with ON DELETE RESTRICT
affects: [01-02, 01-03, 01-04, 01-05, 01-06, configurable-agents]

tech-stack:
  added: []
  patterns: [migration-mirrors-workspaces-no-transaction, fk-backstop-behind-handler-guard]

key-files:
  created:
    - internal/store/migrations/00013_agents.sql
    - internal/store/agents_migration_test.go
  modified: []

key-decisions:
  - "Default agent_id = 1 (Claude) assigned via ADD COLUMN ... DEFAULT 1 so existing projects keep v1.9-and-prior behavior byte-for-byte"
  - "ON DELETE RESTRICT is the DB backstop behind the handler-level block-until-unassigned guard (T03)"
  - "Migration mirrors 00012_workspaces.sql: NO TRANSACTION + PRAGMA foreign_keys off/on, so ADD COLUMN FK is re-armed correctly"

patterns-established:
  - "Staged-upgrade migration test: stage pre-migration state, apply migration, assert data survives + invariants"

requirements-completed: []

coverage:
  - id: D1
    description: agents table + Claude seed + projects.agent_id FK with staged-upgrade data survival
    requirement: ""
    verification:
      - kind: integration
        ref: "internal/store/agents_migration_test.go#TestAgentsMigration"
        status: pass
    human_judgment: false

duration: ~37min
completed: 2026-07-06
status: complete
---

# Plan 01-01: Migration 00013 — agents table + Claude seed + projects.agent_id FK Summary

**SQLite migration 00013 adds the agents table, seeds the Claude Code default agent, and back-fills projects.agent_id (NOT NULL DEFAULT 1, ON DELETE RESTRICT) — staged-upgrade test proves pre-existing data survives.**

## Performance

- **Completed:** 2026-07-06
- **Tasks:** 1
- **Files created:** 2

## Accomplishments
- `agents` table with name/command/engine (default 'custom')/is_default/is_system and a COLLATE NOCASE unique name index
- Claude Code seed row (id=1, engine='claude', is_default=1, is_system=1) so every project defaults to Claude
- `projects.agent_id NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT` — existing projects reassigned to Claude automatically, preserving prior behavior

## Task Commits

1. **T01: migration 00013 + staged-upgrade test** — `1cf2bc1` (feat)

## Files Created/Modified
- `internal/store/migrations/00013_agents.sql` — agents table, Claude seed, projects.agent_id FK (NO TRANSACTION + PRAGMA FK off/on)
- `internal/store/agents_migration_test.go` — mirrors TestWorkspacesMigration: stages pre-00013 state, applies migration, asserts data survival + invariants (reassign-to-1, exactly-one default, is_system=1, NOT NULL, RESTRICT, FK re-armed ON, idempotent)

## Decisions Made
- DEFAULT 1 on the new column assigns existing projects to Claude as part of ADD COLUMN (no separate backfill needed for the column itself)
- ON DELETE RESTRICT chosen as DB backstop; the handler-level block-until-unassigned guard (T03) is the primary protection

## Deviations from Plan

None — work was already implemented and committed; this summary documents the delivered state.

## Issues Encountered
None

## Next Phase Readiness
- agents table ready for BackfillAgents startup hook (01-02) and /api/agents CRUD (01-03)
- projects.agent_id ready to thread through the projects API (01-04)

---
*Phase: 01-agent-data-foundation-and-spawn-engine*
*Completed: 2026-07-06*

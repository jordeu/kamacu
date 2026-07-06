---
id: T01
parent: S01
milestone: M001
key_files:
  - internal/store/migrations/00013_agents.sql
  - internal/store/agents_migration_test.go
key_decisions:
  - Migration 00013 mirrors 00012's NO TRANSACTION + PRAGMA FK off/on shape exactly (proven reversible-migration pattern for NOT NULL REFERENCES ADD COLUMN)
  - Claude seed is engine='claude'+is_system=1+is_default=1 at id=1, so DEFAULT 1 on projects.agent_id preserves v1.9 behavior byte-for-byte for every existing project
  - goose requires BOTH -- +goose NO TRANSACTION and -- +goose Up markers (the omission was the one bug caught in verification)
duration: 
verification_result: passed
completed_at: 2026-07-06T14:23:20.940Z
blocker_discovered: false
---

# T01: Added migration 00013 (agents table + Claude seed + projects.agent_id FK) with a staged-upgrade test proving pre-existing data survives and invariants hold.

**Added migration 00013 (agents table + Claude seed + projects.agent_id FK) with a staged-upgrade test proving pre-existing data survives and invariants hold.**

## What Happened

Created internal/store/migrations/00013_agents.sql mirroring 00012_workspaces.sql exactly: NO TRANSACTION + PRAGMA foreign_keys=OFF/ON + BEGIN/COMMIT, a CREATE TABLE agents (name, command, engine default 'custom', is_default, is_system), a COLLATE NOCASE unique name index, the Claude Code seed (engine='claude', is_default=1, is_system=1, id=1 deterministic), and an ALTER TABLE projects ADD COLUMN agent_id NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT. The DEFAULT 1 assigns every existing project to Claude as part of the ADD COLUMN, preserving v1.9-and-prior behavior.

Added internal/store/agents_migration_test.go mirroring TestWorkspacesMigration: it stages the DB at pre-00013 (UpTo 12), seeds projects+tasks that predate agents, applies 00013, and asserts (a) every pre-seeded project reassigns to agent_id=1, (b) no cascade task loss, (c) exactly one is_default agent at id=1, (d) the seed is engine='claude'+is_system=1, (e) a new project picks up DEFAULT 1, (f) NOT NULL rejects NULL agent_id, (g) ON DELETE RESTRICT rejects deleting the referenced agent, (h) foreign_keys re-armed ON, (i) idempotent re-run yields one agent.

Hit one bug during verification: the first version omitted the `-- +goose Up` directive (00012 carries both `-- +goose NO TRANSACTION` and `-- +goose Up`), causing goose's parser to fail with "unexpected state 0 on PRAGMA". Fixed by adding the missing marker; structural diff vs 00012 then identical.

## Verification

go test ./internal/store/... -run TestAgentsMigration -v → PASS (all staged-upgrade assertions green; 00013 applied in 717µs; idempotent re-run confirmed). Full store package: go test ./internal/store/... → ok 0.197s (workspaces regression unaffected). go vet ./internal/store/... → clean.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go test ./internal/store/... -run TestAgentsMigration -v` | 0 | ✅ pass | 44000ms |
| 2 | `go test ./internal/store/...` | 0 | ✅ pass | 197000ms |
| 3 | `go vet ./internal/store/...` | 0 | ✅ pass | 2000ms |

## Deviations

None.

## Known Issues

None.

## Files Created/Modified

- `internal/store/migrations/00013_agents.sql`
- `internal/store/agents_migration_test.go`

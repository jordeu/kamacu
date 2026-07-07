---
id: T01
parent: S01
milestone: M002
key_files:
  - internal/store/migrations/00015_opencode_agent.sql
  - internal/api/agents_backfill.go
  - cmd/kamacu/main.go
  - internal/store/opencode_agent_migration_test.go
  - internal/api/agents_backfill_test.go
  - internal/store/agents_migration_test.go
  - internal/api/agents_crud_test.go
key_decisions:
  - opencode seed uses NOT EXISTS guard (not INSERT OR IGNORE) to match the idempotent backfill posture and give partial-apply recovery without relying solely on the goose version table
  - opencode is is_default=0 to preserve R019 (claude sole default) so existing/new projects are never silently switched
  - BackfillOpenCodeAgent mirrors BackfillAgents literally rather than generalizing — keeps the two engine seeds decoupled and the diff minimal
duration: 
verification_result: passed
completed_at: 2026-07-07T18:21:27.074Z
blocker_discovered: false
---

# T01: Added migration 00015 + BackfillOpenCodeAgent to seed opencode as a non-deletable system agent (engine='opencode', is_default=0, is_system=1), wired into startup; plus regression-fixed the 3 shared tests that assumed a single system agent baseline.

**Added migration 00015 + BackfillOpenCodeAgent to seed opencode as a non-deletable system agent (engine='opencode', is_default=0, is_system=1), wired into startup; plus regression-fixed the 3 shared tests that assumed a single system agent baseline.**

## What Happened

Seeded opencode as a first-class built-in agent by mirroring the Claude Code seed pattern (00013_agents.sql + BackfillAgents) byte-for-byte in shape, with only the deliberate capability/default differences from D013.

New artifacts:
- `internal/store/migrations/00015_opencode_agent.sql`: a guarded INSERT (NOT EXISTS keyed on engine='opencode') inside goose's default transaction. Seeds name/command/engine='opencode', is_default=0 (claude stays the sole default — R019), is_system=1 (non-deletable — R018). The NOT EXISTS guard is belt-and-braces with the goose version table for partial-apply recovery. Down removes only the system seed.
- `internal/api/agents_backfill.go` → new `BackfillOpenCodeAgent(db)`: the in-process safety-net counterpart to the migration, mirroring BackfillAgents exactly (no-op when the seed exists; re-creates it if dropped; real DB errors propagated, never swallowed). All literal SQL, no string concatenation.
- `cmd/kamacu/main.go`: wired `BackfillOpenCodeAgent` immediately after `BackfillAgentExtraParams`, with the same slog-error-then-exit posture. Ordered after store.Migrate (table must exist).
- `internal/store/opencode_agent_migration_test.go`: two REAL-goose-runner tests — `TestOpencodeAgentMigration` (stages pre-00015, applies 00015, asserts exact seed shape + R019 + is_system=2 + FK re-armed ON + idempotent re-run) and `TestOpencodeAgentMigrationOnFreshDB` (full-migrate fresh DB, both seeds present).
- `internal/api/agents_backfill_test.go` → new `TestBackfillOpenCodeAgent`: idempotent no-op on healthy boot, re-creates after simulated drop, asserts opt-in shape, idempotent on second call.

Regression fixes (unavoidable consequence of T01's core deliverable — a second system seed raises the fresh-migrate baseline from 1→2 agents). All intent-preserving: the tests' actual concerns were per-engine/per-flag, not total count.
- `internal/store/agents_migration_test.go` (9): the "1 agent after re-run" idempotency check now asserts per-engine (1 claude + 1 opencode).
- `internal/api/agents_backfill_test.go` TestBackfillAgents: countAgents() total→countClaudeAgents() by engine (what BackfillAgents owns).
- `internal/api/agents_crud_test.go`: TestAgentList finds the claude seed by engine instead of list[0]; TestAgentCreate expects 3 (2 seeds + 1 custom); TestAgentDeleteUnused expects 2 after delete.

Captured the regression-class gotcha to memory (MEM026) so future system-seed additions anticipate it.

## Verification

Canonical task verify (`go build ./... && go test ./internal/store/ -run Opencode -count=1`) passes. The new migration tests (2 cases) and the new backfill test pass. Full-package regression confirms no breakage in the touched shared files: internal/store (full, incl. TestAgentsMigration fix) and internal/api (full, ~88s, incl. TestBackfillAgents + TestAgentList/Create/DeleteUnused fixes). go vet clean on cmd/kamacu, internal/api, internal/store. The R019 invariant (claude sole is_default=1) and R018 (opencode is_system=1) are asserted directly in the new tests.

## Verification Evidence

| # | Command | Exit Code | Verdict | Duration |
|---|---------|-----------|---------|----------|
| 1 | `go build ./...` | 0 | ✅ pass | 2825ms |
| 2 | `go test ./internal/store/ -run Opencode -count=1` | 0 | ✅ pass | 625ms |
| 3 | `go test ./internal/api/ -run TestBackfillOpenCodeAgent -count=1` | 0 | ✅ pass | 1623ms |
| 4 | `go test ./internal/store/ -count=1 (full, incl. TestAgentsMigration regression fix)` | 0 | ✅ pass | 224ms |
| 5 | `go test ./internal/api/ -count=1 (full, incl. Agent/Backfill regression fixes)` | 0 | ✅ pass | 87976ms |
| 6 | `go vet ./cmd/kamacu/ ./internal/api/ ./internal/store/` | 0 | ✅ pass | 1607ms |

## Deviations

Regression-fixed 3 shared test files not named in T01's Files list (internal/store/agents_migration_test.go, internal/api/agents_backfill_test.go TestBackfillAgents, internal/api/agents_crud_test.go TestAgentList/Create/DeleteUnused). All assumed a single-system-agent baseline that 00015 legitimately raises to 2; fixes are intent-preserving (per-engine/per-flag assertions). This is an unavoidable consequence of the task's core deliverable (seeding a second system agent), not a plan defect — captured as MEM026.

## Known Issues

None. (The is_system=1 deletion protection lives at the API handler, not the DB — by design, identical to the claude seed. TestOpencodeAgentMigration asserts the is_system flag on both seeds rather than attempting a SQL DELETE, which would succeed without FK references.)

## Files Created/Modified

- `internal/store/migrations/00015_opencode_agent.sql`
- `internal/api/agents_backfill.go`
- `cmd/kamacu/main.go`
- `internal/store/opencode_agent_migration_test.go`
- `internal/api/agents_backfill_test.go`
- `internal/store/agents_migration_test.go`
- `internal/api/agents_crud_test.go`

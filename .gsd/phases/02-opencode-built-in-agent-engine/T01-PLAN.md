---
estimated_steps: 11
estimated_files: 4
skills_used: []
---

# T01: Seed the opencode system agent (migration 00015 + idempotent backfill)

WHY: opencode must be a first-class selectable built-in agent. M001's `agents` table is the single source of truth (R012); a `is_system=1` seed row makes opencode non-deletable (R018) and available in the existing Settings + Project Settings dropdowns with NO UI change. This mirrors the Claude Code seed (migration 00013_agents.sql + BackfillAgents in internal/api/agents_backfill.go).

DO:
1. Create `internal/store/migrations/00015_opencode_agent.sql` (goose Up + Down). Up is a DATA-ONLY insert (no ALTER TABLE, no FK toggle) so it does NOT need the `-- +goose NO TRANSACTION` + `PRAGMA foreign_keys=OFF` dance that 00013 needed — a normal transactional migration is correct. Up body:
   `INSERT INTO agents (name, command, engine, is_default, is_system) SELECT 'opencode', 'opencode', 'opencode', 0, 1 WHERE NOT EXISTS (SELECT 1 FROM agents WHERE engine = 'opencode');`
   `is_default=0` is load-bearing — Claude stays the SOLE `is_default=1` row (R019 exactly-one-default invariant). `is_system=1` makes it non-deletable. Down: `DELETE FROM agents WHERE engine = 'opencode' AND is_system = 1;`. Add a header comment citing D013/D014 and explaining is_default=0.
2. Add `BackfillOpenCodeAgent(db *sql.DB) error` to `internal/api/agents_backfill.go`, mirroring `BackfillAgents` posture exactly: `SELECT id FROM agents WHERE engine = 'opencode'` → if found, return nil (no-op, healthy boot); if `sql.ErrNoRows`, INSERT the same literal seed row; propagate any other DB error. Keep the file leaf-ish (no new imports beyond database/sql + errors, already present). Doc comment cites M002 + the idempotent-no-op-on-healthy-boot contract.
3. Wire `api.BackfillOpenCodeAgent(db)` in `cmd/kamacu/main.go` IMMEDIATELY after the `api.BackfillAgentExtraParams(db)` block (~line 153), with the identical `slog.Error(...)+os.Exit(1)` posture and a one-line M002 comment.
4. Add `internal/store/opencode_agent_migration_test.go` mirroring `internal/store/agents_migration_test.go`: run `store.Migrate` on a fresh temp DB, then assert (a) exactly one row with engine='opencode' AND is_system=1 AND is_default=0; (b) exactly one is_default=1 row and its engine='claude' (R019 backstop); (c) calling Migrate twice (or the backfill twice) is idempotent — still exactly one opencode row.

CONSTRAINTS: Do NOT widen the agents_crud.go engine validator in this task (opencode is system-locked; its engine never reaches that validator). Do NOT touch the claude seed. Never use shell-concatenated SQL — literal/parameterized only (T-25-01).

DONE WHEN: the migration is reversible (Down drops the row), the opencode row exists post-Migrate, claude remains the sole default, Migrate is idempotent, and `go build ./...` compiles with the new backfill + main.go wiring.

Skills: go, sql-migration.

## Inputs

- `internal/store/migrations/00013_agents.sql`
- `internal/api/agents_backfill.go`
- `internal/store/agents_migration_test.go`
- `cmd/kamacu/main.go`

## Expected Output

- `internal/store/migrations/00015_opencode_agent.sql`
- `internal/store/opencode_agent_migration_test.go`

## Verification

go build ./... && go test ./internal/store/ -run Opencode -count=1

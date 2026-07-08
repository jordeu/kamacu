-- +goose Up
-- M002 (opencode built-in agent engine): seed a second NON-DELETABLE system
-- agent row for opencode so it is a first-class selectable built-in, mirroring
-- the Claude Code seed in 00013_agents.sql.
--
-- Shape mirrors the Claude seed exactly EXCEPT for the capability/deliberate
-- differences spelled out in D013:
--   engine='opencode'        -- its own capability tier (PTY spawn + on-disk
--                              plugin status hooks, see internal/opencode/)
--   command='opencode'       -- the host opencode CLI the app shells out to
--   is_default=0             -- claude REMAINS the sole is_default=1 row
--                              (R019 invariant intact); opencode is opt-in
--   is_system=1              -- non-deletable (R018), same protection as claude
--
-- This is a plain INSERT (no schema change), so goose's default transaction is
-- safe. The row is GUARDED by a NOT EXISTS predicate keyed on engine='opencode'
-- so the migration is idempotent under the goose version table AND against any
-- future re-run / partial-apply recovery (defence in depth, matching the
-- idempotent backfill posture in internal/api/agents_backfill.go). The unique
-- name index (idx_agents_name_nocase, 00013) makes 'OpenCode' case-insensitively
-- unique among agent names.
INSERT INTO agents (name, command, engine, is_default, is_system)
SELECT 'OpenCode', 'opencode', 'opencode', 0, 1
WHERE NOT EXISTS (SELECT 1 FROM agents WHERE engine = 'opencode');

-- +goose Down
-- Defensive: only remove the SYSTEM opencode seed (never a user-created row
-- that happened to reuse the name/engine). Matches the claude seed's "system
-- row only" posture.
DELETE FROM agents WHERE engine = 'opencode' AND is_system = 1;

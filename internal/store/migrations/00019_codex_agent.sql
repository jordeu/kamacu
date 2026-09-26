-- +goose Up
-- Codex built-in agent engine: seed a third NON-DELETABLE system agent row
-- for codex (OpenAI Codex CLI), mirroring the opencode seed in
-- 00015_opencode_agent.sql.
--
-- Shape mirrors the opencode seed exactly EXCEPT for the engine tier:
--   engine='codex'   -- its own capability tier (PTY spawn + env-gated status
--                       hooks via the exclusive ~/.codex/kamacu.config.toml
--                       profile layer, see internal/codex/)
--   command='codex'  -- the host codex CLI the app shells out to
--   is_default=0     -- claude REMAINS the sole is_default=1 row
--                       (R019 invariant intact); codex is opt-in
--   is_system=1      -- non-deletable (R018), same protection as claude/opencode
--
-- This is a plain INSERT (no schema change), so goose's default transaction is
-- safe. The row is GUARDED by a NOT EXISTS predicate keyed on engine='codex'
-- so the migration is idempotent under the goose version table AND against
-- re-runs / partial-apply recovery (defence in depth, matching 00015). The
-- unique name index (idx_agents_name_nocase, 00013) makes 'Codex'
-- case-insensitively unique among agent names.
INSERT INTO agents (name, command, engine, is_default, is_system)
SELECT 'Codex', 'codex', 'codex', 0, 1
WHERE NOT EXISTS (SELECT 1 FROM agents WHERE engine = 'codex');

-- +goose Down
-- Defensive: only remove the SYSTEM codex seed (never a user-created row that
-- happened to reuse the name/engine). Matches the opencode seed's "system row
-- only" posture.
DELETE FROM agents WHERE engine = 'codex' AND is_system = 1;

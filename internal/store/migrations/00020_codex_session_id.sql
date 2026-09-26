-- +goose Up
-- Codex restart-resume key: codex mints its own opaque session uuid (unlike
-- claude, where kamacu mints --session-id), and kamacu captures it from the
-- hook payloads the kamacu.config.toml profile POSTs (payload session_id —
-- verified against codex 0.157.0). The captured id is the `codex resume <id>`
-- key after a restart, exactly like tasks.opencode_session_id (00016) for
-- opencode.
--
-- One migration, both owners: 00016 could only touch tasks (global_task did
-- not exist until 00017); both tables exist now, so codex arms the task and
-- the global (Scratchpad) columns together. Nullable TEXT, no default: NULL
-- = no codex session captured yet (fresh spawn before the first hook, or an
-- untrusted-hook session).
ALTER TABLE tasks ADD COLUMN codex_session_id TEXT;
ALTER TABLE global_task ADD COLUMN codex_session_id TEXT;

-- +goose Down
ALTER TABLE tasks DROP COLUMN codex_session_id;
ALTER TABLE global_task DROP COLUMN codex_session_id;

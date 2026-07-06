-- +goose NO TRANSACTION
-- +goose Up
-- M001 (AGENTDATA-01/02): add the agents table + a DB-enforced NOT NULL
-- agent_id FK on projects, and assign every existing project to the default
-- Claude Code agent -- all in one migration so NOT NULL holds from 00013 on.
--
-- WHY NO TRANSACTION + PRAGMA foreign_keys=OFF:
--   SQLite rejects `ALTER TABLE ADD COLUMN ... REFERENCES ... NOT NULL DEFAULT`
--   while foreign_keys is ON ("Cannot add a REFERENCES column with non-NULL
--   default value"). foreign_keys can only be toggled OUTSIDE a transaction, and
--   goose runs migrations in a transaction by default -- hence NO TRANSACTION and
--   an explicit BEGIN/COMMIT. store.go opens with SetMaxOpenConns(1), so this same
--   pooled connection MUST be re-armed with foreign_keys=ON at the end.
--   (Same shape as 00012_workspaces.sql, proven there.)
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE agents (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  command    TEXT    NOT NULL,
  engine     TEXT    NOT NULL DEFAULT 'custom',
  is_default INTEGER NOT NULL DEFAULT 0,
  is_system  INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- Case-insensitive unique name ('Claude Code' and 'claude code' collide),
-- mirroring the workspaces idx_workspaces_name_nocase (D-03).
CREATE UNIQUE INDEX idx_agents_name_nocase ON agents (name COLLATE NOCASE);
-- D-M001-3: the protected default. First row into an empty INTEGER PRIMARY KEY
-- table => id = 1 (deterministic), which the DEFAULT below points at. The seed
-- is engine='claude' (the full hook/resume/quota capability tier), non-deletable
-- (is_system=1), and the default (is_default=1) so existing projects and new
-- projects that omit agent_id land on Claude -- preserving v1.9-and-prior
-- behavior byte-for-byte.
INSERT INTO agents (name, command, engine, is_default, is_system)
  VALUES ('Claude Code', 'claude', 'claude', 1, 1);
-- NOT NULL FK. DEFAULT 1 assigns EVERY existing project to the Claude agent as
-- part of the ADD COLUMN (no separate UPDATE needed). ON DELETE RESTRICT is the
-- backstop behind the handler-level block-until-unassigned guard.
ALTER TABLE projects
  ADD COLUMN agent_id INTEGER NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT;
COMMIT;
-- Non-gating parity with the official 12-step (goose Exec ignores its rows); the
-- real guarantee is that Claude(1) was inserted before the ADD COLUMN.
PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down
-- NO TRANSACTION applies to Down too; same FK-toggle discipline. modernc 3.53
-- supports DROP COLUMN directly (as 00007/00008/00009/00012).
PRAGMA foreign_keys = OFF;
BEGIN;
ALTER TABLE projects DROP COLUMN agent_id;
DROP INDEX idx_agents_name_nocase;
DROP TABLE agents;
COMMIT;
PRAGMA foreign_keys = ON;

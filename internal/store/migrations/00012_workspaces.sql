-- +goose NO TRANSACTION
-- +goose Up
-- Phase 25 (WSDATA-01/02): add the workspaces table + a DB-enforced NOT NULL
-- workspace_id FK on projects, and assign every existing project to the default
-- Personal workspace -- all in one migration so NOT NULL holds from 00012 on.
--
-- WHY NO TRANSACTION + PRAGMA foreign_keys=OFF:
--   SQLite rejects `ALTER TABLE ADD COLUMN ... REFERENCES ... NOT NULL DEFAULT`
--   while foreign_keys is ON ("Cannot add a REFERENCES column with non-NULL
--   default value"). foreign_keys can only be toggled OUTSIDE a transaction, and
--   goose runs migrations in a transaction by default -- hence NO TRANSACTION and
--   an explicit BEGIN/COMMIT. store.go opens with SetMaxOpenConns(1), so this same
--   pooled connection MUST be re-armed with foreign_keys=ON at the end.
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE workspaces (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- D-03: case-insensitive unique name ('Personal' and 'personal' collide).
CREATE UNIQUE INDEX idx_workspaces_name_nocase ON workspaces (name COLLATE NOCASE);
-- D-02: the protected default. First row into an empty INTEGER PRIMARY KEY table
-- => id = 1 (deterministic), which the DEFAULT below points at.
INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1);
-- D-04/D-05: NOT NULL FK. DEFAULT 1 assigns EVERY existing project to Personal as
-- part of the ADD COLUMN (no separate UPDATE needed). ON DELETE RESTRICT.
ALTER TABLE projects
  ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE RESTRICT;
COMMIT;
-- Non-gating parity with the official 12-step (goose Exec ignores its rows); the
-- real guarantee is that Personal(1) was inserted before the ADD COLUMN.
PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down
-- NO TRANSACTION applies to Down too; same FK-toggle discipline. modernc 3.53
-- supports DROP COLUMN directly (as 00007/00008/00009).
PRAGMA foreign_keys = OFF;
BEGIN;
ALTER TABLE projects DROP COLUMN workspace_id;
DROP INDEX idx_workspaces_name_nocase;
DROP TABLE workspaces;
COMMIT;
PRAGMA foreign_keys = ON;

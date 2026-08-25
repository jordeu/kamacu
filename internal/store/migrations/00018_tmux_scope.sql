-- +goose NO TRANSACTION
-- +goose Up
-- v1.13 (GDATA-02): rebuild tmux_sessions so task-less global rows can exist:
-- task_id becomes nullable and an explicit scope discriminator ('task' |
-- 'global') plus a table-level XOR CHECK make ambiguity unrepresentable at
-- the DB regardless of caller. Every existing row, label, and created_at
-- literal must survive byte-for-byte -- the staged-upgrade test in
-- global_task_migration_test.go proves it on the real goose upgrade path.
--
-- WHY the sqlite.org-blessed rebuild ordering (lang_altertable.html section 8):
--   create-new -> copy -> drop-old -> rename-new. Renaming the old table away
--   FIRST is the documented way to corrupt references; nothing else in the
--   schema references tmux_sessions (verified against the real install's
--   sqlite_master), so the RENAME rewrites no foreign REFERENCES.
--
-- WHY NO TRANSACTION + PRAGMA foreign_keys=OFF:
--   The rebuild DROPs a table other tables' FK discipline depends on being
--   consistent mid-flight; foreign_keys can only be toggled OUTSIDE a
--   transaction, and goose runs migrations in a transaction by default --
--   hence NO TRANSACTION and an explicit BEGIN/COMMIT. store.go opens with
--   SetMaxOpenConns(1), so this same pooled connection MUST be re-armed with
--   foreign_keys=ON at the end. (Same shape as 00012/00013, proven there.)
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE tmux_sessions_new (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER REFERENCES tasks(id),   -- nullable now: global rows have no task
    scope      TEXT    NOT NULL DEFAULT 'task' CHECK (scope IN ('task','global')),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n),
    CHECK ( (task_id IS NULL) = (scope = 'global') )   -- the XOR, verified spelling
);
-- Copy with scope='task': every pre-existing row is task-backed by the old
-- NOT NULL, and the column list pins the order -- no column omitted.
INSERT INTO tmux_sessions_new (id, task_id, scope, n, name, label, created_at)
  SELECT id, task_id, 'task', n, name, label, created_at FROM tmux_sessions;
DROP TABLE tmux_sessions;
ALTER TABLE tmux_sessions_new RENAME TO tmux_sessions;
COMMIT;
-- Non-gating parity with the official 12-step (goose Exec ignores its rows);
-- the real guarantee is the copy above plus the trailing re-arm.
PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down
-- NO TRANSACTION applies to Down too; same FK-toggle discipline as Up.
-- Rebuilds the old NOT NULL-task_id shape, copying back only task-backed
-- rows -- reverting scope support inherently drops global rows (acceptable
-- for a dev-only Down).
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE tmux_sessions_old (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
INSERT INTO tmux_sessions_old (id, task_id, n, name, label, created_at)
  SELECT id, task_id, n, name, label, created_at FROM tmux_sessions
  WHERE task_id IS NOT NULL;
DROP TABLE tmux_sessions;
ALTER TABLE tmux_sessions_old RENAME TO tmux_sessions;
COMMIT;
PRAGMA foreign_keys = ON;

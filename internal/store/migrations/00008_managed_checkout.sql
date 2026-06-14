-- +goose Up
-- v1.4 managed-checkout marker (D-06). Distinguishes Kangent-managed clones
-- (the app cloned and OWNS the directory under ~/.kangent/repos/) from
-- user-pointed folder projects (the directory must NEVER be removed, D-09).
-- INTEGER 0/1 is SQLite's boolean idiom (as 00007's tasks.source default).
-- NOT NULL + DEFAULT 0 is REQUIRED by SQLite ADD COLUMN and is the correct
-- backfill: every EXISTING project predates v1.4 and is folder-based, so 0
-- (= not managed, never touch its dir) is the safe default for all rows.
ALTER TABLE projects ADD COLUMN managed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild) — same as 00007's Down.
ALTER TABLE projects DROP COLUMN managed;

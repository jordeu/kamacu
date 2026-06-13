-- +goose Up
-- Per-status entry timestamps (D-90). done_at drives the REAP-01 reaper
-- (time-in-Done); the other three are banked for future cycle-time/dwell-time
-- stats and have no current consumer. Set on each status transition in
-- tasks.go move (last-entry-wins). Existing Done rows backfilled from
-- updated_at so they are immediately reapable.
--
-- All four columns are nullable (no DEFAULT) -- a NULL means "never entered
-- that status". SQLite requires one ALTER TABLE ... ADD COLUMN per statement.
ALTER TABLE tasks ADD COLUMN todo_at TEXT;
ALTER TABLE tasks ADD COLUMN in_progress_at TEXT;
ALTER TABLE tasks ADD COLUMN in_review_at TEXT;
ALTER TABLE tasks ADD COLUMN done_at TEXT;

-- The only load-bearing backfill: make existing Done tasks reapable now.
-- todo_at/in_progress_at/in_review_at stay NULL for existing rows -- they are
-- banked stats with no current consumer, so approximating them is noise.
UPDATE tasks SET done_at = updated_at WHERE status = 'done';

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild). One DROP per statement, reverse order of the adds.
ALTER TABLE tasks DROP COLUMN done_at;
ALTER TABLE tasks DROP COLUMN in_review_at;
ALTER TABLE tasks DROP COLUMN in_progress_at;
ALTER TABLE tasks DROP COLUMN todo_at;

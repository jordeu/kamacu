-- +goose Up
ALTER TABLE tasks ADD COLUMN recurrent INTEGER NOT NULL DEFAULT 0 CHECK (recurrent IN (0, 1));
-- Retained even if the successor is deleted: completing an instance is one-shot.
ALTER TABLE tasks ADD COLUMN recurrence_spawned INTEGER NOT NULL DEFAULT 0 CHECK (recurrence_spawned IN (0, 1));

-- +goose Down
ALTER TABLE tasks DROP COLUMN recurrence_spawned;
ALTER TABLE tasks DROP COLUMN recurrent;

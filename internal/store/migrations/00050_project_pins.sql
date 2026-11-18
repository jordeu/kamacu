-- +goose Up
ALTER TABLE projects ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1));

-- +goose Down
ALTER TABLE projects DROP COLUMN pinned;

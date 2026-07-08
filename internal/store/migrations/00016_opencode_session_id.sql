-- +goose Up
ALTER TABLE tasks ADD COLUMN opencode_session_id TEXT;
-- +goose Down
ALTER TABLE tasks DROP COLUMN opencode_session_id;

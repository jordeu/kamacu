-- +goose Up
ALTER TABLE tasks ADD COLUMN claude_session_id TEXT;
-- +goose Down
ALTER TABLE tasks DROP COLUMN claude_session_id;

-- +goose Up
ALTER TABLE tasks ADD COLUMN branch TEXT;
ALTER TABLE tasks ADD COLUMN worktree_path TEXT;
ALTER TABLE tasks ADD COLUMN worktree_error TEXT;
-- +goose Down
ALTER TABLE tasks DROP COLUMN worktree_error;
ALTER TABLE tasks DROP COLUMN worktree_path;
ALTER TABLE tasks DROP COLUMN branch;

-- +goose Up
ALTER TABLE monitor_episodes ADD COLUMN input_generation INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE monitor_episodes DROP COLUMN input_generation;

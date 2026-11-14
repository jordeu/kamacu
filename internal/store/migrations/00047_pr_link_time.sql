-- +goose Up
ALTER TABLE task_pr_links ADD COLUMN linked_at TEXT NOT NULL DEFAULT '';
-- Historical association times were not recorded separately from evidence.
UPDATE task_pr_links SET linked_at=observed_at;

-- +goose Down
ALTER TABLE task_pr_links DROP COLUMN linked_at;

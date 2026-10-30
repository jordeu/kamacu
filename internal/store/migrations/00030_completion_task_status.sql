-- +goose Up
ALTER TABLE inbox_questions ADD COLUMN completion_task_status TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE inbox_questions DROP COLUMN completion_task_status;

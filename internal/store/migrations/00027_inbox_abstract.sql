-- +goose Up
ALTER TABLE inbox_questions ADD COLUMN abstract TEXT NOT NULL DEFAULT '';
UPDATE inbox_questions SET abstract = substr(summary, 1, 180);

-- +goose Down
ALTER TABLE inbox_questions DROP COLUMN abstract;

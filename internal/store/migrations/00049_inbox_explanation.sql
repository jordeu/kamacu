-- +goose Up
ALTER TABLE inbox_questions ADD COLUMN explanation TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE inbox_questions DROP COLUMN explanation;

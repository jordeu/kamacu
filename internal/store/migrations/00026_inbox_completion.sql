-- +goose Up
ALTER TABLE inbox_questions ADD COLUMN kind TEXT NOT NULL DEFAULT 'question' CHECK(kind IN ('question','completion'));

-- +goose Down
ALTER TABLE inbox_questions DROP COLUMN kind;

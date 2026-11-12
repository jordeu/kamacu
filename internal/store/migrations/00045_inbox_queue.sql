-- +goose Up
ALTER TABLE inbox_questions ADD COLUMN queue_position INTEGER NOT NULL DEFAULT 0;
WITH positions AS (
  SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, rowid) AS position
  FROM inbox_questions
)
UPDATE inbox_questions SET queue_position=(SELECT position FROM positions WHERE positions.id=inbox_questions.id);
CREATE INDEX inbox_queue_position ON inbox_questions(queue_position);

-- All message producers, including notification batches, join the same queue.
-- +goose StatementBegin
CREATE TRIGGER inbox_enqueue AFTER INSERT ON inbox_questions
BEGIN
  UPDATE inbox_questions
  SET queue_position=(SELECT MAX(queue_position)+1 FROM inbox_questions)
  WHERE id=NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER inbox_enqueue;
DROP INDEX inbox_queue_position;
ALTER TABLE inbox_questions DROP COLUMN queue_position;

-- +goose Up
ALTER TABLE tasks ADD COLUMN snoozed_until TEXT;
ALTER TABLE tasks ADD COLUMN snoozed_from TEXT CHECK (snoozed_from IN ('in_progress', 'in_review'));
CREATE INDEX tasks_snoozed_until ON tasks(snoozed_until) WHERE snoozed_until IS NOT NULL;

-- +goose Down
DROP INDEX tasks_snoozed_until;
ALTER TABLE tasks DROP COLUMN snoozed_from;
ALTER TABLE tasks DROP COLUMN snoozed_until;

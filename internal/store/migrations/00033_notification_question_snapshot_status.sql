-- +goose Up
-- Earlier snapshots only contained pending questions, so the default also
-- preserves the meaning of queued processing requests created before upgrade.
ALTER TABLE notification_batch_questions ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
-- +goose Down
ALTER TABLE notification_batch_questions DROP COLUMN status;

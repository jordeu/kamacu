-- +goose Up
ALTER TABLE notification_runs ADD COLUMN next_attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_runs ADD COLUMN recovery_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_runs ADD COLUMN recovery_class TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_action_steps ADD COLUMN failure_class TEXT NOT NULL DEFAULT '';
CREATE INDEX notification_runs_due ON notification_runs(state,next_attempt,updated_at);
-- +goose Down
DROP INDEX notification_runs_due;
ALTER TABLE notification_action_steps DROP COLUMN failure_class;
ALTER TABLE notification_runs DROP COLUMN recovery_class;
ALTER TABLE notification_runs DROP COLUMN recovery_attempts;
ALTER TABLE notification_runs DROP COLUMN next_attempt;

-- +goose Up
-- Lifecycle observations have their own clock; a ready prompt or exit must not
-- refresh the timestamp of an older work summary.
ALTER TABLE session_work ADD COLUMN state_observed_at TEXT NOT NULL DEFAULT '';
UPDATE session_work SET state_observed_at=observed_at;
ALTER TABLE session_pr_links ADD COLUMN task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE;
UPDATE session_pr_links SET task_id=(SELECT task_id FROM session_work w WHERE w.session_id=session_pr_links.session_id);
-- +goose Down
ALTER TABLE session_pr_links DROP COLUMN task_id;
ALTER TABLE session_work DROP COLUMN state_observed_at;

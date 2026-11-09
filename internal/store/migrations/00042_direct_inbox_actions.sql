-- +goose Up
-- Development cards without option-specific actions cannot be executed by the
-- new executor. Preserve history and uncertain runs; requeue only unexecuted work.
UPDATE notification_events SET raw_status='pending',raw_revision=raw_revision+1 WHERE id IN (
 SELECT e.event_id FROM notification_question_events e JOIN inbox_questions q ON q.id=e.question_id WHERE q.origin='notification' AND (q.status='pending' OR q.delivery_status='queued')
 UNION SELECT notification_id FROM inbox_questions WHERE origin='notification' AND (status='pending' OR delivery_status='queued') AND notification_id IS NOT NULL
);
UPDATE inbox_questions SET status='unanswered',reason='notification_actions_changed',revision=revision+1 WHERE origin='notification' AND status='pending';
UPDATE inbox_questions SET delivery_status='failed',execution_result='Saved option actions changed. Process pending notifications again.' WHERE origin='notification' AND delivery_status='queued';
UPDATE inbox_questions SET delivery_status='uncertain',execution_result='Execution was interrupted. Check linked tasks before continuing.' WHERE origin='notification' AND delivery_status='running';
UPDATE notification_runs SET state='failed' WHERE state='queued';
UPDATE notification_runs SET state='uncertain' WHERE state='running';
UPDATE notification_batches SET status='failed',error='Inbox action format changed. Process pending notifications again.' WHERE status IN ('queued','running');
UPDATE notification_rule_previews SET status='failed',error='Inbox action format changed. Request a new preview.' WHERE status IN ('queued','running');
CREATE TABLE notification_runs_v2 (
 id TEXT PRIMARY KEY REFERENCES inbox_questions(id),
 state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','uncertain','blocked','partial')),
 history TEXT NOT NULL DEFAULT '[]',
 updated_at INTEGER NOT NULL,
 plan TEXT NOT NULL DEFAULT '{}',
 action TEXT NOT NULL DEFAULT '{}',
 attention_pending INTEGER NOT NULL DEFAULT 0,
 attention_retryable INTEGER NOT NULL DEFAULT 0,
 attention_question_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO notification_runs_v2(id,state,history,updated_at,plan) SELECT id,state,history,updated_at,plan FROM notification_runs;
DROP TABLE notification_runs;
ALTER TABLE notification_runs_v2 RENAME TO notification_runs;
CREATE UNIQUE INDEX notification_runs_attention_question ON notification_runs(attention_question_id) WHERE attention_question_id!='';
CREATE INDEX notification_runs_pending_attention ON notification_runs(updated_at) WHERE attention_pending=1;
CREATE TABLE notification_action_steps (
 run_id TEXT NOT NULL REFERENCES notification_runs(id),
 step_key TEXT NOT NULL,
 operation_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 is_mutation INTEGER NOT NULL DEFAULT 0,
 arguments TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('running','succeeded','failed','uncertain','blocked')),
 result TEXT NOT NULL DEFAULT '',
 task_id INTEGER NOT NULL DEFAULT 0,
 session_id TEXT NOT NULL DEFAULT '',
 started_at INTEGER NOT NULL,
 finished_at INTEGER,
 PRIMARY KEY(run_id,step_key)
);
-- +goose Down
DROP TABLE notification_action_steps;
DROP INDEX notification_runs_attention_question;
DROP INDEX notification_runs_pending_attention;
ALTER TABLE notification_runs DROP COLUMN attention_pending;
ALTER TABLE notification_runs DROP COLUMN attention_retryable;
ALTER TABLE notification_runs DROP COLUMN attention_question_id;
ALTER TABLE notification_runs DROP COLUMN action;

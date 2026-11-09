-- +goose Up
CREATE TABLE notification_settings (
 id INTEGER PRIMARY KEY CHECK(id=1),
 profile_id INTEGER REFERENCES monitor_profiles(id),
 revision INTEGER NOT NULL DEFAULT 1,
 context_bytes INTEGER NOT NULL DEFAULT 1500000 CHECK(context_bytes BETWEEN 64000 AND 8000000),
 output_tokens INTEGER NOT NULL DEFAULT 16000 CHECK(output_tokens BETWEEN 1024 AND 64000),
 timeout_seconds INTEGER NOT NULL DEFAULT 300 CHECK(timeout_seconds BETWEEN 30 AND 900),
 max_reads INTEGER NOT NULL DEFAULT 12 CHECK(max_reads BETWEEN 0 AND 40)
);
INSERT INTO notification_settings(id) VALUES(1);
UPDATE notification_rule_sets SET rules_json=(SELECT COALESCE(json_group_array(json_object('id',json_extract(r.value,'$.id'),'revision',COALESCE(json_extract(r.value,'$.revision'),1),'text',json_extract(r.value,'$.text'),'enabled',json(CASE WHEN json_extract(r.value,'$.enabled') THEN 'true' ELSE 'false' END))),'[]') FROM json_each(notification_rule_sets.rules_json) r),diagnostics='[]';
-- A development installation may have saved instructions that never compiled.
-- Keep that latest list when there was no active compiled set.
UPDATE notification_rule_state SET active_set_id=(SELECT id FROM notification_rule_sets ORDER BY revision DESC LIMIT 1) WHERE active_set_id IS NULL;
UPDATE notification_rule_sets SET status='active' WHERE id=(SELECT active_set_id FROM notification_rule_state WHERE id=1);
INSERT INTO notification_rule_sets(id,revision,status,rules_json,created_at,updated_at)
 SELECT 'notification-agent-initial-rules',COALESCE((SELECT max(revision) FROM notification_rule_sets),0)+1,'active','[]',unixepoch()*1000,unixepoch()*1000
 WHERE NOT EXISTS(SELECT 1 FROM notification_rule_state WHERE active_set_id IS NOT NULL);
UPDATE notification_rule_state SET active_set_id='notification-agent-initial-rules' WHERE active_set_id IS NULL;
UPDATE notification_rule_sets SET status='draft' WHERE status='compiling';
UPDATE notification_rule_previews SET status='failed',error='Notification processing replaced; request a new preview.' WHERE status IN ('queued','running');
UPDATE notification_events SET raw_status='pending',raw_revision=raw_revision+1 WHERE id IN (
 SELECT e.event_id FROM notification_question_events e JOIN inbox_questions q ON q.id=e.question_id WHERE q.origin='notification' AND (q.status='pending' OR q.delivery_status='queued')
 UNION SELECT notification_id FROM inbox_questions WHERE origin='notification' AND (status='pending' OR delivery_status='queued') AND notification_id IS NOT NULL
);
UPDATE inbox_questions SET status='unanswered',reason='notification_processor_changed',revision=revision+1 WHERE origin='notification' AND status='pending';
UPDATE notification_runs SET state='failed' WHERE state='queued';
UPDATE inbox_questions SET delivery_status='failed',execution_result='Notification processor changed; process the updates again.' WHERE origin='notification' AND delivery_status='queued';
UPDATE notification_runs SET state='uncertain' WHERE state='running';
UPDATE inbox_questions SET delivery_status='uncertain',execution_result='Execution was interrupted. Check the linked task before retrying.' WHERE origin='notification' AND delivery_status='running';
UPDATE notification_batches SET status='failed',error='Notification processor changed. Choose a notification model and process pending updates.' WHERE status IN ('queued','running');
DROP TABLE notification_rule_overrides;
ALTER TABLE notification_batches ADD COLUMN processor_snapshot TEXT NOT NULL DEFAULT '{}';
ALTER TABLE notification_batches ADD COLUMN processor_result TEXT NOT NULL DEFAULT '{}';
ALTER TABLE notification_batches ADD COLUMN processor_config TEXT NOT NULL DEFAULT '{}';
ALTER TABLE notification_batches ADD COLUMN processor_trace TEXT NOT NULL DEFAULT '[]';
ALTER TABLE notification_rule_decisions ADD COLUMN explanation TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_rule_decisions ADD COLUMN rule_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE notification_rule_decisions ADD COLUMN evidence_refs TEXT NOT NULL DEFAULT '[]';
UPDATE notification_card_plans SET plan=json_remove(plan,'$.clarification_key','$.uncertain');
-- +goose Down
ALTER TABLE notification_rule_decisions DROP COLUMN evidence_refs;
ALTER TABLE notification_rule_decisions DROP COLUMN rule_ids;
ALTER TABLE notification_rule_decisions DROP COLUMN explanation;
-- The processor migration invalidates executable development cards permanently.
CREATE TABLE notification_rule_overrides(rule_set_id TEXT NOT NULL REFERENCES notification_rule_sets(id),activity_id TEXT NOT NULL,rule_id TEXT NOT NULL,question_id TEXT NOT NULL REFERENCES inbox_questions(id),PRIMARY KEY(rule_set_id,activity_id,rule_id));
ALTER TABLE notification_batches DROP COLUMN processor_trace;
ALTER TABLE notification_batches DROP COLUMN processor_config;
ALTER TABLE notification_batches DROP COLUMN processor_result;
ALTER TABLE notification_batches DROP COLUMN processor_snapshot;
DROP TABLE notification_settings;

-- +goose Up
CREATE TABLE notification_rule_sets (
 id TEXT PRIMARY KEY,
 revision INTEGER NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK(status IN ('draft','compiling','ready','invalid','active','retired')),
 rules_json TEXT NOT NULL,
 diagnostics TEXT NOT NULL DEFAULT '[]',
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL
);
CREATE TABLE notification_rule_state (
 id INTEGER PRIMARY KEY CHECK(id=1),
 active_set_id TEXT REFERENCES notification_rule_sets(id)
);
INSERT INTO notification_rule_state(id) VALUES(1);
-- Preserve development instructions as a draft for explicit compilation and
-- activation. These rows are never consulted as runtime fallback settings.
WITH RECURSIVE paragraphs(n,text,rest) AS (
 SELECT 1,substr(value,1,instr(value||char(10)||char(10),char(10)||char(10))-1),substr(value,instr(value||char(10)||char(10),char(10)||char(10))+2)
 FROM settings WHERE key='notification_system_prompt' AND trim(value)!=''
 UNION ALL
 SELECT n+1,CASE WHEN n=98 THEN rest ELSE substr(rest,1,instr(rest||char(10)||char(10),char(10)||char(10))-1) END,CASE WHEN n=98 THEN '' ELSE substr(rest,instr(rest||char(10)||char(10),char(10)||char(10))+2) END
 FROM paragraphs WHERE trim(rest)!='' AND n<99
)
INSERT INTO notification_rule_sets(id,revision,status,rules_json,created_at,updated_at)
 SELECT 'imported-development-rules',1,'draft',json_group_array(json_object('id',printf('R%03d',n*2-1),'revision',1,'text',trim(text),'enabled',json('true'),'clauses',json('[]'))),unixepoch()*1000,unixepoch()*1000
 FROM paragraphs WHERE trim(text)!='' HAVING count(*)>0;
INSERT INTO notification_rule_sets(id,revision,status,rules_json,created_at,updated_at)
 SELECT 'imported-development-rules',1,'draft','[]',unixepoch()*1000,unixepoch()*1000
 WHERE NOT EXISTS(SELECT 1 FROM notification_rule_sets) AND EXISTS(SELECT 1 FROM settings WHERE key='notification_policy' AND value!='{}');
UPDATE notification_rule_sets SET rules_json=json_insert(rules_json,'$[#]',json_object('id','R002','revision',1,'text','Apply these explicitly configured filtering and grouping preferences: '||(SELECT value FROM settings WHERE key='notification_policy'),'enabled',json('true'),'clauses',json('[]')))
 WHERE id='imported-development-rules' AND EXISTS(SELECT 1 FROM settings WHERE key='notification_policy' AND value!='{}');
DELETE FROM settings WHERE key IN ('notification_system_prompt','notification_policy');
ALTER TABLE notification_runs ADD COLUMN plan TEXT NOT NULL DEFAULT '{}';
ALTER TABLE notification_batches ADD COLUMN rule_set_id TEXT REFERENCES notification_rule_sets(id);
ALTER TABLE notification_events ADD COLUMN held INTEGER NOT NULL DEFAULT 0;
CREATE TABLE notification_rule_decisions (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id),
 activity_id TEXT NOT NULL,
 event_ids TEXT NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('admitted','suppressed','held','duplicate')),
 evaluations TEXT NOT NULL,
 semantic TEXT NOT NULL DEFAULT '[]',
 facts TEXT NOT NULL,
 PRIMARY KEY(batch_id,activity_id)
);
CREATE TABLE notification_card_plans (
 question_id TEXT PRIMARY KEY REFERENCES inbox_questions(id) ON DELETE CASCADE,
 rule_set_id TEXT NOT NULL REFERENCES notification_rule_sets(id),
 plan TEXT NOT NULL
);
CREATE TABLE notification_rule_previews (
 id TEXT PRIMARY KEY,
 rule_set_id TEXT NOT NULL REFERENCES notification_rule_sets(id),
 event_ids TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','running','completed','failed')),
 result TEXT NOT NULL DEFAULT '{}',
 error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL
);
-- Requeue every input behind work invalidated by this development migration.
UPDATE notification_events SET raw_status='pending',held=0,raw_revision=raw_revision+1,state='queued'
 WHERE id IN (
 SELECT e.event_id FROM notification_question_events e JOIN inbox_questions q ON q.id=e.question_id
 WHERE q.origin='notification' AND (q.status='pending' OR q.delivery_status IN ('queued','running'))
 UNION SELECT notification_id FROM inbox_questions WHERE origin='notification' AND (status='pending' OR delivery_status IN ('queued','running')) AND notification_id IS NOT NULL
 UNION SELECT e.event_id FROM notification_batch_events e JOIN notification_batches b ON b.id=e.batch_id WHERE b.status IN ('queued','running')
 );
-- Invalidate pending development cards and queued replies from the retired path.
UPDATE inbox_questions SET status='unanswered',reason='notification_rules_replaced',revision=revision+1 WHERE origin='notification' AND status='pending';
UPDATE inbox_questions SET delivery_status='failed',execution_result='Notification rules replaced; process notifications again.' WHERE origin='notification' AND delivery_status IN ('queued','running');
UPDATE notification_runs SET state='failed' WHERE state IN ('queued','running');
UPDATE notification_batches SET status='failed',error='Notification rules replaced; process again after activating rules.' WHERE status IN ('queued','running');
-- +goose Down
DROP TABLE notification_rule_previews;
DROP TABLE notification_card_plans;
ALTER TABLE notification_runs DROP COLUMN plan;
DROP TABLE notification_rule_decisions;
ALTER TABLE notification_events DROP COLUMN held;
ALTER TABLE notification_batches DROP COLUMN rule_set_id;
DROP TABLE notification_rule_state;
DROP TABLE notification_rule_sets;

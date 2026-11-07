-- +goose Up
ALTER TABLE notification_rule_decisions RENAME TO notification_rule_decisions_old;
CREATE TABLE notification_rule_decisions (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id),
 activity_id TEXT NOT NULL,
 event_ids TEXT NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('admitted','suppressed','clarification','duplicate')),
 evaluations TEXT NOT NULL,
 semantic TEXT NOT NULL DEFAULT '[]',
 facts TEXT NOT NULL,
 PRIMARY KEY(batch_id,activity_id)
);
INSERT INTO notification_rule_decisions SELECT batch_id,activity_id,event_ids,CASE outcome WHEN 'held' THEN 'clarification' ELSE outcome END,evaluations,semantic,facts FROM notification_rule_decisions_old;
DROP TABLE notification_rule_decisions_old;
UPDATE notification_events SET raw_revision=raw_revision+1 WHERE held=1;
ALTER TABLE notification_events DROP COLUMN held;
CREATE TABLE notification_rule_overrides (
 rule_set_id TEXT NOT NULL REFERENCES notification_rule_sets(id),
 activity_id TEXT NOT NULL,
 rule_id TEXT NOT NULL,
 question_id TEXT NOT NULL REFERENCES inbox_questions(id),
 PRIMARY KEY(rule_set_id,activity_id,rule_id)
);
CREATE TABLE notification_rule_feedback (
 id TEXT PRIMARY KEY,
 question_id TEXT REFERENCES inbox_questions(id),
 rule_set_id TEXT NOT NULL REFERENCES notification_rule_sets(id),
 kind TEXT NOT NULL,
 instructions TEXT NOT NULL,
 result TEXT NOT NULL DEFAULT '{}',
 created_at INTEGER NOT NULL
);
CREATE TABLE notification_decision_snapshots (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id),
 activity_id TEXT NOT NULL,
 evidence TEXT NOT NULL CHECK(json_valid(evidence)),
 PRIMARY KEY(batch_id,activity_id)
);
-- +goose Down
ALTER TABLE notification_events ADD COLUMN held INTEGER NOT NULL DEFAULT 0;
DROP TABLE notification_decision_snapshots;
DROP TABLE notification_rule_feedback;
DROP TABLE notification_rule_overrides;
ALTER TABLE notification_rule_decisions RENAME TO notification_rule_decisions_new;
CREATE TABLE notification_rule_decisions (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id), activity_id TEXT NOT NULL, event_ids TEXT NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('admitted','suppressed','held','duplicate')),
 evaluations TEXT NOT NULL, semantic TEXT NOT NULL DEFAULT '[]', facts TEXT NOT NULL,
 PRIMARY KEY(batch_id,activity_id)
);
INSERT INTO notification_rule_decisions SELECT batch_id,activity_id,event_ids,CASE outcome WHEN 'clarification' THEN 'held' ELSE outcome END,evaluations,semantic,facts FROM notification_rule_decisions_new;
DROP TABLE notification_rule_decisions_new;

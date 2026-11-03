-- +goose Up
ALTER TABLE notification_events ADD COLUMN raw_status TEXT NOT NULL DEFAULT 'pending' CHECK(raw_status IN ('pending','archived'));
ALTER TABLE notification_events ADD COLUMN raw_revision INTEGER NOT NULL DEFAULT 1;
UPDATE notification_events SET raw_status='archived' WHERE state!='queued';
CREATE INDEX notification_raw_status ON notification_events(raw_status,id);
CREATE TABLE notification_batches (
 id TEXT PRIMARY KEY,
 status TEXT NOT NULL CHECK(status IN ('queued','running','completed','failed')),
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 error TEXT NOT NULL DEFAULT '',
 action_count INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX notification_batch_active ON notification_batches((1)) WHERE status IN ('queued','running');
CREATE TABLE notification_batch_events (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id) ON DELETE CASCADE,
 event_id INTEGER NOT NULL REFERENCES notification_events(id),
 revision INTEGER NOT NULL,
 PRIMARY KEY(batch_id,event_id)
);
CREATE TABLE notification_batch_questions (
 batch_id TEXT NOT NULL REFERENCES notification_batches(id) ON DELETE CASCADE,
 question_id TEXT NOT NULL REFERENCES inbox_questions(id),
 revision INTEGER NOT NULL,
 PRIMARY KEY(batch_id,question_id)
);
ALTER TABLE inbox_questions ADD COLUMN notification_group TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX notification_one_pending_group ON inbox_questions(notification_group) WHERE status='pending' AND notification_group!='';
CREATE TABLE notification_question_events (
 question_id TEXT NOT NULL REFERENCES inbox_questions(id) ON DELETE CASCADE,
 event_id INTEGER NOT NULL REFERENCES notification_events(id),
 PRIMARY KEY(question_id,event_id)
);
INSERT INTO notification_question_events SELECT id,notification_id FROM inbox_questions WHERE notification_id IS NOT NULL;
-- +goose Down
DROP TABLE notification_question_events;
DROP INDEX notification_one_pending_group;
ALTER TABLE inbox_questions DROP COLUMN notification_group;
DROP TABLE notification_batch_questions;
DROP TABLE notification_batch_events;
DROP INDEX notification_batch_active;
DROP TABLE notification_batches;
DROP INDEX notification_raw_status;
ALTER TABLE notification_events DROP COLUMN raw_revision;
ALTER TABLE notification_events DROP COLUMN raw_status;

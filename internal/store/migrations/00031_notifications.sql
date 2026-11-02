-- +goose Up
CREATE TABLE notification_sources (
 id TEXT PRIMARY KEY,
 provider TEXT NOT NULL,
 name TEXT NOT NULL,
 config TEXT NOT NULL,
 enabled INTEGER NOT NULL DEFAULT 1,
 retired INTEGER NOT NULL DEFAULT 0,
 account TEXT NOT NULL DEFAULT '',
 checkpoint TEXT NOT NULL,
 next_poll INTEGER NOT NULL DEFAULT 0,
 last_success INTEGER,
 last_error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL
);
CREATE TABLE notification_events (
 id INTEGER PRIMARY KEY,
 source_id TEXT NOT NULL REFERENCES notification_sources(id),
 external_id TEXT NOT NULL,
 version TEXT NOT NULL,
 payload TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','ignored','published')),
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 UNIQUE(source_id,external_id,version)
);
ALTER TABLE inbox_questions ADD COLUMN notification_id INTEGER REFERENCES notification_events(id);
ALTER TABLE inbox_questions ADD COLUMN source_name TEXT NOT NULL DEFAULT '';
ALTER TABLE inbox_questions ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE inbox_questions ADD COLUMN execution_result TEXT NOT NULL DEFAULT '';
DROP INDEX inbox_one_pending_session;
CREATE UNIQUE INDEX inbox_one_pending_session ON inbox_questions(session_id) WHERE status='pending' AND origin!='notification';
CREATE TABLE notification_runs (
 id TEXT PRIMARY KEY REFERENCES inbox_questions(id),
 state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','uncertain')),
 history TEXT NOT NULL DEFAULT '[]',
 updated_at INTEGER NOT NULL
);
CREATE INDEX notification_events_queue ON notification_events(state,next_attempt,id);
-- +goose Down
DROP TABLE notification_runs;
DELETE FROM inbox_questions WHERE origin='notification';
DROP INDEX inbox_one_pending_session;
CREATE UNIQUE INDEX inbox_one_pending_session ON inbox_questions(session_id) WHERE status='pending';
ALTER TABLE inbox_questions DROP COLUMN execution_result;
ALTER TABLE inbox_questions DROP COLUMN source_url;
ALTER TABLE inbox_questions DROP COLUMN source_name;
ALTER TABLE inbox_questions DROP COLUMN notification_id;
DROP TABLE notification_events;
DROP TABLE notification_sources;

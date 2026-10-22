-- +goose Up
CREATE TABLE inbox_questions (
  id TEXT PRIMARY KEY,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,
  created_session_id TEXT NOT NULL,
  request_key TEXT NOT NULL,
  summary TEXT NOT NULL,
  context TEXT NOT NULL,
  question TEXT NOT NULL,
  options TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','answered','dismissed','unanswered')),
  reason TEXT NOT NULL DEFAULT '',
  answer TEXT NOT NULL DEFAULT '',
  answer_source TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  heartbeat_at INTEGER NOT NULL,
  acknowledged_at INTEGER,
  UNIQUE(task_id, created_session_id, request_key)
);
CREATE UNIQUE INDEX inbox_one_pending_session ON inbox_questions(session_id) WHERE status = 'pending';
CREATE INDEX inbox_status_updated ON inbox_questions(status, updated_at DESC);
CREATE TABLE inbox_events (
  id INTEGER PRIMARY KEY,
  question_id TEXT NOT NULL REFERENCES inbox_questions(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  session_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE inbox_events;
DROP TABLE inbox_questions;

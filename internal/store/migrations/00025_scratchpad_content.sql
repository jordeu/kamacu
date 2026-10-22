-- +goose Up
-- NULL task_id represents Scratchpad; public APIs use the existing sentinel 0.
CREATE TEMP TABLE saved_inbox_events AS SELECT * FROM inbox_events;
CREATE TEMP TABLE saved_inbox_questions AS SELECT * FROM inbox_questions;
CREATE TEMP TABLE saved_artifact_uploads AS SELECT * FROM artifact_uploads;
CREATE TEMP TABLE saved_artifact_revisions AS SELECT * FROM artifact_revisions;
CREATE TEMP TABLE saved_artifacts AS SELECT * FROM artifacts;
DROP TABLE inbox_events;
DROP TABLE inbox_questions;
DROP TABLE artifact_uploads;
DROP TABLE artifact_revisions;
DROP TABLE artifacts;
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE CHECK(task_id IS NULL OR task_id > 0),
  title TEXT NOT NULL,
  revision TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX artifacts_task ON artifacts(task_id, created_at, id);
CREATE TABLE artifact_revisions (
  id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE
);
CREATE TABLE artifact_uploads (
  id TEXT PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE CHECK(task_id IS NULL OR task_id > 0),
  artifact_id TEXT NOT NULL,
  title TEXT NOT NULL,
  filename TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  expected_revision TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  uploaded INTEGER NOT NULL DEFAULT 0,
  published_revision TEXT NOT NULL DEFAULT ''
);

CREATE TABLE inbox_questions (
  id TEXT PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE CHECK(task_id IS NULL OR task_id > 0),
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

CREATE UNIQUE INDEX inbox_scratchpad_request ON inbox_questions(created_session_id, request_key) WHERE task_id IS NULL;
INSERT INTO artifacts SELECT * FROM saved_artifacts;
DROP TABLE saved_artifacts;
INSERT INTO artifact_revisions SELECT * FROM saved_artifact_revisions;
DROP TABLE saved_artifact_revisions;
INSERT INTO artifact_uploads SELECT * FROM saved_artifact_uploads;
DROP TABLE saved_artifact_uploads;
INSERT INTO inbox_questions SELECT * FROM saved_inbox_questions;
DROP TABLE saved_inbox_questions;
INSERT INTO inbox_events SELECT * FROM saved_inbox_events;
DROP TABLE saved_inbox_events;

-- +goose Down
CREATE TEMP TABLE saved_inbox_events AS SELECT * FROM inbox_events;
CREATE TEMP TABLE saved_inbox_questions AS SELECT * FROM inbox_questions;
CREATE TEMP TABLE saved_artifact_uploads AS SELECT * FROM artifact_uploads;
CREATE TEMP TABLE saved_artifact_revisions AS SELECT * FROM artifact_revisions;
CREATE TEMP TABLE saved_artifacts AS SELECT * FROM artifacts;
DROP TABLE inbox_events;
DROP TABLE inbox_questions;
DROP TABLE artifact_uploads;
DROP TABLE artifact_revisions;
DROP TABLE artifacts;
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  revision TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX artifacts_task ON artifacts(task_id, created_at, id);
CREATE TABLE artifact_revisions (
  id TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE
);
CREATE TABLE artifact_uploads (
  id TEXT PRIMARY KEY,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL,
  title TEXT NOT NULL,
  filename TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  expected_revision TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  uploaded INTEGER NOT NULL DEFAULT 0,
  published_revision TEXT NOT NULL DEFAULT ''
);

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

INSERT INTO artifacts SELECT * FROM saved_artifacts WHERE task_id IS NOT NULL;
DROP TABLE saved_artifacts;
INSERT INTO artifact_revisions SELECT * FROM saved_artifact_revisions WHERE artifact_id IN (SELECT id FROM artifacts);
DROP TABLE saved_artifact_revisions;
INSERT INTO artifact_uploads SELECT * FROM saved_artifact_uploads WHERE task_id IS NOT NULL;
DROP TABLE saved_artifact_uploads;
INSERT INTO inbox_questions SELECT * FROM saved_inbox_questions WHERE task_id IS NOT NULL;
DROP TABLE saved_inbox_questions;
INSERT INTO inbox_events SELECT * FROM saved_inbox_events WHERE question_id IN (SELECT id FROM inbox_questions);
DROP TABLE saved_inbox_events;

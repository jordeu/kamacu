-- +goose Up
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

-- +goose Down
DROP TABLE artifact_uploads;
DROP TABLE artifact_revisions;
DROP TABLE artifacts;

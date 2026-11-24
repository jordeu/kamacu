-- +goose Up
CREATE TABLE inbox_agent_settings (
 id INTEGER PRIMARY KEY CHECK(id=1),
 profile_id INTEGER REFERENCES monitor_profiles(id),
 revision INTEGER NOT NULL DEFAULT 1,
 context_bytes INTEGER NOT NULL DEFAULT 1500000 CHECK(context_bytes BETWEEN 64000 AND 8000000),
 output_tokens INTEGER NOT NULL DEFAULT 4000 CHECK(output_tokens BETWEEN 1024 AND 64000),
 timeout_seconds INTEGER NOT NULL DEFAULT 300 CHECK(timeout_seconds BETWEEN 30 AND 900),
 rules TEXT NOT NULL DEFAULT '[]'
);
INSERT INTO inbox_agent_settings(id) VALUES(1);
ALTER TABLE inbox_questions ADD COLUMN publishing INTEGER NOT NULL DEFAULT 0;
ALTER TABLE inbox_questions ADD COLUMN automation TEXT NOT NULL DEFAULT '{}';
CREATE TABLE inbox_agent_decisions (
 id TEXT PRIMARY KEY,
 question_id TEXT NOT NULL REFERENCES inbox_questions(id) ON DELETE CASCADE,
 question_revision INTEGER NOT NULL,
 config TEXT NOT NULL,
 profile TEXT NOT NULL DEFAULT '{}',
 snapshot TEXT NOT NULL DEFAULT '{}',
 decision TEXT NOT NULL DEFAULT '{}',
 outcome TEXT NOT NULL DEFAULT 'assessing',
 error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 UNIQUE(question_id,question_revision)
);
-- +goose Down
DROP TABLE inbox_agent_decisions;
ALTER TABLE inbox_questions DROP COLUMN automation;
ALTER TABLE inbox_questions DROP COLUMN publishing;
DROP TABLE inbox_agent_settings;

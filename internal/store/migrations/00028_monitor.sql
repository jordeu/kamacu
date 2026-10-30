-- +goose Up
CREATE TABLE monitor_profiles (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL,
 backend TEXT NOT NULL,
 model TEXT NOT NULL DEFAULT '',
 endpoint TEXT NOT NULL DEFAULT '',
 executable TEXT NOT NULL DEFAULT '',
 credential_env TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE monitor_settings (
 id INTEGER PRIMARY KEY CHECK(id=1),
 default_profile_id INTEGER REFERENCES monitor_profiles(id) ON DELETE RESTRICT
);
INSERT INTO monitor_settings(id) VALUES(1);
ALTER TABLE workspaces ADD COLUMN monitor_profile_id INTEGER REFERENCES monitor_profiles(id) ON DELETE RESTRICT;
ALTER TABLE inbox_questions ADD COLUMN origin TEXT NOT NULL DEFAULT 'agent';
ALTER TABLE inbox_questions ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE inbox_questions ADD COLUMN delivery_mode TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE inbox_questions ADD COLUMN delivery_status TEXT NOT NULL DEFAULT '';
ALTER TABLE inbox_questions ADD COLUMN evidence_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE inbox_questions ADD COLUMN input_generation INTEGER NOT NULL DEFAULT 0;
CREATE TABLE monitor_episodes (
 session_id TEXT PRIMARY KEY,
 episode_id TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 decision TEXT NOT NULL,
 seen INTEGER NOT NULL DEFAULT 0,
 active INTEGER NOT NULL DEFAULT 1
);
-- +goose Down
DROP TABLE monitor_episodes;
ALTER TABLE inbox_questions DROP COLUMN input_generation;
ALTER TABLE inbox_questions DROP COLUMN evidence_revision;
ALTER TABLE inbox_questions DROP COLUMN delivery_status;
ALTER TABLE inbox_questions DROP COLUMN delivery_mode;
ALTER TABLE inbox_questions DROP COLUMN hidden;
ALTER TABLE inbox_questions DROP COLUMN origin;
ALTER TABLE workspaces DROP COLUMN monitor_profile_id;
DROP TABLE monitor_settings;
DROP TABLE monitor_profiles;

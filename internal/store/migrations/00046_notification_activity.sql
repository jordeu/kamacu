-- +goose Up
ALTER TABLE notification_events ADD COLUMN force_replay INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_batches ADD COLUMN activity_metrics TEXT NOT NULL DEFAULT '{}';
CREATE TABLE notification_activity_subjects (
 source_id TEXT NOT NULL REFERENCES notification_sources(id),
 subject_url TEXT NOT NULL,
 account TEXT NOT NULL,
 fetched_through TEXT NOT NULL DEFAULT '',
 processed_through TEXT NOT NULL DEFAULT '',
 repair_from TEXT NOT NULL DEFAULT '',
 reconciled_at INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(source_id,subject_url)
);
CREATE TABLE notification_activities (
 id INTEGER PRIMARY KEY,
 source_id TEXT NOT NULL REFERENCES notification_sources(id),
 subject_url TEXT NOT NULL,
 resource_url TEXT NOT NULL,
 kind TEXT NOT NULL,
 digest TEXT NOT NULL,
 revision INTEGER NOT NULL,
 UNIQUE(source_id,subject_url,resource_url)
);
CREATE TABLE notification_activity_versions (
 activity_id INTEGER NOT NULL REFERENCES notification_activities(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL,
 digest TEXT NOT NULL,
 payload TEXT NOT NULL CHECK(json_valid(payload)),
 context_only INTEGER NOT NULL,
 fetched_at INTEGER NOT NULL,
 processed_batch_id TEXT REFERENCES notification_batches(id),
 PRIMARY KEY(activity_id,revision)
);
CREATE TABLE notification_activity_scans (
 id INTEGER PRIMARY KEY,
 source_id TEXT NOT NULL REFERENCES notification_sources(id),
 subject_url TEXT NOT NULL,
 started_at TEXT NOT NULL,
 complete INTEGER NOT NULL,
 manifest TEXT NOT NULL CHECK(json_valid(manifest)),
 history_ceiling INTEGER NOT NULL,
 processed_batch_id TEXT REFERENCES notification_batches(id)
);
CREATE INDEX notification_activities_subject ON notification_activities(source_id,subject_url);
-- +goose Down
ALTER TABLE notification_batches DROP COLUMN activity_metrics;
ALTER TABLE notification_events DROP COLUMN force_replay;
DROP TABLE notification_activity_scans;
DROP TABLE notification_activity_versions;
DROP TABLE notification_activities;
DROP TABLE notification_activity_subjects;

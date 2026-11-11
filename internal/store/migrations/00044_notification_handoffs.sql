-- +goose Up
CREATE TABLE notification_handoffs (
 id INTEGER PRIMARY KEY,
 run_id TEXT NOT NULL REFERENCES notification_runs(id) ON DELETE CASCADE,
 step_key TEXT NOT NULL,
 task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 session_id TEXT NOT NULL,
 instruction TEXT NOT NULL,
 blocker TEXT NOT NULL,
 initial_evidence TEXT NOT NULL DEFAULT '',
 checked_at INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL DEFAULT 'blocked',
 fingerprint TEXT NOT NULL DEFAULT '',
 applied_fingerprint TEXT NOT NULL DEFAULT '',
 changed_at INTEGER NOT NULL DEFAULT 0,
 assessed_at INTEGER NOT NULL DEFAULT 0,
 assessment TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 UNIQUE(run_id,step_key)
);
CREATE TABLE notification_handoff_assessments (
 handoff_id INTEGER NOT NULL REFERENCES notification_handoffs(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'running',
 result TEXT NOT NULL DEFAULT '',
 evidence TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 PRIMARY KEY(handoff_id,fingerprint)
);
-- +goose Down
DROP TABLE notification_handoff_assessments;
DROP TABLE notification_handoffs;

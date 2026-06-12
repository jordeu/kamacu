-- +goose Up
-- tmux session identity (NEVER status -- tmux itself is the status authority,
-- probed via has-session). Rows are written by the spawn handler BEFORE the
-- spawn (reserving n under the UNIQUE constraints) and are NOT deleted on
-- kill in Phase 8: MAX(n)+1 stays monotonic across restarts so names never
-- collide with surviving tmux sessions; Phase 9's lazy GC owns row cleanup.
CREATE TABLE tmux_sessions (
    id         INTEGER PRIMARY KEY,
    task_id    INTEGER NOT NULL REFERENCES tasks(id),
    n          INTEGER NOT NULL,
    name       TEXT    NOT NULL UNIQUE,
    label      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(task_id, n)
);
-- +goose Down
DROP TABLE tmux_sessions;

-- +goose Up
-- Per-file "Viewed" state for the GitHub-style diff review (DIFF-03/04). This is
-- a KEEP-HISTORY table: one row per (task_id, file_path, diff_hash), where
-- diff_hash is the rendered-diff fingerprint from internal/diff (Plan 22-01). A
-- file reads "viewed" iff a row exists at its CURRENT rendered hash, so:
--   * changing a file mints a new hash with no matching row -> auto-resets to
--     un-viewed (DIFF-04), while the old-hash row is deliberately kept (D-02);
--   * reverting a file to a previously-viewed byte-identical diff restores the
--     old hash -> its lingering row matches again -> the checkmark returns.
-- The composite PRIMARY KEY enforces keep-history uniqueness AND doubles as the
-- read index (leftmost-prefix task_id) -- no separate secondary index is needed.
-- The task FK cascades on delete, pruning every row when the owning task is gone.
CREATE TABLE diff_viewed (
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    file_path  TEXT    NOT NULL,
    diff_hash  TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (task_id, file_path, diff_hash)
);

-- +goose Down
DROP TABLE diff_viewed;

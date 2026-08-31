-- +goose Up
-- v1.13 (GDATA-01): the global_task singleton -- the storage contract for the
-- task-less "global scope" Scratchpad (D-09 label / D-11 internal naming).
-- One row, id=1, forever: CHECK (id = 1) makes a second row unrepresentable,
-- and agent_id -> agents(id) ON DELETE RESTRICT keeps the referenced agent
-- alive behind the handler-level 409 guard (the projects.agent_id pattern).
--
-- The seed runs in-migration because every later v1.13 phase reads the row
-- unconditionally: Phase 14's config API GETs it, Phase 15's spawn path
-- writes resume ids into it. agent_id is seeded from the is_default agent --
-- NEVER a hardcoded id (13-RESEARCH Pitfall 4: installs may hold several
-- agents and POST /api/agents/{id}/default moves the flag). A boot backfill
-- (13-02) re-arms the row if a hand-DELETE ever drops it.
--
-- Storage contract for the root (written by Phase 14, only baked here):
--   root_path = '' (unset) | folder root | the managed clone path, whose
--   format is ~/.kamacu/repos/global/<owner>/<name> (D-02 -- `global` is a
--   reserved pseudo-owner inside the existing repos/ tree, structurally
--   unremovable by a project's gated delete, D-01).
--   github_repo NULL => root_path is a folder root; NOT NULL => managed clone.
CREATE TABLE global_task (
  id                  INTEGER PRIMARY KEY CHECK (id = 1),
  root_path           TEXT NOT NULL DEFAULT '',
  github_repo         TEXT,
  agent_id            INTEGER NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
  claude_session_id   TEXT,
  opencode_session_id TEXT,
  created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- Seed from the default agent (is_default = 1), never a literal id. 00013's
-- hardcoded DEFAULT was safe only because its own seed created id 1 in the
-- same migration; here the flag may point anywhere.
INSERT INTO global_task (id, agent_id)
  SELECT 1, id FROM agents WHERE is_default = 1;

-- +goose Down
-- Plain transaction: nothing references global_task yet, so a bare DROP is
-- safe -- none of the ALTER/foreign_keys restrictions apply.
DROP TABLE global_task;

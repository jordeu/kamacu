-- +goose NO TRANSACTION
-- +goose Up
-- SQLite requires foreign_keys off when adding a reference with a non-null default.
PRAGMA foreign_keys = OFF;
BEGIN;
ALTER TABLE projects RENAME COLUMN agent_id TO previous_agent_id;
ALTER TABLE projects ADD COLUMN agent_id INTEGER DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT;
UPDATE projects SET agent_id = previous_agent_id;
ALTER TABLE projects DROP COLUMN previous_agent_id;

ALTER TABLE workspaces ADD COLUMN default_agent_id INTEGER REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE tasks ADD COLUMN workspace_agent_id INTEGER REFERENCES agents(id) ON DELETE RESTRICT;
CREATE INDEX idx_workspaces_default_agent_id ON workspaces(default_agent_id);
CREATE INDEX idx_tasks_workspace_agent_id ON tasks(workspace_agent_id);

CREATE VIEW workspace_agent_defaults AS
SELECT id AS workspace_id,
       COALESCE(default_agent_id,
                (SELECT id FROM agents WHERE is_default = 1),
                (SELECT id FROM agents WHERE engine = 'claude' AND is_system = 1)) AS agent_id
FROM workspaces;

CREATE VIEW task_agents AS
SELECT t.id AS task_id,
       COALESCE(p.agent_id, t.workspace_agent_id, w.agent_id) AS default_agent_id,
       COALESCE(t.agent_id, p.agent_id, t.workspace_agent_id, w.agent_id) AS agent_id
FROM tasks t
JOIN projects p ON p.id = t.project_id
JOIN workspace_agent_defaults w ON w.workspace_id = p.workspace_id;

-- Keep the previous default on existing tasks when a project opts in.
-- +goose StatementBegin
CREATE TRIGGER preserve_task_agent_on_workspace_inheritance
BEFORE UPDATE OF agent_id ON projects
WHEN OLD.agent_id IS NOT NULL AND NEW.agent_id IS NULL
BEGIN
  UPDATE tasks SET workspace_agent_id = OLD.agent_id
  WHERE project_id = OLD.id;
END;
-- +goose StatementEnd
COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;
BEGIN;
UPDATE tasks SET agent_id = workspace_agent_id
WHERE agent_id IS NULL AND workspace_agent_id IS NOT NULL
  AND project_id IN (SELECT id FROM projects WHERE agent_id IS NULL);
DROP TRIGGER preserve_task_agent_on_workspace_inheritance;
UPDATE projects
SET agent_id = (SELECT agent_id FROM workspace_agent_defaults WHERE workspace_id = projects.workspace_id)
WHERE agent_id IS NULL;
DROP VIEW task_agents;
DROP VIEW workspace_agent_defaults;
DROP INDEX idx_tasks_workspace_agent_id;
DROP INDEX idx_workspaces_default_agent_id;
ALTER TABLE tasks DROP COLUMN workspace_agent_id;
ALTER TABLE workspaces DROP COLUMN default_agent_id;

ALTER TABLE projects RENAME COLUMN agent_id TO previous_agent_id;
ALTER TABLE projects ADD COLUMN agent_id INTEGER NOT NULL DEFAULT 1 REFERENCES agents(id) ON DELETE RESTRICT;
UPDATE projects SET agent_id = previous_agent_id;
ALTER TABLE projects DROP COLUMN previous_agent_id;
COMMIT;
PRAGMA foreign_keys = ON;

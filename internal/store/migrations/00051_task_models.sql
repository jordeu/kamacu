-- +goose Up
ALTER TABLE agents ADD COLUMN models TEXT NOT NULL DEFAULT '[]';
ALTER TABLE agents ADD COLUMN model_argument TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN model_discovery_command TEXT NOT NULL DEFAULT '';
UPDATE agents SET model_argument = '--model {{model}}' WHERE engine IN ('claude', 'codex', 'opencode');
ALTER TABLE tasks ADD COLUMN agent_id INTEGER REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE tasks ADD COLUMN model TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_tasks_agent_id ON tasks(agent_id);

-- +goose Down
DROP INDEX idx_tasks_agent_id;
ALTER TABLE tasks DROP COLUMN model;
ALTER TABLE tasks DROP COLUMN agent_id;
ALTER TABLE agents DROP COLUMN model_discovery_command;
ALTER TABLE agents DROP COLUMN model_argument;
ALTER TABLE agents DROP COLUMN models;

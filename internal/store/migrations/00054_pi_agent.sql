-- +goose Up
ALTER TABLE tasks ADD COLUMN pi_session_id TEXT;
ALTER TABLE global_task ADD COLUMN pi_session_id TEXT;

INSERT INTO agents (name, command, engine, is_system, model_argument)
SELECT candidate, 'pi', 'pi', 1, '--model {{model}}'
FROM (
    SELECT 'Pi' AS candidate, 0 AS preference
    UNION ALL SELECT 'Pi (built-in)', 1
    UNION ALL SELECT 'Pi (built-in ' || id || ')', id + 1 FROM agents
    UNION ALL SELECT 'Pi (built-in ' || (COALESCE(MAX(id), 0) + 1) || ')', COALESCE(MAX(id), 0) + 2 FROM agents
)
WHERE NOT EXISTS (SELECT 1 FROM agents WHERE name = candidate COLLATE NOCASE)
  AND NOT EXISTS (SELECT 1 FROM agents WHERE engine = 'pi' AND is_system = 1)
ORDER BY preference LIMIT 1;

-- +goose Down
UPDATE agents SET engine = 'custom', is_system = 0 WHERE engine = 'pi' AND is_system = 1;
ALTER TABLE global_task DROP COLUMN pi_session_id;
ALTER TABLE tasks DROP COLUMN pi_session_id;

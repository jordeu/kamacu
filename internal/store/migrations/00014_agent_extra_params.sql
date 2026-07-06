-- +goose Up
-- M001 gate follow-up: relocate claude's extra-params from a global setting
-- (agent_extra_params) onto the agents row, so the Claude agent is configured
-- wholly in its edit dialog (Name + Command + Extra parameters) rather than
-- split across a global Settings section. Custom agents ignore the column
-- (their flags live in the command template).
--
-- DEFAULT '' so the column is NOT NULL without baking the code default
-- ('--dangerously-skip-permissions', AGENT-02) into the schema. The effective
-- value (code default OR user override) is copied into the claude seed row by
-- the BackfillAgentExtraParams startup hook (mirroring BackfillAgents), so no
-- one loses their config or the default. The hook is idempotent and a no-op
-- once the seed row carries a non-empty value.
ALTER TABLE agents ADD COLUMN extra_params TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agents DROP COLUMN extra_params;

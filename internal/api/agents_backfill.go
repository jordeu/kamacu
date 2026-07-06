package api

import (
	"database/sql"
	"errors"
)

// BackfillAgents guarantees the AGENTDATA invariant at startup, mirroring
// api.BackfillWorkspaces: it runs ONCE right after store.Migrate(db) and is
// IDEMPOTENT -- a cheap no-op on healthy boots.
//
// Like workspaces, migration 00013 already creates the default Claude Code
// agent AND assigns every project to it via the projects.agent_id NOT NULL
// DEFAULT 1 FK column. So this hook's active job is only the safety net:
// guarantee a default agent row exists (at least one agent always exists, and
// at least one is_default). Under NOT NULL + FK + DEFAULT no project can be
// agent-less, so there is deliberately NO SELECT->UPDATE reassignment loop
// over projects -- nothing to reassign.
//
// All SQL is parameterless/literal (a SELECT and a literal claude-seed INSERT)
// -- no string-concatenated input, mirroring T-25-01.
func BackfillAgents(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM agents WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	// No default agent: re-create the Claude seed (defensive; 00013 normally
	// created it). engine='claude' is the full capability tier (hooks/resume/
	// quota); is_system=1 makes it non-deletable; is_default=1 satisfies the
	// exactly-one-default invariant.
	_, err = db.Exec(`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('Claude Code', 'claude', 'claude', 1, 1)`)
	return err
}

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

// BackfillAgentExtraParams is the M001 gate-follow-up one-shot: it copies the
// effective value of the legacy global `agent_extra_params` setting (code
// default '--dangerously-skip-permissions' OR the user's override) into the
// Claude seed row's extra_params column, so the relocation from a global setting
// onto the agent row loses no one's config or the default. Idempotent: a no-op
// once the seed row already carries a non-empty value (the user may have
// intentionally cleared it to '' — preserved by the non-empty guard on the
// *seed*, while the very first run copies the effective setting).
//
// Runs after store.Migrate (the column must exist) and after BackfillAgents
// (the seed row must exist). Mirrors the BackfillAgents posture.
func BackfillAgentExtraParams(db *sql.DB) error {
	// If the claude seed already has a non-empty extra_params, leave it (the
	// user configured it via the edit dialog, or a prior backfill ran).
	var cur string
	err := db.QueryRow(`SELECT extra_params FROM agents WHERE engine = 'claude' AND is_system = 1`).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // no claude seed yet (BackfillAgents hasn't run); safe no-op
	}
	if err != nil {
		return err
	}
	if cur != "" {
		return nil // already populated — never overwrite a configured value
	}
	// Copy the effective setting value (settings.Get returns the code default
	// when the KV row is absent, so the AGENT-02 default survives).
	effective, err := effectiveExtraParams(db)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE agents SET extra_params = ? WHERE engine = 'claude' AND is_system = 1 AND extra_params = ''`, effective)
	return err
}

// BackfillOpenCodeAgent guarantees the opencode system seed (M002 D013) exists
// at startup, mirroring BackfillAgents: it runs ONCE right after store.Migrate(db)
// and is IDEMPOTENT -- a cheap no-op on healthy boots. The seed is created by
// migration 00015 (guarded by a NOT EXISTS predicate), so this hook's job is the
// same safety net posture as BackfillAgents: re-create the row if it was somehow
// dropped from a migrated DB (e.g. a partial-apply recovery, or a user who
// manually deleted it past the is_system guard).
//
// The seed is engine='opencode' (its own capability tier: PTY spawn + on-disk
// plugin status hooks, see internal/opencode/), command='opencode' (the host CLI),
// is_default=0 (claude REMAINS the sole default -- R019 invariant intact), and
// is_system=1 (non-deletable, R018). Mirrors the BackfillAgents literal-INSERT
// posture: all SQL is parameterless/literal -- no string-concatenated input.
func BackfillOpenCodeAgent(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM agents WHERE engine = 'opencode' AND is_system = 1`).Scan(&id)
	if err == nil {
		return nil // opencode seed already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	// No opencode seed: re-create it (defensive; 00015 normally created it).
	// is_default=0 preserves the exactly-one-default invariant (claude stays it).
	_, err = db.Exec(`INSERT INTO agents (name, command, engine, is_default, is_system) VALUES ('opencode', 'opencode', 'opencode', 0, 1)`)
	return err
}

// effectiveExtraParams returns the legacy setting's effective value without
// importing the settings package (agents_backfill.go stays leaf-ish). The code
// default ('--dangerously-skip-permissions', AGENT-02) is applied when the KV
// row is absent — mirroring settings.Get's semantics for this one key.
func effectiveExtraParams(db *sql.DB) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = 'agent_extra_params'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "--dangerously-skip-permissions", nil // AGENT-02 code default
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

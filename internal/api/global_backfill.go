package api

import (
	"database/sql"
	"errors"
)

// BackfillGlobalTask guarantees the GDATA-01 invariant at startup, mirroring
// api.BackfillAgents: it runs ONCE right after store.Migrate(db) and the other
// backfills and is IDEMPOTENT -- a cheap no-op on healthy boots.
//
// Like agents, migration 00017 already creates AND seeds the global_task
// singleton (id=1, agent_id from the is_default agent). But a migration runs
// exactly once: a hand-deleted row would stay missing forever, while every
// later v1.13 phase reads the row unconditionally (Phase 14's config API GETs
// it, Phase 15's spawn path writes resume ids into it). This hook's job is the
// safety net: guarantee the singleton exists on every boot.
//
// Wiring order in serve.go is LOAD-BEARING: it must run AFTER BackfillAgents /
// BackfillOpenCodeAgent, because the re-insert seed reads the default agent
// (13-RESEARCH Pitfall 6 -- wiring it before BackfillAgents would garble boot
// on an agents-wiped install).
//
// All SQL is parameterless/literal -- no string-concatenated input, mirroring
// the BackfillAgents leaf posture.
func BackfillGlobalTask(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM global_task WHERE id = 1`).Scan(&id)
	if err == nil {
		return nil // singleton already present -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	// Hand-deleted singleton: re-insert id=1 seeded from the default agent --
	// the same never-hardcode-the-agent-id rule as migration 00017 (Pitfall 4:
	// the is_default flag is movable on real installs).
	_, err = db.Exec(`INSERT INTO global_task (id, agent_id) SELECT 1, id FROM agents WHERE is_default = 1`)
	return err
}

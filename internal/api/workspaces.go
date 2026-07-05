package api

import (
	"database/sql"
	"errors"
)

// BackfillWorkspaces guarantees the WSDATA-02 invariant at startup, mirroring
// api.BackfillProjectIcons: it runs ONCE right after store.Migrate(db) and is
// IDEMPOTENT -- a cheap no-op on healthy boots.
//
// Unlike the icon backfill (which FILLS blank columns via a SELECT->UPDATE loop),
// migration 00012 already creates the default Personal workspace AND assigns every
// project to it via the projects.workspace_id NOT NULL DEFAULT 1 FK column. So this
// hook's active job is only the safety net: guarantee a default workspace row
// exists (WSMGMT-04: at least one workspace always exists). Under NOT NULL + FK +
// DEFAULT no project can be workspace-less, so there is deliberately NO
// SELECT->UPDATE reassignment loop over projects -- nothing to reassign.
//
// This is the LEAN reinterpretation of D-07's "collect-then-update" mechanism,
// user-confirmed 2026-07-05 via /gsd:plan-phase (see the D-07 note in 25-03-PLAN.md):
// both forms meet D-07's stated job identically with no data risk. All SQL is
// parameterless/literal (only a SELECT and a literal 'Personal' INSERT) -- no
// string-concatenated input (T-25-01).
func BackfillWorkspaces(db *sql.DB) error {
	var id int64
	err := db.QueryRow(`SELECT id FROM workspaces WHERE is_default = 1`).Scan(&id)
	if err == nil {
		return nil // default already exists -> no-op (the healthy-boot path)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err // a real DB error is propagated, never swallowed
	}
	// No default workspace: re-create Personal (defensive; 00012 normally created it).
	_, err = db.Exec(`INSERT INTO workspaces (name, is_default) VALUES ('Personal', 1)`)
	return err
}

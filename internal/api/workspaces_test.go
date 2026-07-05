package api

import (
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

// TestBackfillWorkspaces proves the three BackfillWorkspaces behaviors on a
// migrated DB (store.Open + store.Migrate harness):
//
//	(1) no-op on a healthy boot where migration 00012 already created the default
//	    Personal workspace (never a second Personal);
//	(2) re-create Personal when the is_default=1 row is missing;
//	(3) idempotency on a double run (still exactly one workspace).
//
// This is the LEAN invariant guard, not a project-reassign loop: migration 00012's
// NOT NULL DEFAULT 1 FK already assigns every project, so there is nothing to
// backfill on projects (see the D-07 reinterpretation note in 25-03-PLAN.md,
// user-confirmed 2026-07-05).
func TestBackfillWorkspaces(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	countWorkspaces := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&n); err != nil {
			t.Fatalf("count workspaces: %v", err)
		}
		return n
	}
	countDefaults := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE is_default = 1`).Scan(&n); err != nil {
			t.Fatalf("count default workspaces: %v", err)
		}
		return n
	}

	// (1) Healthy boot: migration 00012 already created Personal, so
	// BackfillWorkspaces is a no-op and does NOT insert a second workspace.
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after migrate: workspaces = %d, want 1 (migration seeds Personal)", got)
	}
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (healthy boot): %v", err)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after no-op backfill: workspaces = %d, want 1 (no second Personal)", got)
	}

	// (2) Missing default: with no referencing projects the ON DELETE RESTRICT FK
	// permits dropping the default row, leaving the table empty; BackfillWorkspaces
	// re-creates Personal (is_default=1).
	if _, err := db.Exec(`DELETE FROM workspaces WHERE is_default = 1`); err != nil {
		t.Fatalf("delete default workspace: %v", err)
	}
	if got := countWorkspaces(); got != 0 {
		t.Fatalf("after delete: workspaces = %d, want 0", got)
	}
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (missing default): %v", err)
	}
	if got := countDefaults(); got != 1 {
		t.Fatalf("after re-create: default workspaces = %d, want 1", got)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after re-create: workspaces = %d, want 1", got)
	}

	// (3) Idempotent double run: calling again keeps exactly one workspace
	// (no second Personal).
	if err := BackfillWorkspaces(db); err != nil {
		t.Fatalf("BackfillWorkspaces (double run): %v", err)
	}
	if got := countWorkspaces(); got != 1 {
		t.Fatalf("after double run: workspaces = %d, want 1 (idempotent)", got)
	}
}

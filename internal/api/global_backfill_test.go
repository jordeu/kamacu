package api

import (
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

// TestBackfillGlobalTask proves the GDATA-01 singleton safety net is
// idempotent and re-inserts id=1 — seeded from the is_default agent — only
// when a hand-DELETE dropped the row. Mirrors TestBackfillAgents exactly.
func TestBackfillGlobalTask(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	countSingletons := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM global_task`).Scan(&n); err != nil {
			t.Fatalf("count global_task rows: %v", err)
		}
		return n
	}

	// (1) Healthy boot: migration 00017 already seeded the singleton, so
	// BackfillGlobalTask is a no-op and does NOT insert a second row.
	if got := countSingletons(); got != 1 {
		t.Fatalf("after migrate: global_task rows = %d, want 1 (migration seeds it)", got)
	}
	if err := BackfillGlobalTask(db); err != nil {
		t.Fatalf("BackfillGlobalTask (healthy boot): %v", err)
	}
	if got := countSingletons(); got != 1 {
		t.Fatalf("after no-op backfill: global_task rows = %d, want 1 (no second row)", got)
	}

	// (2) Hand-deleted row: the backfill re-inserts exactly one row with
	//     id=1 and agent_id = the is_default agent. Asserted via SQL
	//     comparison — NEVER a hardcoded agent id (the default flag is
	//     movable on real installs, 13-RESEARCH Pitfall 4).
	if _, err := db.Exec(`DELETE FROM global_task`); err != nil {
		t.Fatalf("delete global_task row: %v", err)
	}
	if got := countSingletons(); got != 0 {
		t.Fatalf("after delete: global_task rows = %d, want 0", got)
	}
	if err := BackfillGlobalTask(db); err != nil {
		t.Fatalf("BackfillGlobalTask (missing row): %v", err)
	}
	if got := countSingletons(); got != 1 {
		t.Fatalf("after re-insert: global_task rows = %d, want 1", got)
	}
	var seeded int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM global_task WHERE id = 1 AND agent_id = (SELECT id FROM agents WHERE is_default = 1)`,
	).Scan(&seeded); err != nil {
		t.Fatalf("seed-shape comparison: %v", err)
	}
	if seeded != 1 {
		t.Fatal("re-inserted row must be id=1 seeded from the is_default agent")
	}

	// (3) Idempotent on a second call after re-insert — still exactly one row.
	if err := BackfillGlobalTask(db); err != nil {
		t.Fatalf("BackfillGlobalTask (second call): %v", err)
	}
	if got := countSingletons(); got != 1 {
		t.Errorf("after second backfill: global_task rows = %d, want 1", got)
	}
}

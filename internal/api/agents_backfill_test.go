package api

import (
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

// TestBackfillAgents proves the AGENTDATA invariant hook is idempotent and
// re-creates the Claude seed only when the default is gone. Mirrors
// TestBackfillWorkspaces exactly.
func TestBackfillAgents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	countAgents := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&n); err != nil {
			t.Fatalf("count agents: %v", err)
		}
		return n
	}
	countDefaults := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE is_default = 1`).Scan(&n); err != nil {
			t.Fatalf("count default agents: %v", err)
		}
		return n
	}

	// (1) Healthy boot: migration 00013 already created the Claude seed, so
	// BackfillAgents is a no-op and does NOT insert a second agent.
	if got := countAgents(); got != 1 {
		t.Fatalf("after migrate: agents = %d, want 1 (migration seeds Claude)", got)
	}
	if err := BackfillAgents(db); err != nil {
		t.Fatalf("BackfillAgents (healthy boot): %v", err)
	}
	if got := countAgents(); got != 1 {
		t.Fatalf("after no-op backfill: agents = %d, want 1 (no second Claude)", got)
	}

	// (2) Missing default: with no referencing projects the ON DELETE RESTRICT FK
	// permits dropping the default row, leaving the table empty; BackfillAgents
	// re-creates the Claude seed (is_default=1, is_system=1, engine='claude').
	if _, err := db.Exec(`DELETE FROM agents WHERE is_default = 1`); err != nil {
		t.Fatalf("delete default agent: %v", err)
	}
	if got := countAgents(); got != 0 {
		t.Fatalf("after delete: agents = %d, want 0", got)
	}
	if err := BackfillAgents(db); err != nil {
		t.Fatalf("BackfillAgents (missing default): %v", err)
	}
	if got := countDefaults(); got != 1 {
		t.Fatalf("after re-create: default agents = %d, want 1", got)
	}

	// (3) The re-created seed carries the claude engine + system marker.
	var engine string
	var isSystem int
	if err := db.QueryRow(`SELECT engine, is_system FROM agents WHERE is_default = 1`).Scan(&engine, &isSystem); err != nil {
		t.Fatalf("select re-created seed: %v", err)
	}
	if engine != "claude" {
		t.Errorf("re-created seed engine = %q, want \"claude\"", engine)
	}
	if isSystem != 1 {
		t.Errorf("re-created seed is_system = %d, want 1", isSystem)
	}

	// (4) Idempotent on a second call after re-create -- still exactly one.
	if err := BackfillAgents(db); err != nil {
		t.Fatalf("BackfillAgents (second call): %v", err)
	}
	if got := countAgents(); got != 1 {
		t.Errorf("after second backfill: agents = %d, want 1", got)
	}
}

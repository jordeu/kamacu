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

	countClaudeAgents := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'claude'`).Scan(&n); err != nil {
			t.Fatalf("count claude agents: %v", err)
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
	// BackfillAgents is a no-op and does NOT insert a second claude row. (Counts
	// the CLAUDE seed specifically -- what BackfillAgents owns -- not the total
	// agent count, which M002 migration 00015 legitimately raises to 2 by adding
	// the opt-in opencode system seed.)
	if got := countClaudeAgents(); got != 1 {
		t.Fatalf("after migrate: claude agents = %d, want 1 (migration seeds Claude)", got)
	}
	if err := BackfillAgents(db); err != nil {
		t.Fatalf("BackfillAgents (healthy boot): %v", err)
	}
	if got := countClaudeAgents(); got != 1 {
		t.Fatalf("after no-op backfill: claude agents = %d, want 1 (no second Claude)", got)
	}

	// (2) Missing default: with no referencing projects the ON DELETE RESTRICT FK
	// permits dropping the default row, leaving the table without the Claude seed;
	// BackfillAgents re-creates the Claude seed (is_default=1, is_system=1,
	// engine='claude'). The opencode system seed (M002) is untouched by this hook.
	// v1.13 ordering: the global_task singleton (00017) REFERENCES the default
	// agent ON DELETE RESTRICT, so a hand-wiped-agents install must drop the
	// singleton FIRST — the raw agent DELETE is (correctly) refused otherwise.
	// BackfillGlobalTask re-arms the singleton on the next boot (proven separately
	// in TestBackfillGlobalTask).
	if _, err := db.Exec(`DELETE FROM global_task`); err != nil {
		t.Fatalf("delete global_task singleton: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM agents WHERE is_default = 1`); err != nil {
		t.Fatalf("delete default agent: %v", err)
	}
	if got := countClaudeAgents(); got != 0 {
		t.Fatalf("after delete: claude agents = %d, want 0", got)
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

	// (4) Idempotent on a second call after re-create -- still exactly one
	//    claude seed.
	if err := BackfillAgents(db); err != nil {
		t.Fatalf("BackfillAgents (second call): %v", err)
	}
	if got := countClaudeAgents(); got != 1 {
		t.Errorf("after second backfill: claude agents = %d, want 1", got)
	}
}

// TestBackfillOpenCodeAgent proves the M002 opencode safety-net hook (the
// in-process counterpart to migration 00015) is idempotent and re-creates the
// opencode seed only when it is gone. Mirrors TestBackfillAgents exactly,
// adapted to the opencode seed's opt-in shape (is_default=0, engine='opencode').
func TestBackfillOpenCodeAgent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	countOpencode := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'opencode' AND is_system = 1`).Scan(&n); err != nil {
			t.Fatalf("count opencode agents: %v", err)
		}
		return n
	}

	// (1) Healthy boot: migration 00015 already created the opencode seed, so
	// BackfillOpenCodeAgent is a no-op and does NOT insert a second opencode row.
	if got := countOpencode(); got != 1 {
		t.Fatalf("after migrate: opencode agents = %d, want 1 (migration seeds it)", got)
	}
	if err := BackfillOpenCodeAgent(db); err != nil {
		t.Fatalf("BackfillOpenCodeAgent (healthy boot): %v", err)
	}
	if got := countOpencode(); got != 1 {
		t.Fatalf("after no-op backfill: opencode agents = %d, want 1 (no second opencode)", got)
	}

	// (2) Missing seed: simulate a recovery where the opencode row was dropped
	// (partial-apply / manual delete past the is_system guard). The backfill hook
	// re-creates it (is_default=0, is_system=1, engine='opencode').
	if _, err := db.Exec(`DELETE FROM agents WHERE engine = 'opencode' AND is_system = 1`); err != nil {
		t.Fatalf("delete opencode agent: %v", err)
	}
	if got := countOpencode(); got != 0 {
		t.Fatalf("after delete: opencode agents = %d, want 0", got)
	}
	if err := BackfillOpenCodeAgent(db); err != nil {
		t.Fatalf("BackfillOpenCodeAgent (missing seed): %v", err)
	}
	if got := countOpencode(); got != 1 {
		t.Fatalf("after re-create: opencode agents = %d, want 1", got)
	}

	// (3) The re-created seed carries the opencode engine + system marker and is
	//     NOT a default (R019: claude stays the sole default). No user-created row
	//     is ever clobbered by the backfill (it keys on engine='opencode' AND
	//     is_system=1, which a user-created row can never match).
	var ocName, ocCommand, ocEngine string
	var ocIsDefault, ocIsSystem int
	if err := db.QueryRow(
		`SELECT name, command, engine, is_default, is_system FROM agents WHERE engine = 'opencode' AND is_system = 1`,
	).Scan(&ocName, &ocCommand, &ocEngine, &ocIsDefault, &ocIsSystem); err != nil {
		t.Fatalf("select re-created opencode seed: %v", err)
	}
	if ocName != "OpenCode" || ocCommand != "opencode" || ocEngine != "opencode" {
		t.Errorf("re-created opencode seed = %q/%q/%q, want OpenCode/opencode/opencode", ocName, ocCommand, ocEngine)
	}
	if ocIsSystem != 1 {
		t.Errorf("re-created opencode seed is_system = %d, want 1", ocIsSystem)
	}
	if ocIsDefault != 0 {
		t.Errorf("re-created opencode seed is_default = %d, want 0 (claude sole default, R019)", ocIsDefault)
	}

	// (4) Idempotent on a second call after re-create -- still exactly one.
	if err := BackfillOpenCodeAgent(db); err != nil {
		t.Fatalf("BackfillOpenCodeAgent (second call): %v", err)
	}
	if got := countOpencode(); got != 1 {
		t.Errorf("after second backfill: opencode agents = %d, want 1", got)
	}
}

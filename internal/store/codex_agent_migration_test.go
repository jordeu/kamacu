package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestCodexAgentMigration proves the codex-seed invariants of migration
// 00019_codex_agent.sql against the REAL goose runner: it stages the DB at
// the pre-00019 schema (migrations up to 00018, so the Claude and OpenCode
// seeds already exist), THEN applies 00019 and asserts the codex system agent
// row exists with the exact seed shape AND that the R019 invariant (claude
// remains the SOLE is_default=1 row) is intact — plus idempotent re-run.
// Mirrors TestOpencodeAgentMigration.
func TestCodexAgentMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00019 (migrations up to 00018): the "existing
	// install" shape before the codex upgrade — claude + opencode seeds, no codex.
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 18); err != nil {
		t.Fatalf("UpTo(18): %v", err)
	}

	// Sanity: no codex row yet (we are truly pre-00019), and the seeds 00019
	// layers on top of are present.
	var pre int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'codex'`).Scan(&pre); err != nil {
		t.Fatalf("pre-check codex row: %v", err)
	}
	if pre != 0 {
		t.Fatalf("codex agent exists before 00019 (staging failed)")
	}
	for _, engine := range []string{"claude", "opencode"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = ?`, engine).Scan(&n); err != nil {
			t.Fatalf("pre-check %s row: %v", engine, err)
		}
		if n != 1 {
			t.Fatalf("%s seed count before 00019 = %d, want 1", engine, n)
		}
	}

	// Apply 00019 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00019): %v", err)
	}

	// (a) The codex system seed row now exists with the exact seed shape:
	//     name='Codex', command='codex', engine='codex', is_system=1,
	//     extra_params='' (codex ignores the column), is_default=0 (R019).
	var (
		cxName      string
		cxCommand   string
		cxEngine    string
		cxIsDefault int
		cxIsSystem  int
		cxExtra     string
	)
	err = db.QueryRow(
		`SELECT name, command, engine, is_default, is_system, extra_params FROM agents WHERE engine = 'codex'`,
	).Scan(&cxName, &cxCommand, &cxEngine, &cxIsDefault, &cxIsSystem, &cxExtra)
	if err != nil {
		t.Fatalf("select codex seed: %v", err)
	}
	if cxName != "Codex" {
		t.Errorf("codex seed name = %q, want \"Codex\"", cxName)
	}
	if cxCommand != "codex" {
		t.Errorf("codex seed command = %q, want \"codex\" (the host codex CLI)", cxCommand)
	}
	if cxEngine != "codex" {
		t.Errorf("codex seed engine = %q, want \"codex\"", cxEngine)
	}
	if cxIsDefault != 0 {
		t.Errorf("codex seed is_default = %d, want 0 (claude stays the sole default, R019)", cxIsDefault)
	}
	if cxIsSystem != 1 {
		t.Errorf("codex seed is_system = %d, want 1 (non-deletable, R018)", cxIsSystem)
	}
	if cxExtra != "" {
		t.Errorf("codex seed extra_params = %q, want \"\" (codex ignores the column)", cxExtra)
	}

	// (b) R019 invariant: exactly one is_default row, and it is claude.
	var defEngine string
	var defCount int
	if err := db.QueryRow(`SELECT engine, COUNT(*) FROM agents WHERE is_default = 1`).Scan(&defEngine, &defCount); err != nil {
		t.Fatalf("select default agent: %v", err)
	}
	if defCount != 1 || defEngine != "claude" {
		t.Errorf("default agent after 00019 = (%q, %d), want (\"claude\", 1)", defEngine, defCount)
	}

	// (c) Idempotent: a re-run leaves exactly one codex row (NOT EXISTS guard).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate re-run: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'codex'`).Scan(&n); err != nil {
		t.Fatalf("count codex rows after re-run: %v", err)
	}
	if n != 1 {
		t.Errorf("codex rows after idempotent re-run = %d, want 1", n)
	}
}

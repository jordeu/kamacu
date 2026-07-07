package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestOpencodeAgentMigration proves the M002 opencode-seed invariants of
// migration 00015_opencode_agent.sql against the REAL goose runner: it stages
// the DB at the pre-00015 schema (migrations up to 00014, so the Claude seed
// from 00013 is already present), THEN applies 00015 and asserts the opencode
// system agent row exists with the exact seed shape AND that the R019 invariant
// (claude remains the SOLE is_default=1 row) is intact -- plus the structural
// guarantees (is_system=1 on both seeds, idempotent re-run, FK re-armed ON).
// Mirrors TestAgentsMigration.
func TestOpencodeAgentMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00015 (migrations up to 00014). This yields the
	// agents table with the Claude seed + extra_params column but NO opencode
	// row -- the real "existing install" shape before the M002 upgrade.
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 14); err != nil {
		t.Fatalf("UpTo(14): %v", err)
	}

	// Sanity: the opencode seed must NOT exist yet (we are truly pre-00015).
	var pre int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM agents WHERE engine = 'opencode'`,
	).Scan(&pre); err != nil {
		t.Fatalf("pre-check opencode row: %v", err)
	}
	if pre != 0 {
		t.Fatalf("opencode agent exists before 00015 (staging failed)")
	}
	// And claude is present (the 00013 seed) so 00015 runs against a populated table.
	var claudeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'claude'`).Scan(&claudeCount); err != nil {
		t.Fatalf("pre-check claude row: %v", err)
	}
	if claudeCount != 1 {
		t.Fatalf("claude seed count before 00015 = %d, want 1 (00013 seeded it)", claudeCount)
	}

	// Apply 00015 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00015): %v", err)
	}

	// (a) The opencode system seed row now exists with the exact seed shape:
	//     name='opencode', command='opencode', engine='opencode', is_system=1,
	//     extra_params='' (the 00014 default; opencode ignores the column), and
	//     is_default=0 (R019: claude stays the sole default).
	var (
		ocName       string
		ocCommand    string
		ocEngine     string
		ocIsDefault  int
		ocIsSystem   int
		ocExtra      string
	)
	err = db.QueryRow(
		`SELECT name, command, engine, is_default, is_system, extra_params FROM agents WHERE engine = 'opencode'`,
	).Scan(&ocName, &ocCommand, &ocEngine, &ocIsDefault, &ocIsSystem, &ocExtra)
	if err != nil {
		t.Fatalf("select opencode seed: %v", err)
	}
	if ocName != "opencode" {
		t.Errorf("opencode seed name = %q, want \"opencode\"", ocName)
	}
	if ocCommand != "opencode" {
		t.Errorf("opencode seed command = %q, want \"opencode\"", ocCommand)
	}
	if ocEngine != "opencode" {
		t.Errorf("opencode seed engine = %q, want \"opencode\"", ocEngine)
	}
	if ocIsSystem != 1 {
		t.Errorf("opencode seed is_system = %d, want 1 (non-deletable, R018)", ocIsSystem)
	}
	if ocExtra != "" {
		t.Errorf("opencode seed extra_params = %q, want \"\" (00014 default; opencode ignores it)", ocExtra)
	}

	// (b) R019 invariant: claude REMAINS the sole is_default=1 row. opencode is
	//     opt-in (is_default=0), so existing/new projects keep landing on claude
	//     byte-for-byte and no one is silently switched.
	if ocIsDefault != 0 {
		t.Errorf("opencode seed is_default = %d, want 0 (claude stays the default)", ocIsDefault)
	}
	var defaultCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE is_default = 1`).Scan(&defaultCount); err != nil {
		t.Fatalf("count default agents after 00015: %v", err)
	}
	if defaultCount != 1 {
		t.Errorf("is_default agent count after 00015 = %d, want 1 (claude sole default, R019)", defaultCount)
	}
	var defaultEngine string
	if err := db.QueryRow(`SELECT engine FROM agents WHERE is_default = 1`).Scan(&defaultEngine); err != nil {
		t.Fatalf("select default agent engine after 00015: %v", err)
	}
	if defaultEngine != "claude" {
		t.Errorf("default agent engine after 00015 = %q, want \"claude\" (R019)", defaultEngine)
	}

	// (c) Both system seeds (claude + opencode) are non-deletable. With no
	//     projects created in this test, deleting either still SUCCEEDS at the
	//     SQL layer (no FK references) -- the is_system=1 protection lives at
	//     the API handler, not the DB. So instead assert the is_system flag is
	//     present on BOTH, which is what the handler's block-on-is_system reads.
	var sysCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE is_system = 1`).Scan(&sysCount); err != nil {
		t.Fatalf("count system agents after 00015: %v", err)
	}
	if sysCount != 2 {
		t.Errorf("is_system agent count after 00015 = %d, want 2 (claude + opencode)", sysCount)
	}

	// (d) FK enforcement is re-armed ON on the same pooled handle (Pitfall 3:
	//     00013 toggled it OFF/ON outside a transaction; 00015 is a plain
	//     transactional INSERT and must not have left it off).
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
	}

	// (e) Idempotency: a second Migrate is a clean no-op -- still exactly two
	//     agents, exactly one opencode, exactly one default (claude). The 00015
	//     NOT EXISTS guard + the goose version table make this belt-and-braces.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var agCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&agCount); err != nil {
		t.Fatalf("count agents after re-run: %v", err)
	}
	if agCount != 2 {
		t.Errorf("agents after re-run = %d, want 2 (no second opencode)", agCount)
	}
	var ocCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE engine = 'opencode'`).Scan(&ocCount); err != nil {
		t.Fatalf("count opencode agents after re-run: %v", err)
	}
	if ocCount != 1 {
		t.Errorf("opencode agents after re-run = %d, want 1 (idempotent seed)", ocCount)
	}
}

// TestOpencodeAgentMigrationOnFreshDB proves the opencode seed lands on a
// brand-new install (all migrations from 00001), not just an upgrade path --
// closing the "fresh install also gets opencode" case that the staged test
// above cannot cover (it starts at 00014).
func TestOpencodeAgentMigrationOnFreshDB(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (fresh): %v", err)
	}

	// Fresh install yields BOTH seeds in one pass: claude (default, 00013) and
	// opencode (opt-in system, 00015).
	var ocEngine string
	var ocIsDefault int
	var ocIsSystem int
	err = db.QueryRow(
		`SELECT engine, is_default, is_system FROM agents WHERE engine = 'opencode'`,
	).Scan(&ocEngine, &ocIsDefault, &ocIsSystem)
	if err != nil {
		t.Fatalf("fresh install: select opencode seed: %v", err)
	}
	if ocEngine != "opencode" {
		t.Errorf("fresh install: opencode engine = %q, want \"opencode\"", ocEngine)
	}
	if ocIsDefault != 0 {
		t.Errorf("fresh install: opencode is_default = %d, want 0", ocIsDefault)
	}
	if ocIsSystem != 1 {
		t.Errorf("fresh install: opencode is_system = %d, want 1", ocIsSystem)
	}

	var defaultCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE is_default = 1`).Scan(&defaultCount); err != nil {
		t.Fatalf("fresh install: count defaults: %v", err)
	}
	if defaultCount != 1 {
		t.Errorf("fresh install: is_default count = %d, want 1 (claude sole default)", defaultCount)
	}
}

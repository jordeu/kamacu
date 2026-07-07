package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// columnInfo holds the row shape returned by PRAGMA table_info: cid, name,
// type, notnull, dflt_value, pk. We only assert name/type/notnull/dflt for the
// opencode_session_id column.
type columnInfo struct {
	name    string
	typ     string
	notnull int
	dflt    sql.NullString
	pk      int
}

// tasksColumnCount runs PRAGMA table_info(tasks) and returns how many columns
// match the given name, plus the full info row for the first match (zero value
// when count==0). Used to assert a column is absent pre-migration, present and
// exactly-once post-migration.
func tasksColumnCount(t *testing.T, db *sql.DB, name string) (int, columnInfo) {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(tasks)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(tasks): %v", err)
	}
	defer rows.Close()

	var (
		count int
		first columnInfo
	)
	for rows.Next() {
		var c columnInfo
		var cid int
		if err := rows.Scan(&cid, &c.name, &c.typ, &c.notnull, &c.dflt, &c.pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if c.name == name {
			count++
			if count == 1 {
				first = c
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return count, first
}

// TestOpencodeSessionIdMigration proves the nullable-column invariant of
// migration 00016_opencode_session_id.sql against the REAL goose runner: it
// stages the DB at the pre-00016 schema (migrations up to 00015, so the
// opencode seed from 00015 is already present), THEN applies 00016 and asserts
// tasks.opencode_session_id exists as a nullable TEXT column -- plus the
// structural guarantees (FK re-armed ON, idempotent re-run, fresh-install
// lands the column alongside claude_session_id). Mirrors the 00015
// migration-test posture (TestOpencodeAgentMigration) but for a pure ALTER
// TABLE ADD COLUMN instead of a seed INSERT.
func TestOpencodeSessionIdMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00016 (migrations up to 00015). This yields the
	// full tasks table including claude_session_id (00003) but NO
	// opencode_session_id -- the real "existing install" shape before this
	// M002/S03 upgrade.
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 15); err != nil {
		t.Fatalf("UpTo(15): %v", err)
	}

	// Sanity: the opencode_session_id column must NOT exist yet (truly
	// pre-00016), but claude_session_id (00003) DOES -- so 00016 runs against
	// the real populated tasks schema, not a degenerate one.
	preCount, _ := tasksColumnCount(t, db, "opencode_session_id")
	if preCount != 0 {
		t.Fatalf("opencode_session_id exists before 00016 (staging failed): count=%d", preCount)
	}
	claudeCount, _ := tasksColumnCount(t, db, "claude_session_id")
	if claudeCount != 1 {
		t.Fatalf("claude_session_id count before 00016 = %d, want 1 (00003 seeded it)", claudeCount)
	}

	// Apply 00016 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00016): %v", err)
	}

	// (a) The opencode_session_id column now exists, exactly once, as nullable
	//     TEXT with a NULL default -- no NOT NULL. This is the strategy-
	//     invariant data model: like claude_session_id, it is empty/NULL until
	//     the capture path (T03) writes the discovered ses_ id.
	ocCount, ocCol := tasksColumnCount(t, db, "opencode_session_id")
	if ocCount != 1 {
		t.Fatalf("opencode_session_id count after 00016 = %d, want 1", ocCount)
	}
	if ocCol.typ != "TEXT" {
		t.Errorf("opencode_session_id type = %q, want \"TEXT\"", ocCol.typ)
	}
	if ocCol.notnull != 0 {
		t.Errorf("opencode_session_id notnull = %d, want 0 (nullable; empty/NULL until T03 writes it)", ocCol.notnull)
	}
	if ocCol.dflt.Valid && ocCol.dflt.String != "" {
		t.Errorf("opencode_session_id dflt = %q, want NULL/empty (no default value)", ocCol.dflt.String)
	}
	// And the sibling column is still there unchanged.
	postClaudeCount, _ := tasksColumnCount(t, db, "claude_session_id")
	if postClaudeCount != 1 {
		t.Errorf("claude_session_id count after 00016 = %d, want 1 (unchanged)", postClaudeCount)
	}

	// (b) FK enforcement is re-armed ON on the same pooled handle. 00016 is a
	//     plain transactional ALTER TABLE ADD COLUMN (no PRAGMA dance like
	//     00013) and must not have left foreign_keys off.
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
	}

	// (c) Idempotency: a second Migrate is a clean no-op -- still exactly one
	//     opencode_session_id column. The goose version table makes this so;
	//     asserting it is belt-and-braces.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	rerunCount, _ := tasksColumnCount(t, db, "opencode_session_id")
	if rerunCount != 1 {
		t.Errorf("opencode_session_id count after re-run = %d, want 1 (idempotent)", rerunCount)
	}
}

// TestOpencodeSessionIdMigrationOnFreshDB proves the column lands on a brand-
// new install (all migrations from 00001), not just the upgrade path -- so a
// fresh kamacu install has BOTH claude_session_id (00003) and
// opencode_session_id (00016) on the tasks table in one pass.
func TestOpencodeSessionIdMigrationOnFreshDB(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (fresh): %v", err)
	}

	ocCount, ocCol := tasksColumnCount(t, db, "opencode_session_id")
	if ocCount != 1 {
		t.Fatalf("fresh install: opencode_session_id count = %d, want 1", ocCount)
	}
	if ocCol.typ != "TEXT" {
		t.Errorf("fresh install: opencode_session_id type = %q, want \"TEXT\"", ocCol.typ)
	}
	if ocCol.notnull != 0 {
		t.Errorf("fresh install: opencode_session_id notnull = %d, want 0 (nullable)", ocCol.notnull)
	}

	// The sibling resume key is present too -- both strategy-invariant columns
	// coexist on a fresh tasks table.
	claudeCount, _ := tasksColumnCount(t, db, "claude_session_id")
	if claudeCount != 1 {
		t.Errorf("fresh install: claude_session_id count = %d, want 1 (coexists with opencode_session_id)", claudeCount)
	}
}

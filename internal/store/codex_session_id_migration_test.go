package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// tableColumnCount runs PRAGMA table_info(<table>) and returns how many
// columns match name, plus the full info row for the first match (zero value
// when count==0). Asserts a column is absent pre-migration, present and
// exactly-once post-migration.
func tableColumnCount(t *testing.T, db *sql.DB, table, name string) (int, columnInfo) {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
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

// TestCodexSessionIdMigration proves the nullable-column invariants of
// migration 00020_codex_session_id.sql against the REAL goose runner: it
// stages the DB at the pre-00020 schema (migrations up to 00019, so the codex
// agent seed is already present), THEN applies 00020 and asserts the nullable
// TEXT codex_session_id column exists EXACTLY ONCE on both tasks and
// global_task. Mirrors TestOpencodeSessionIdMigration (which covers only
// tasks — global_task did not exist until 00017; codex adds both in one
// migration because both tables already exist).
func TestCodexSessionIdMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00020 (migrations up to 00019).
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 19); err != nil {
		t.Fatalf("UpTo(19): %v", err)
	}

	// Sanity: codex_session_id absent from BOTH tables pre-migration.
	for _, table := range []string{"tasks", "global_task"} {
		if n, _ := tableColumnCount(t, db, table, "codex_session_id"); n != 0 {
			t.Fatalf("%s.codex_session_id exists before 00020 (staging failed)", table)
		}
	}

	// Apply 00020.
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00020): %v", err)
	}

	// Post: exactly one nullable TEXT column named codex_session_id per table.
	for _, table := range []string{"tasks", "global_task"} {
		n, info := tableColumnCount(t, db, table, "codex_session_id")
		if n != 1 {
			t.Errorf("%s.codex_session_id column count after 00020 = %d, want 1", table, n)
			continue
		}
		if info.typ != "TEXT" {
			t.Errorf("%s.codex_session_id type = %q, want TEXT", table, info.typ)
		}
		if info.notnull != 0 {
			t.Errorf("%s.codex_session_id notnull = %d, want 0 (nullable: NULL = no codex session captured yet)", table, info.notnull)
		}
		if info.dflt.Valid {
			t.Errorf("%s.codex_session_id has default %q, want none (NULL until the capture poller persists an id)", table, info.dflt.String)
		}
	}

	// Round-trip: the column accepts NULL (unset) and a uuid (captured) on
	// both tables — the restart-resume key's two real states.
	if _, err := db.Exec(`INSERT INTO projects (name, repo_path) VALUES ('p', '/tmp/p')`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (project_id, title, status, position) VALUES (1, 't', 'todo', 0)`); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	var nullID sql.NullString
	if err := db.QueryRow(`SELECT codex_session_id FROM tasks WHERE id = 1`).Scan(&nullID); err != nil {
		t.Fatalf("select fresh task codex_session_id: %v", err)
	}
	if nullID.Valid {
		t.Errorf("fresh task codex_session_id = %q, want NULL", nullID.String)
	}
	const uuid = "0d1a2b3c-4444-4444-4444-444444444444"
	if _, err := db.Exec(`UPDATE tasks SET codex_session_id = ? WHERE id = 1`, uuid); err != nil {
		t.Fatalf("update task codex_session_id: %v", err)
	}
	if _, err := db.Exec(`UPDATE global_task SET codex_session_id = ? WHERE id = 1`, uuid); err != nil {
		t.Fatalf("update global_task codex_session_id: %v", err)
	}
}

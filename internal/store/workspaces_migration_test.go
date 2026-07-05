package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestWorkspacesMigration proves the WSDATA-01/02 invariants of migration
// 00012_workspaces.sql against the REAL goose runner by exercising the actual
// upgrade path: it stages the DB at the pre-00012 schema (migrations up to
// 00011), seeds projects + tasks that existed BEFORE workspaces, THEN applies
// 00012 and asserts every existing project is reassigned to Personal (id 1)
// with all tasks intact -- plus the structural guarantees (single is_default
// Personal, DEFAULT 1, NOT NULL + RESTRICT enforced at the DB, FK re-armed ON,
// idempotent re-run). Seeding AFTER Migrate would only prove a new row picks up
// DEFAULT 1; the load-bearing guarantee is that PRE-EXISTING data survives.
func TestWorkspacesMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00012 (migrations up to 00011), mirroring the
	// goose setup in migrate.go. This yields projects + tasks with NO
	// workspace_id column -- the real "existing install" shape before upgrade.
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 11); err != nil {
		t.Fatalf("UpTo(11): %v", err)
	}

	// Sanity: the workspaces table must NOT exist yet (we are truly pre-00012).
	var pre int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workspaces'",
	).Scan(&pre); err != nil {
		t.Fatalf("pre-check workspaces table: %v", err)
	}
	if pre != 0 {
		t.Fatalf("workspaces table exists before 00012 (staging failed)")
	}

	// Seed >=2 projects and >=1 task into the pre-00012 schema (the data that
	// must survive the upgrade). repo_path is UNIQUE, so vary it per project.
	seeded := []struct{ name, repo string }{
		{"alpha", "/tmp/ws-alpha"},
		{"beta", "/tmp/ws-beta"},
	}
	var seededIDs []int64
	for _, p := range seeded {
		res, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES (?, ?)", p.name, p.repo)
		if err != nil {
			t.Fatalf("seed project %q: %v", p.name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("LastInsertId: %v", err)
		}
		seededIDs = append(seededIDs, id)
	}
	// One task per seeded project (tasks -> project via ON DELETE CASCADE); if
	// 00012 wrongly rebuilt/dropped projects, these would be cascade-wiped.
	for _, pid := range seededIDs {
		if _, err := db.Exec(
			"INSERT INTO tasks (project_id, title, position) VALUES (?, ?, ?)",
			pid, "t", 1.0,
		); err != nil {
			t.Fatalf("seed task for project %d: %v", pid, err)
		}
	}
	var wantTasks int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&wantTasks); err != nil {
		t.Fatalf("count tasks (pre): %v", err)
	}
	if wantTasks < 1 {
		t.Fatalf("seeded task count = %d, want >= 1", wantTasks)
	}

	// Apply 00012 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00012): %v", err)
	}

	// --- STAGED ASSERTIONS: pre-existing data survives + is assigned Personal ---

	// (a) EVERY pre-seeded project is assigned to Personal (workspace_id == 1).
	for _, pid := range seededIDs {
		var wsID int64
		if err := db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", pid).Scan(&wsID); err != nil {
			t.Fatalf("select workspace_id for pre-seeded project %d: %v", pid, err)
		}
		if wsID != 1 {
			t.Errorf("pre-seeded project %d workspace_id = %d, want 1 (assigned to Personal)", pid, wsID)
		}
	}

	// (b) No cascade data loss: task count unchanged after the migration.
	var gotTasks int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&gotTasks); err != nil {
		t.Fatalf("count tasks (post): %v", err)
	}
	if gotTasks != wantTasks {
		t.Errorf("tasks after 00012 = %d, want %d (no cascade wipe)", gotTasks, wantTasks)
	}

	// (c) Exactly one Personal workspace (is_default=1), and its id == 1.
	var defaultCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE is_default = 1").Scan(&defaultCount); err != nil {
		t.Fatalf("count default workspaces: %v", err)
	}
	if defaultCount != 1 {
		t.Errorf("is_default workspace count = %d, want 1", defaultCount)
	}
	var defaultID int64
	if err := db.QueryRow("SELECT id FROM workspaces WHERE is_default = 1").Scan(&defaultID); err != nil {
		t.Fatalf("select default workspace id: %v", err)
	}
	if defaultID != 1 {
		t.Errorf("default workspace id = %d, want 1", defaultID)
	}

	// --- STRUCTURAL / FORWARD ASSERTIONS on the migrated handle ---

	// (6) A project inserted WITHOUT workspace_id reads back workspace_id == 1
	//     (the column DEFAULT 1 -> Personal).
	res, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES (?, ?)", "gamma", "/tmp/ws-gamma")
	if err != nil {
		t.Fatalf("insert project without workspace_id: %v", err)
	}
	gammaID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId (gamma): %v", err)
	}
	var gammaWS int64
	if err := db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", gammaID).Scan(&gammaWS); err != nil {
		t.Fatalf("select gamma workspace_id: %v", err)
	}
	if gammaWS != 1 {
		t.Errorf("new project workspace_id = %d, want 1 (DEFAULT 1)", gammaWS)
	}

	// (7) An explicit workspace_id = NULL insert errors (NOT NULL enforced, D-05).
	if _, err := db.Exec(
		"INSERT INTO projects (name, repo_path, workspace_id) VALUES (?, ?, NULL)",
		"delta", "/tmp/ws-delta",
	); err == nil {
		t.Error("insert with workspace_id = NULL succeeded, want NOT NULL failure")
	}

	// (8) DELETE the referenced Personal workspace errors (ON DELETE RESTRICT, D-04).
	if _, err := db.Exec("DELETE FROM workspaces WHERE id = 1"); err == nil {
		t.Error("delete of workspace 1 with referencing projects succeeded, want RESTRICT failure")
	}

	// (9) FK enforcement is re-armed ON on the same pooled handle (Pitfall 3).
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
	}

	// (10) Idempotency: a second Migrate is a clean no-op -- still one workspace.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var wsCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&wsCount); err != nil {
		t.Fatalf("count workspaces after re-run: %v", err)
	}
	if wsCount != 1 {
		t.Errorf("workspaces after re-run = %d, want 1 (no second Personal)", wsCount)
	}
}

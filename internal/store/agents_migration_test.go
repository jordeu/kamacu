package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestAgentsMigration proves the AGENTDATA-01/02 invariants of migration
// 00013_agents.sql against the REAL goose runner by exercising the actual
// upgrade path: it stages the DB at the pre-00013 schema (migrations up to
// 00012), seeds projects + tasks that existed BEFORE agents, THEN applies
// 00013 and asserts every existing project is reassigned to the Claude agent
// (id 1) with all tasks intact -- plus the structural guarantees (single
// is_default Claude, is_system=1, DEFAULT 1, NOT NULL + RESTRICT enforced at
// the DB, FK re-armed ON, idempotent re-run). Seeding AFTER Migrate would only
// prove a new row picks up DEFAULT 1; the load-bearing guarantee is that
// PRE-EXISTING data survives. Mirrors TestWorkspacesMigration.
func TestAgentsMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Stage the schema at pre-00013 (migrations up to 00012), mirroring the
	// goose setup in migrate.go. This yields projects + tasks with NO
	// agent_id column -- the real "existing install" shape before upgrade.
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 12); err != nil {
		t.Fatalf("UpTo(12): %v", err)
	}

	// Sanity: the agents table must NOT exist yet (we are truly pre-00013).
	var pre int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agents'",
	).Scan(&pre); err != nil {
		t.Fatalf("pre-check agents table: %v", err)
	}
	if pre != 0 {
		t.Fatalf("agents table exists before 00013 (staging failed)")
	}

	// Seed >=2 projects and >=1 task into the pre-00013 schema (the data that
	// must survive the upgrade). repo_path is UNIQUE, so vary it per project.
	// workspace_id=1 (Personal) is required post-00012; set it explicitly so the
	// seed is valid against the staged schema.
	seeded := []struct{ name, repo string }{
		{"alpha", "/tmp/ag-alpha"},
		{"beta", "/tmp/ag-beta"},
	}
	var seededIDs []int64
	for _, p := range seeded {
		res, err := db.Exec(
			"INSERT INTO projects (name, repo_path, workspace_id) VALUES (?, ?, 1)",
			p.name, p.repo,
		)
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
	// 00013 wrongly rebuilt/dropped projects, these would be cascade-wiped.
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

	// Apply 00013 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00013): %v", err)
	}

	// --- STAGED ASSERTIONS: pre-existing data survives + is assigned Claude ---

	// (a) EVERY pre-seeded project is assigned to the Claude agent (agent_id == 1).
	for _, pid := range seededIDs {
		var agID int64
		if err := db.QueryRow("SELECT agent_id FROM projects WHERE id = ?", pid).Scan(&agID); err != nil {
			t.Fatalf("select agent_id for pre-seeded project %d: %v", pid, err)
		}
		if agID != 1 {
			t.Errorf("pre-seeded project %d agent_id = %d, want 1 (assigned to Claude)", pid, agID)
		}
	}

	// (b) No cascade data loss: task count unchanged after the migration.
	var gotTasks int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&gotTasks); err != nil {
		t.Fatalf("count tasks (post): %v", err)
	}
	if gotTasks != wantTasks {
		t.Errorf("tasks after 00013 = %d, want %d (no cascade wipe)", gotTasks, wantTasks)
	}

	// (c) Exactly one default agent (is_default=1), and its id == 1.
	var defaultCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM agents WHERE is_default = 1").Scan(&defaultCount); err != nil {
		t.Fatalf("count default agents: %v", err)
	}
	if defaultCount != 1 {
		t.Errorf("is_default agent count = %d, want 1", defaultCount)
	}
	var defaultID int64
	if err := db.QueryRow("SELECT id FROM agents WHERE is_default = 1").Scan(&defaultID); err != nil {
		t.Fatalf("select default agent id: %v", err)
	}
	if defaultID != 1 {
		t.Errorf("default agent id = %d, want 1", defaultID)
	}

	// (d) The seeded agent is the Claude capability tier and is non-deletable
	//     (is_system=1, engine='claude') -- the protected default.
	var engine string
	var isSystem int
	if err := db.QueryRow("SELECT engine, is_system FROM agents WHERE id = 1").Scan(&engine, &isSystem); err != nil {
		t.Fatalf("select seed agent engine/is_system: %v", err)
	}
	if engine != "claude" {
		t.Errorf("seed agent engine = %q, want \"claude\"", engine)
	}
	if isSystem != 1 {
		t.Errorf("seed agent is_system = %d, want 1 (non-deletable)", isSystem)
	}

	// --- STRUCTURAL / FORWARD ASSERTIONS on the migrated handle ---

	// (5) A project inserted WITHOUT agent_id reads back agent_id == 1
	//     (the column DEFAULT 1 -> Claude).
	res, err := db.Exec("INSERT INTO projects (name, repo_path, workspace_id) VALUES (?, ?, 1)", "gamma", "/tmp/ag-gamma")
	if err != nil {
		t.Fatalf("insert project without agent_id: %v", err)
	}
	gammaID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId (gamma): %v", err)
	}
	var gammaAg int64
	if err := db.QueryRow("SELECT agent_id FROM projects WHERE id = ?", gammaID).Scan(&gammaAg); err != nil {
		t.Fatalf("select gamma agent_id: %v", err)
	}
	if gammaAg != 1 {
		t.Errorf("new project agent_id = %d, want 1 (DEFAULT 1)", gammaAg)
	}

	// (6) An explicit agent_id = NULL insert errors (NOT NULL enforced).
	if _, err := db.Exec(
		"INSERT INTO projects (name, repo_path, workspace_id, agent_id) VALUES (?, ?, 1, NULL)",
		"delta", "/tmp/ag-delta",
	); err == nil {
		t.Error("insert with agent_id = NULL succeeded, want NOT NULL failure")
	}

	// (7) DELETE the referenced Claude agent errors (ON DELETE RESTRICT) while a
	//     project still references it.
	if _, err := db.Exec("DELETE FROM agents WHERE id = 1"); err == nil {
		t.Error("delete of agent 1 with referencing projects succeeded, want RESTRICT failure")
	}

	// (8) FK enforcement is re-armed ON on the same pooled handle (Pitfall 3).
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
	}

	// (9) Idempotency: a second Migrate is a clean no-op -- still one agent.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var agCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&agCount); err != nil {
		t.Fatalf("count agents after re-run: %v", err)
	}
	if agCount != 1 {
		t.Errorf("agents after re-run = %d, want 1 (no second Claude)", agCount)
	}
}

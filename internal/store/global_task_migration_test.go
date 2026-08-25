package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// stageV116 stages a fresh DB at goose v16 -- the pre-00017 v1.12 schema, the
// exact shape a real install has before this phase's upgrade -- using the REAL
// embedded migrations and the REAL goose runner (the staging discipline of
// TestAgentsMigration / TestWorkspacesMigration). It also sanity-asserts via
// sqlite_master that global_task does not exist yet: seeding data into a
// schema that silently already had the new tables would prove nothing.
func stageV116(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("SetDialect: %v", err)
	}
	if err := goose.UpTo(db, "migrations", 16); err != nil {
		t.Fatalf("UpTo(16): %v", err)
	}

	var pre int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='global_task'",
	).Scan(&pre); err != nil {
		t.Fatalf("pre-check global_task table: %v", err)
	}
	if pre != 0 {
		t.Fatalf("global_task table exists before 00017 (staging failed)")
	}
	return db
}

// TestGlobalTaskMigration proves the GDATA-01 invariants of migration
// 00017_global_task.sql against the REAL goose runner by exercising the
// actual upgrade path: stage at pre-00017 (v16), then Migrate and assert the
// singleton exists, is seeded from the is_default agent (queried via the
// flag -- never a literal id), carries the unconfigured root defaults, and is
// protected at the DB: CHECK(id=1) rejects a second row, ON DELETE RESTRICT
// refuses deleting the referenced agent, and a second Migrate is a no-op.
func TestGlobalTaskMigration(t *testing.T) {
	db := stageV116(t)

	// Apply 00017 + 00018 (Migrate re-sets base FS + dialect and runs goose.Up).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00017): %v", err)
	}

	// (a) Exactly one global_task row.
	var rowCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM global_task").Scan(&rowCount); err != nil {
		t.Fatalf("count global_task rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("global_task row count = %d, want 1", rowCount)
	}

	// (b) id == 1 and agent_id == the is_default agent (query the flag, never
	//     assert the literal -- the flag is movable via POST /api/agents/{id}/default).
	var defaultAgentID int64
	if err := db.QueryRow("SELECT id FROM agents WHERE is_default = 1").Scan(&defaultAgentID); err != nil {
		t.Fatalf("select default agent id: %v", err)
	}
	var id, agentID int64
	if err := db.QueryRow("SELECT id, agent_id FROM global_task").Scan(&id, &agentID); err != nil {
		t.Fatalf("select global_task row: %v", err)
	}
	if id != 1 {
		t.Errorf("global_task id = %d, want 1", id)
	}
	if agentID != defaultAgentID {
		t.Errorf("global_task agent_id = %d, want %d (the is_default agent)", agentID, defaultAgentID)
	}

	// (c) Unconfigured root: root_path '' (empty string), github_repo and both
	//     resume-id columns NULL.
	var rootPath string
	var githubRepo, claudeSID, opencodeSID sql.NullString
	if err := db.QueryRow(
		"SELECT root_path, github_repo, claude_session_id, opencode_session_id FROM global_task",
	).Scan(&rootPath, &githubRepo, &claudeSID, &opencodeSID); err != nil {
		t.Fatalf("select global_task root columns: %v", err)
	}
	if rootPath != "" {
		t.Errorf("global_task root_path = %q, want \"\" (unconfigured)", rootPath)
	}
	if githubRepo.Valid {
		t.Errorf("global_task github_repo = %q, want NULL", githubRepo.String)
	}
	if claudeSID.Valid {
		t.Errorf("global_task claude_session_id = %q, want NULL", claudeSID.String)
	}
	if opencodeSID.Valid {
		t.Errorf("global_task opencode_session_id = %q, want NULL", opencodeSID.String)
	}

	// (d) A second singleton row is unrepresentable: CHECK (id = 1) rejects it.
	if _, err := db.Exec(
		"INSERT INTO global_task (id, agent_id) VALUES (2, ?)", defaultAgentID,
	); err == nil {
		t.Error("insert global_task id=2 succeeded, want CHECK(id=1) failure")
	}

	// (e) Deleting the referenced agent errors (ON DELETE RESTRICT) -- the DB
	//     backstop behind the handler-level 409 delete guard.
	if _, err := db.Exec(
		"DELETE FROM agents WHERE id = (SELECT agent_id FROM global_task)",
	); err == nil {
		t.Error("delete of the agent referenced by global_task succeeded, want RESTRICT failure")
	}

	// (f) Idempotency: a second Migrate is a clean no-op -- still exactly one row.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM global_task").Scan(&rowCount); err != nil {
		t.Fatalf("count global_task after re-run: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("global_task rows after re-run = %d, want 1 (no second singleton)", rowCount)
	}
}

// tmuxSnapshot reads the byte-for-byte survival surface of tmux_sessions in a
// stable row order, formatted as exact strings -- id, task_id, n, name,
// label, created_at. A COUNT-only comparison cannot catch label/timestamp
// corruption, so the test compares these snapshots row-for-row.
func tmuxSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		"SELECT id, task_id, n, name, label, created_at FROM tmux_sessions ORDER BY id",
	)
	if err != nil {
		t.Fatalf("snapshot tmux_sessions: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, taskID, n int64
		var name, label, createdAt string
		if err := rows.Scan(&id, &taskID, &n, &name, &label, &createdAt); err != nil {
			t.Fatalf("scan tmux_snapshot row: %v", err)
		}
		out = append(out, fmt.Sprintf("%d|%d|%d|%s|%s|%s", id, taskID, n, name, label, createdAt))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tmux_snapshot: %v", err)
	}
	return out
}

// TestTmuxScopeRebuildMigration proves the GDATA-02 invariants of migration
// 00018_tmux_scope.sql against the REAL goose runner: it stages the DB at the
// pre-00017 v1.12 schema, seeds an "existing install" (project, two tasks,
// three tmux_sessions rows with CUSTOM labels and exact created_at literals),
// snapshots the survival surface, THEN applies Migrate and asserts every
// pre-existing row survives byte-for-byte -- plus the structural guarantees:
// copied rows read scope='task', task-less global rows are representable,
// ambiguous and neither rows are rejected (both XOR directions), the FK is
// re-armed and bites on the same pooled handle, and a second Migrate is a
// no-op.
func TestTmuxScopeRebuildMigration(t *testing.T) {
	db := stageV116(t)

	// Pre-check we are truly pre-00018: tmux_sessions has no scope column yet.
	var scopeCols int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('tmux_sessions') WHERE name='scope'",
	).Scan(&scopeCols); err != nil {
		t.Fatalf("pre-check scope column: %v", err)
	}
	if scopeCols != 0 {
		t.Fatalf("tmux_sessions.scope exists before 00018 (staging failed)")
	}

	// Seed the "existing install" (the data that must survive byte-for-byte).
	// repo_path is UNIQUE; workspace_id=1 (Personal) is required post-00012
	// (agent_id gets its DEFAULT 1 from 00013).
	res, err := db.Exec(
		"INSERT INTO projects (name, repo_path, workspace_id) VALUES (?, ?, 1)",
		"alpha", "/tmp/gd-alpha",
	)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	projectID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId (project): %v", err)
	}
	for _, title := range []string{"t1", "t2"} {
		if _, err := db.Exec(
			"INSERT INTO tasks (project_id, title, position) VALUES (?, ?, ?)",
			projectID, title, 1.0,
		); err != nil {
			t.Fatalf("seed task %q: %v", title, err)
		}
	}
	var taskOne, taskTwo int64
	if err := db.QueryRow("SELECT MIN(id), MAX(id) FROM tasks").Scan(&taskOne, &taskTwo); err != nil {
		t.Fatalf("select seeded task ids: %v", err)
	}

	// Three tmux rows: two custom labels + exact created_at literals, one with
	// label '' to cover the DEFAULT, one under the second task.
	seeded := []struct {
		taskID    int64
		n         int64
		name      string
		label     string
		createdAt string
	}{
		{taskOne, 1, "kamacu-1-1", "Build", "2026-01-02T03:04:05.678Z"},
		{taskOne, 2, "kamacu-1-2", "", "2026-02-03T04:05:06.789Z"},
		{taskTwo, 1, "kamacu-2-1", "Deploy", "2026-03-04T05:06:07.890Z"},
	}
	for _, s := range seeded {
		if _, err := db.Exec(
			"INSERT INTO tmux_sessions (task_id, n, name, label, created_at) VALUES (?, ?, ?, ?, ?)",
			s.taskID, s.n, s.name, s.label, s.createdAt,
		); err != nil {
			t.Fatalf("seed tmux row %q: %v", s.name, err)
		}
	}
	before := tmuxSnapshot(t, db)
	if len(before) != 3 {
		t.Fatalf("seeded tmux rows = %d, want 3", len(before))
	}

	// Apply 00017 + 00018 (the real upgrade path boot runs).
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate (apply 00018): %v", err)
	}

	// (a) Byte-for-byte survival: the AFTER snapshot matches the BEFORE
	//     snapshot row-for-row as exact strings.
	after := tmuxSnapshot(t, db)
	if len(after) != len(before) {
		t.Errorf("tmux rows after 00018 = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if i >= len(after) {
			break
		}
		if before[i] != after[i] {
			t.Errorf("tmux row %d corrupted by rebuild:\n  before: %s\n  after:  %s", i, before[i], after[i])
		}
	}

	// (b) Every copied row reads scope='task'.
	var taskScoped int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM tmux_sessions WHERE scope = 'task'",
	).Scan(&taskScoped); err != nil {
		t.Fatalf("count scope='task' rows: %v", err)
	}
	if taskScoped != len(before) {
		t.Errorf("scope='task' rows = %d, want %d (every copied row)", taskScoped, len(before))
	}

	// (c) Acceptance direction 1: a task-less global row is representable.
	if _, err := db.Exec(
		"INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (NULL, 'global', 1, 'kamacu-global-1', '')",
	); err != nil {
		t.Errorf("insert global-scoped row failed, want success: %v", err)
	}

	// (d) Rejection direction 1 -- ambiguous (task_id set + scope='global').
	//     n=7 avoids colliding with UNIQUE(task_id, n) so the XOR CHECK is the
	//     certain failure.
	if _, err := db.Exec(
		"INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (?, 'global', 7, 'kamacu-bad-amb', '')",
		taskOne,
	); err == nil {
		t.Error("insert ambiguous row (task_id set + scope='global') succeeded, want XOR CHECK failure")
	}

	// (e) Rejection direction 2 -- neither (task_id NULL + scope='task').
	//     An inverted or OR-spelled CHECK is only catchable by asserting BOTH
	//     rejection directions (13-RESEARCH Pitfall 3).
	if _, err := db.Exec(
		"INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (NULL, 'task', 3, 'kamacu-bad-nei', '')",
	); err == nil {
		t.Error("insert neither row (task_id NULL + scope='task') succeeded, want XOR CHECK failure")
	}

	// (f) The re-armed FK bites on this same pooled handle: a bogus task_id
	//     insert fails.
	if _, err := db.Exec(
		"INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (999, 'task', 4, 'kamacu-bad-fk', '')",
	); err == nil {
		t.Error("insert with bogus task_id=999 succeeded, want FK failure (re-armed)")
	}

	// (g) FK enforcement is re-armed ON on the same pooled handle -- the
	//     load-bearing re-arm canary (00012/00013 discipline).
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys after migration = %d, want 1 (re-armed ON)", fk)
	}

	// (h) Idempotency: a second Migrate is a clean no-op -- row count unchanged.
	var wantRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM tmux_sessions").Scan(&wantRows); err != nil {
		t.Fatalf("count tmux rows (pre re-run): %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var gotRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM tmux_sessions").Scan(&gotRows); err != nil {
		t.Fatalf("count tmux rows (post re-run): %v", err)
	}
	if gotRows != wantRows {
		t.Errorf("tmux rows after re-run = %d, want %d (unchanged)", gotRows, wantRows)
	}
}

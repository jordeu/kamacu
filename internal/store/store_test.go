package store

import (
	"path/filepath"
	"testing"
)

func TestOpenPragmas(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestMigrateCreatesTables(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"projects", "tasks"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestDeleteProjectCascadesToTasks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	res, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES (?, ?)", "p1", "/tmp/p1")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projectID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	_, err = db.Exec(
		"INSERT INTO tasks (project_id, title, position) VALUES (?, ?, ?)",
		projectID, "t1", 1.0,
	)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	if _, err := db.Exec("DELETE FROM projects WHERE id = ?", projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM tasks WHERE project_id = ?", projectID,
	).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 0 {
		t.Errorf("tasks after project delete = %d, want 0 (cascade)", count)
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES (?, ?)", "persisted", "/tmp/persisted"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()

	var name string
	if err := db2.QueryRow("SELECT name FROM projects WHERE repo_path = ?", "/tmp/persisted").Scan(&name); err != nil {
		t.Fatalf("select after reopen: %v", err)
	}
	if name != "persisted" {
		t.Errorf("name = %q, want %q", name, "persisted")
	}
}

func TestTaskStatusCheckConstraint(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	res, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES (?, ?)", "p1", "/tmp/p1")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projectID, _ := res.LastInsertId()

	_, err = db.Exec(
		"INSERT INTO tasks (project_id, title, status, position) VALUES (?, ?, ?, ?)",
		projectID, "bad", "bogus", 1.0,
	)
	if err == nil {
		t.Error("insert with status 'bogus' succeeded, want CHECK constraint failure")
	}
}

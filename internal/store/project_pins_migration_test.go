package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestProjectPinsMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 49); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO projects (name, repo_path) VALUES ('Existing project', '/tmp/pin-migration')"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var name string
	var pinned bool
	if err := db.QueryRow("SELECT name, pinned FROM projects WHERE repo_path = '/tmp/pin-migration'").Scan(&name, &pinned); err != nil {
		t.Fatal(err)
	}
	if name != "Existing project" || pinned {
		t.Fatalf("migrated project: name=%q pinned=%v", name, pinned)
	}
	if _, err := db.Exec("UPDATE projects SET pinned = 1 WHERE repo_path = '/tmp/pin-migration'"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT pinned FROM projects WHERE repo_path = '/tmp/pin-migration'").Scan(&pinned); err != nil {
		t.Fatal(err)
	}
	if !pinned {
		t.Fatal("repeated migration reset the pin")
	}
}

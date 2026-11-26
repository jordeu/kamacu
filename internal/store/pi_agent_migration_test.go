package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestPiAgentMigrationPreservesExistingPiCommand(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "previous.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 53); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents (name, command) VALUES ('pi', 'my-pi-wrapper'), ('Pi (built-in)', 'other-wrapper')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var name, command, arg string
	var isDefault bool
	if err := db.QueryRow(`SELECT name, command, model_argument, is_default FROM agents WHERE engine='pi' AND is_system=1`).
		Scan(&name, &command, &arg, &isDefault); err != nil {
		t.Fatal(err)
	}
	if name == "Pi" || name == "Pi (built-in)" || command != "pi" || arg != "--model {{model}}" || isDefault {
		t.Fatalf("built-in Pi seed = %q, %q, %q, default=%v", name, command, arg, isDefault)
	}
	if err := db.QueryRow(`SELECT command FROM agents WHERE name='pi' AND engine='custom'`).Scan(&command); err != nil || command != "my-pi-wrapper" {
		t.Fatalf("custom Pi command = %q, %v", command, err)
	}
}

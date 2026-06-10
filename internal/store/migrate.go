package store

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending embedded migrations to db.
// Idempotent: goose's version table makes Up a no-op when current.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil { // dialect string is "sqlite3"
		return err // even though driver name is "sqlite"
	}
	return goose.Up(db, "migrations")
}

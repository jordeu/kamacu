package store

import (
	"database/sql"

	_ "modernc.org/sqlite" // driver name is "sqlite", NOT "sqlite3"
)

// Open opens the SQLite database at dbPath with the mandatory pragma
// discipline: WAL journal mode, busy timeout, foreign keys enforced,
// NORMAL synchronous, and immediate transaction locking.
func Open(dbPath string) (*sql.DB, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single-writer discipline: eliminates SQLITE_BUSY
	return db, db.Ping()
}

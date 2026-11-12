package store

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestInboxQueueMigrationPreservesOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 44); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO inbox_questions(id,session_id,created_session_id,request_key,summary,context,question,options,status,origin,created_at,updated_at,heartbeat_at) VALUES(?,'',?,?,'Summary','Context','Question','[]','pending','notification',?,1,1)`
	// Arrival order differs from creation order; equal times retain arrival order.
	for i, id := range []string{"later", "first", "second"} {
		created := 100
		if i == 0 {
			created = 200
		}
		if _, err = db.Exec(insert, id, id, id, created); err != nil {
			t.Fatal(err)
		}
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	// New arrivals must follow existing messages regardless of timestamp.
	if _, err = db.Exec(insert, "new", "new", "new", 1); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT id,queue_position FROM inbox_questions ORDER BY queue_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for i, want := range []string{"first", "second", "later", "new"} {
		var id string
		var position int
		if !rows.Next() {
			t.Fatalf("missing queue entry %s: %v", want, rows.Err())
		}
		if err = rows.Scan(&id, &position); err != nil || id != want || position != i+1 {
			t.Fatalf("queue entry: %s %d, want %s %d: %v", id, position, want, i+1, err)
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected queue entries: %v", rows.Err())
	}
}

package store

import (
	"github.com/pressly/goose/v3"
	"path/filepath"
	"testing"
)

func TestScratchpadContentMigrationPreservesExistingData(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 24); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`INSERT INTO projects(id,name,repo_path) VALUES(1,'P','/tmp/p')`,
		`INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'T',0)`,
		`INSERT INTO artifacts VALUES('a',1,'Preview','r','created','updated')`,
		`INSERT INTO artifact_revisions VALUES('r','a')`,
		`INSERT INTO artifact_uploads VALUES('u',1,'a','Preview','preview.html',10,'r','hash',123,1,'')`,
		`INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,summary,context,question,options,status,created_at,updated_at,heartbeat_at) VALUES('q',1,'s','s','k','summary','context','question','[]','pending',1,2,3)`,
		`INSERT INTO inbox_events VALUES(1,'q','created','s',1)`,
	}
	for _, q := range queries {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	check := func() {
		t.Helper()
		for _, table := range []string{"artifacts", "artifact_revisions", "artifact_uploads", "inbox_questions", "inbox_events"} {
			var n int
			if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 1 {
				t.Fatalf("%s count=%d: %v", table, n, err)
			}
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("broken foreign key")
		}
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err = db.Exec(`INSERT INTO artifacts VALUES('global',NULL,'Scratchpad','gr','c','u')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO artifact_revisions VALUES('gr','global')`); err != nil {
		t.Fatal(err)
	}
	if err = goose.DownTo(db, "migrations", 24); err != nil {
		t.Fatal(err)
	}
	check()
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	check()
	if _, err = db.Exec("DELETE FROM tasks WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"artifacts", "artifact_revisions", "artifact_uploads", "inbox_questions", "inbox_events"} {
		var n int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("cascade %s: %d %v", table, n, err)
		}
	}
}

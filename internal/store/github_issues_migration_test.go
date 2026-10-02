package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestGithubIssueMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "issues.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 20); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO projects (id,name,repo_path) VALUES (1,'one','/one'),(2,'two','/two')`,
		`INSERT INTO tasks (project_id,title,description,position) VALUES (1,'Existing','Keep me',1)`,
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	var title, description string
	var repo, url sql.NullString
	var number sql.NullInt64
	if err = db.QueryRow(`SELECT title,description,issue_repo,issue_number,issue_url FROM tasks WHERE id=1`).Scan(&title, &description, &repo, &number, &url); err != nil {
		t.Fatal(err)
	}
	if title != "Existing" || description != "Keep me" || repo.Valid || number.Valid || url.Valid {
		t.Fatal("existing task changed during migration")
	}
	insert := `INSERT INTO tasks(project_id,title,position,issue_repo,issue_number) VALUES (?,'Imported',0,?,42)`
	if _, err = db.Exec(insert, 1, "owner/repo"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(insert, 1, "OWNER/REPO"); err == nil {
		t.Fatal("case-insensitive duplicate accepted")
	}
	for _, tc := range []struct {
		project int
		repo    string
	}{{2, "owner/repo"}, {1, "owner/another"}} {
		if _, err = db.Exec(insert, tc.project, tc.repo); err != nil {
			t.Fatalf("independent import blocked: %v", err)
		}
	}
	if err = goose.Down(db, "migrations"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT description FROM tasks WHERE id=1`).Scan(&description); err != nil || description != "Keep me" {
		t.Fatalf("rollback lost task: %v", err)
	}
}

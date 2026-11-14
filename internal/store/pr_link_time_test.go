package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

func TestPRLinkTimeSurvivesEvidenceRefreshAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 46); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`,
		`INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Implement',0)`,
		`INSERT INTO task_pr_links(task_id,repository,number,role,source,evidence,uncertainty,observed_at) VALUES(1,'acme/app',12,'implementation','explicit','linked','','2026-11-13T10:00:00Z')`,
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	l := PRLink{TaskID: 1, Repository: "acme/app", Number: 47, Role: "implementation", Source: "explicit", Evidence: "linked", ObservedAt: first.Add(time.Hour)}
	if err = PutPRLink(t.Context(), db, l); err != nil {
		t.Fatal(err)
	}
	// Mentioning the older PR again must not replace the latest association.
	l.Number, l.ObservedAt = 12, first.Add(2*time.Hour)
	if err = PutPRLink(t.Context(), db, l); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, batch := range []bool{false, true} {
		links, readErr := PRLinks(t.Context(), db, 1, "", 0)
		if batch {
			links, readErr = PRLinksForTasks(t.Context(), db, []int64{1})
		}
		if readErr != nil || len(links) != 2 {
			t.Fatalf("links=%+v: %v", links, readErr)
		}
		if links[0].LinkedAt == nil || !links[0].LinkedAt.Equal(first) || !links[0].ObservedAt.Equal(l.ObservedAt) {
			t.Fatalf("legacy link time changed after evidence refresh: %+v", links[0])
		}
		if links[1].LinkedAt == nil || !links[1].LinkedAt.Equal(first.Add(time.Hour)) {
			t.Fatalf("new link time missing: %+v", links[1])
		}
	}
	// Explicitly rejecting and re-linking a PR creates a new association time.
	l.Corrected, l.Role, l.ObservedAt = true, "incidental", first.Add(3*time.Hour)
	if err = PutPRLink(t.Context(), db, l); err != nil {
		t.Fatal(err)
	}
	l.Role, l.ObservedAt = "implementation", first.Add(4*time.Hour)
	if err = PutPRLink(t.Context(), db, l); err != nil {
		t.Fatal(err)
	}
	links, err := PRLinks(t.Context(), db, 1, "", 0)
	if err != nil || len(links) != 2 || links[0].LinkedAt == nil || !links[0].LinkedAt.Equal(l.ObservedAt) {
		t.Fatalf("re-link time not updated: %+v: %v", links, err)
	}
	if err = goose.DownTo(db, "migrations", 46); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
}

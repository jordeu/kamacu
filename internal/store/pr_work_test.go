package store

import (
	"context"
	"fmt"
	"github.com/pressly/goose/v3"
	"path/filepath"
	"testing"
	"time"
)

func TestPRWorkOrderingCorrectionsAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Implement',0),(2,1,'Review',1)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().Add(-time.Hour).UTC()
	l := PRLink{TaskID: 1, Repository: "https://github.com/ACME/Repo/pull/7", Role: "implementation", Source: "terminal:s1", Evidence: "Created PR", ObservedAt: at}
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	l.Repository = "acme/repo"
	l.Number = 7
	l.Role = "review"
	l.ObservedAt = at.Add(-time.Minute)
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	links, err := PRLinks(ctx, db, 1, "", 0)
	if err != nil || len(links) != 1 || links[0].Role != "implementation" {
		t.Fatalf("older update won: %+v %v", links, err)
	}
	l.Role = "incidental"
	l.Corrected = true
	l.Source = "explicit"
	l.ObservedAt = at.Add(time.Minute)
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	l.Corrected = false
	l.Role = "review"
	l.ObservedAt = at.Add(2 * time.Minute)
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	links, _ = PRLinks(ctx, db, 1, "", 0)
	if !links[0].Corrected || links[0].Role != "incidental" {
		t.Fatal("lost correction", links)
	}
	l.TaskID = 2
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	l.Number = 8
	if err = PutPRLink(ctx, db, l); err != nil {
		t.Fatal(err)
	}
	taskID := int64(2)
	work := SessionWork{SessionID: "s2", TaskID: &taskID, State: "idle", Summary: "Review complete", Evidence: "Approved", ObservedAt: at}
	if err = PutSessionWork(ctx, db, work); err != nil {
		t.Fatal(err)
	}
	work.Summary = "older"
	work.ObservedAt = at.Add(-time.Minute)
	if err = PutSessionWork(ctx, db, work); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	related, err := RelatedPRWork(ctx, db, "Acme/Repo", 7)
	if err != nil || len(related) != 1 || len(related[0].Sessions) != 1 || related[0].Sessions[0].Summary != "Review complete" || !related[0].Sessions[0].Stale {
		t.Fatalf("restart/context: %+v %v", related, err)
	}
	links, _ = PRLinks(ctx, db, 2, "", 0)
	if len(links) != 2 {
		t.Fatal("many-to-many links lost", links)
	}
}
func TestPRIdentity(t *testing.T) {
	for _, s := range []string{"https://evil.test/acme/repo/pull/7", "https://user@github.com/acme/repo/pull/7", "acme/repo/extra", "https://github.com/acme/repo/issues/7"} {
		if _, _, err := PRIdentity(s, 7); err == nil {
			t.Fatal("accepted", s)
		}
	}
	if _, _, err := PRIdentity("https://github.com/acme/repo/pull/7", 8); err == nil {
		t.Fatal("accepted mismatched number")
	}
}

func TestPRWorkBackfillsReviewTasks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "backfill.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 34); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path,github_repo) VALUES(1,'repo','/repo','ACME/Repo')`, `INSERT INTO tasks(id,project_id,title,position,source,pr_number) VALUES(1,1,'Review',0,'github_pr',7)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	links, err := PRLinks(context.Background(), db, 1, "", 0)
	if err != nil || len(links) != 1 || links[0].Repository != "acme/repo" || links[0].Role != "review" {
		t.Fatalf("backfill: %+v %v", links, err)
	}
}

func TestLinkedSessionWithoutSummaryAndCorrection(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "links.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Review',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, taskID := range []int64{0, 1} {
		id := fmt.Sprint(taskID)
		link := PRLink{TaskID: taskID, Repository: "acme/repo", Number: 7, Role: "review", Source: "terminal:" + id, Evidence: "Reviewing PR", ObservedAt: time.Now()}
		if err = PutSessionPRLink(ctx, db, id, link); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := RelatedPRSessions(ctx, db, "acme/repo", 7)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("links without summaries invisible: %+v %v", sessions, err)
	}
	for _, w := range sessions {
		if w.State != "unknown" || w.Summary != "" || !w.StateStale {
			t.Fatalf("fabricated activity for summaryless link: %+v", w)
		}
	}
	if err = PutPRLink(ctx, db, PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "incidental", Source: "explicit", Corrected: true, Evidence: "Wrong PR", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	sessions, err = RelatedPRSessions(ctx, db, "acme/repo", 7)
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "0" {
		t.Fatalf("correction ignored without summary: %+v %v", sessions, err)
	}
}

func TestSessionLifecycleIndependentOfSummary(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute)
	old := SessionWork{SessionID: "agent", State: "working", Summary: "Review underway", Evidence: "Reviewing PR", ObservedAt: at}
	if err = PutSessionWork(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if err = PutSessionState(ctx, db, "agent", nil, "idle", at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	old.ObservedAt = at.Add(time.Millisecond)
	old.Summary = "Earlier work evidence"
	if err = PutSessionWork(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	w, err := WorkForSession(ctx, db, "agent")
	if err != nil || w.State != "idle" || !w.StateObservedAt.Equal(at.Add(time.Second)) || !w.ObservedAt.Equal(old.ObservedAt) {
		t.Fatalf("summary rolled back lifecycle: %+v %v", w, err)
	}
	if err = ReconcileSessionStates(ctx, db, map[string]bool{"agent": false}, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = PutSessionState(ctx, db, "agent", nil, "working", at.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	w, _ = WorkForSession(ctx, db, "agent")
	if w.State != "exited" {
		t.Fatal("delayed classification resurrected an exited process", w)
	}
	if err = PutSessionState(ctx, db, "missing", nil, "working", at); err != nil {
		t.Fatal(err)
	}
	if err = ReconcileSessionStates(ctx, db, map[string]bool{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	w, _ = WorkForSession(ctx, db, "missing")
	if w.State != "unknown" {
		t.Fatal("missing restored process retained working state", w)
	}
}

func TestSessionLifecycleMigrationPreservesExistingEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpTo(db, "migrations", 35); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Format(workTimeFormat)
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'repo','/repo')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Review',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO session_work(session_id,task_id,state,summary,evidence,uncertainty,observed_at) VALUES('agent',1,'working','Review underway','Reviewing PR','',?)`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO session_pr_links(session_id,repository,number,role,source,evidence,uncertainty,observed_at) VALUES('agent','acme/repo',7,'review','terminal','Reviewing PR','',?)`, at); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(db); err != nil {
		t.Fatal(err)
	}
	w, err := WorkForSession(context.Background(), db, "agent")
	if err != nil || w.Summary != "Review underway" || !w.StateObservedAt.Equal(w.ObservedAt) {
		t.Fatalf("migration lost summary/state: %+v %v", w, err)
	}
	var taskID int64
	if err = db.QueryRow(`SELECT task_id FROM session_pr_links WHERE session_id='agent'`).Scan(&taskID); err != nil || taskID != 1 {
		t.Fatalf("missing association task identity: %d %v", taskID, err)
	}
}

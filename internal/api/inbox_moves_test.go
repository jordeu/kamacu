package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/store"
	"kamacu/internal/worktree"
)

func TestCompletionMoveActions(t *testing.T) {
	for _, tc := range []struct {
		name, action, status string
		code                 int
	}{
		{"review", inbox.MoveToReview, "in_progress", 200},
		{"done", inbox.MoveToDone, "in_progress", 200},
		{"review to done", inbox.MoveToDone, "in_review", 200},
		{"already in review", inbox.MoveToReview, "in_review", 400},
		{"already done", inbox.MoveToDone, "done", 400},
		{"stale task", inbox.MoveToReview, "in_progress", 409},
		{"move failure", inbox.MoveToDone, "in_progress", 500},
		{"answer failure", inbox.MoveToDone, "in_progress", 500},
		{"recurring", inbox.MoveToDone, "in_progress", 200},
		{"not recommended", inbox.MoveToDone, "in_progress", 200},
		{"PR review", inbox.MoveToDone, "in_progress", 400},
		{"stale message", inbox.MoveToDone, "in_progress", 200},
		{"hidden", inbox.MoveToDone, "in_progress", 409},
		{"new input", inbox.MoveToDone, "in_progress", 409},
		{"close PR review", inbox.CloseSession, "todo", 200},
		{"close done task", inbox.CloseSession, "done", 200},
		{"close new input", inbox.CloseSession, "in_progress", 409},
		{"close new output", inbox.CloseSession, "in_progress", 409},
		{"close hidden", inbox.CloseSession, "in_progress", 409},
		{"close stale message", inbox.CloseSession, "in_progress", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "inbox.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = store.Migrate(db); err != nil {
				t.Fatal(err)
			}
			for _, sql := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'Project','/tmp/project')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Task',0)`} {
				if _, err = db.Exec(sql); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec(`UPDATE tasks SET status=? WHERE id=1`, tc.status); err != nil {
				t.Fatal(err)
			}
			if tc.name == "PR review" || tc.name == "close PR review" {
				if _, err = db.Exec(`UPDATE tasks SET source='github_pr' WHERE id=1`); err != nil {
					t.Fatal(err)
				}
			}
			var input uint64
			revision := uint64(1)
			svc := inbox.New(db, func(string) inbox.Session {
				return inbox.Session{TaskID: 1, Live: true, Revision: revision, Input: input}
			})
			defer svc.Close()
			ConfigureInboxTaskMoves(svc, db, worktree.NewService(t.TempDir()))
			if tc.name == "recurring" {
				if _, err = db.Exec(`UPDATE tasks SET recurrent=1 WHERE id=1`); err != nil {
					t.Fatal(err)
				}
			}
			options := []inbox.Option{{ID: tc.action, Label: "model-provided label", Recommended: true}}
			if tc.name == "not recommended" {
				options = nil
			}
			created, err := svc.Observe(ctx, inbox.Identity{TaskID: 1, SessionID: "origin"}, inbox.Create{Key: "done", Abstract: "Done", Summary: "Done", Context: "Verified", Question: "Work completed", Options: options}, "completion", "terminal", 1, 0)
			if err != nil {
				t.Fatal(err)
			}
			q := created.Question
			if tc.name == "stale task" {
				_, err = db.Exec(`UPDATE tasks SET status='done' WHERE id=1`)
			}
			if tc.name == "move failure" {
				_, err = db.Exec(`CREATE TRIGGER fail_move BEFORE UPDATE OF status ON tasks BEGIN SELECT RAISE(ABORT,'failed move'); END`)
			}
			if tc.name == "answer failure" {
				_, err = db.Exec(`CREATE TRIGGER fail_answer BEFORE UPDATE OF status ON inbox_questions BEGIN SELECT RAISE(ABORT,'failed answer'); END`)
			}
			if tc.name == "new input" || tc.name == "close new input" {
				input = 1
			}
			if tc.name == "close new output" {
				revision++
			}
			if tc.name == "hidden" || tc.name == "close hidden" {
				err = svc.HideObserved(ctx, "origin", true)
			}
			if err != nil {
				t.Fatal(err)
			}
			stopped, delivered := 0, 0
			mux := http.NewServeMux()
			inboxRoutes(mux, svc, "token", func(inbox.Question) error { delivered++; return nil }, func(id string) {
				if id != "origin" {
					t.Fatal("wrong session")
				}
				stopped++
			})
			messageRevision := q.Revision
			if tc.name == "stale message" || tc.name == "close stale message" {
				messageRevision--
			}
			raw, _ := json.Marshal(inboxAnswer{Revision: messageRevision, OptionID: tc.action})
			call := func() {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/inbox/"+q.ID+"/answer", bytes.NewReader(raw)))
				if w.Code != tc.code {
					t.Fatalf("status %d: %s", w.Code, w.Body)
				}
			}
			call()
			saved, err := svc.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			var status string
			if err = db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if tc.code == 200 && tc.name != "stale message" && tc.name != "close stale message" {
				target := inbox.CompletionTarget(tc.action)
				if tc.action == inbox.CloseSession {
					target = tc.status
				}
				if status != target || saved.Status != "answered" || saved.AcknowledgedAt == nil || stopped != 1 {
					t.Fatalf("status=%s message=%+v stops=%d", status, saved, stopped)
				}
				// A retry closes only the same originating session and never moves again.
				if _, err = db.Exec(`UPDATE tasks SET status='todo' WHERE id=1`); err != nil {
					t.Fatal(err)
				}
				call()
				if err = db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&status); err != nil || status != "todo" {
					t.Fatalf("retry moved task again: %s %v", status, err)
				}
			} else {
				want := tc.status
				if tc.name == "stale task" {
					want = "done"
				}
				if status != want || saved.Status != "pending" || stopped != 0 {
					t.Fatalf("rejected action changed state: %s %+v stops=%d", status, saved, stopped)
				}
			}
			if tc.name == "recurring" {
				var count int
				if err = db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("successors: %d %v", count, err)
				}
			}
			if delivered != 0 {
				t.Fatal("board action was sent as terminal input")
			}
		})
	}
}

package api

import (
	"context"
	"errors"
	"fmt"
	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/notifications"
	"kamacu/internal/session"
	"kamacu/internal/store"
	"kamacu/internal/worktree"
)

func TestDirectInboxMoveUsesBoardTransactionAndRecurringSuccessor(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "actions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects(id,name,repo_path) VALUES(1,'Project','/tmp/project');INSERT INTO tasks(id,project_id,title,status,position,recurrent) VALUES(1,1,'Recurring task','in_progress',0,1),(2,1,'Already done','done',1,0)`); err != nil {
		t.Fatal(err)
	}
	r := &inboxActionRuntime{db: db, mgr: session.NewManager(), wt: worktree.NewService(t.TempDir())}
	ctx := context.Background()
	if err = r.Move(ctx, 1, "in_progress", "done"); err != nil {
		t.Fatal(err)
	}
	var state string
	var count int
	db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&state)
	db.QueryRow(`SELECT count(*) FROM tasks WHERE recurrent=1 AND status='todo'`).Scan(&count)
	if state != "done" || count != 1 {
		t.Fatal(state, count)
	}
	if err = r.Move(ctx, 1, "in_progress", "done"); !errors.Is(err, notifications.ErrActionBlocked) {
		t.Fatal("stale move accepted", err)
	}
	db.QueryRow(`SELECT count(*) FROM tasks WHERE recurrent=1 AND status='todo'`).Scan(&count)
	if count != 1 {
		t.Fatal("recurring successor duplicated", count)
	}
}

func TestInboxOptionQueuesOnlyTheServerSavedAction(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "saved-action.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO notification_sources(id,provider,name,config,checkpoint,created_at) VALUES('source','github','GitHub','{}','',1);INSERT INTO notification_events(id,source_id,external_id,version,payload,created_at) VALUES(1,'source','event','v1','{}',1)`); err != nil {
		t.Fatal(err)
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{} })
	defer box.Close()
	seq := inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{{ID: "ignore", Kind: "ignore"}}}
	q, err := box.PublishNotification(context.Background(), 1, "GitHub", "https://github.com/acme/repo/pull/7", inbox.Create{Abstract: "Source update", Summary: "Source update", Context: "Review the source", Question: "Leave it alone?", Options: []inbox.Option{{ID: "skip", Label: "Ignore", Action: &seq, ActionSummary: seq.Summary()}}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	InboxRoutes(mux, box, "")
	call := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/inbox/"+q.ID+"/answer", strings.NewReader(body)))
		return w
	}
	bad := call(fmt.Sprintf(`{"revision":%d,"option_id":"skip","action":{"kind":"delete_task"}}`, q.Revision))
	if bad.Code != 400 {
		t.Fatal("client action payload accepted", bad.Code, bad.Body.String())
	}
	body := fmt.Sprintf(`{"revision":%d,"option_id":"skip"}`, q.Revision)
	for i := 0; i < 2; i++ {
		w := call(body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var count int
	var saved string
	db.QueryRow(`SELECT count(*),action FROM notification_runs WHERE id=?`, q.ID).Scan(&count, &saved)
	if count != 1 || !strings.Contains(saved, `"kind":"ignore"`) || strings.Contains(saved, "delete_task") {
		t.Fatal(count, saved)
	}
}

func TestDirectMoveDoesNotTreatStopRequestedAsConfirmedExit(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "stopping.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO projects(id,name,repo_path) VALUES(1,'Project','/tmp/project');INSERT INTO tasks(id,project_id,title,status,position) VALUES(1,1,'Task','in_progress',0)`); err != nil {
		t.Fatal(err)
	}
	mgr := session.NewManager()
	sess, err := mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "trap '' TERM; printf 'STOP_TEST_READY\\n'; while :; do sleep 1; done"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, sess, []byte("STOP_TEST_READY"))
	stopped := make(chan struct{})
	go func() { defer close(stopped); sess.Stop() }()
	defer func() { <-stopped }()
	deadline := time.Now().Add(time.Second)
	for !sess.Info().StopRequested {
		if time.Now().After(deadline) {
			t.Fatal("stop request not recorded")
		}
		time.Sleep(time.Millisecond)
	}
	r := &inboxActionRuntime{db: db, mgr: mgr, wt: worktree.NewService(t.TempDir())}
	if err = r.Move(context.Background(), 1, "in_progress", "done"); !errors.Is(err, notifications.ErrActionBlocked) {
		t.Fatal("task moved before process exit was confirmed", err)
	}
	var status string
	db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&status)
	if status != "in_progress" {
		t.Fatal(status)
	}
}

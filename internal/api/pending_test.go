package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/store"
)

func pendingFixture(t *testing.T) (*sessionHandlers, pendingFilter) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	h := &sessionHandlers{db: db, mgr: session.NewManager(), globRoot: t.TempDir()}
	t.Cleanup(func() {
		for _, info := range h.mgr.List() {
			if s, ok := h.mgr.Get(info.ID); ok {
				s.Stop()
			}
		}
		db.Close()
	})
	execPending(t, db, `INSERT INTO workspaces(id,name) VALUES(2,'Work')`)
	execPending(t, db, `INSERT INTO projects(id,name,repo_path,workspace_id,agent_id) VALUES
 (1,'One','/one',1,(SELECT id FROM agents WHERE engine='codex')),
 (2,'Two','/two',1,(SELECT id FROM agents WHERE engine='codex')),
 (3,'Three','/three',2,(SELECT id FROM agents WHERE engine='codex'))`)
	return h, pendingFilter{Scope: "workspace", WorkspaceID: 1, Statuses: []string{"in_review", "in_progress"}, TerminalStates: []string{"closed", "active"}}
}
func execPending(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func pendingTask(t *testing.T, h *sessionHandlers, id, project int, status string) {
	t.Helper()
	execPending(t, h.db, `INSERT INTO tasks(id,project_id,title,status,position,worktree_path,codex_session_id) VALUES(?,?,?, ?,0,?,'saved-conversation')`, id, project, "Card", status, t.TempDir())
}
func TestPendingScopeAndExclusions(t *testing.T) {
	h, f := pendingFixture(t)
	pendingTask(t, h, 1, 1, "in_review")
	pendingTask(t, h, 2, 2, "in_progress")
	pendingTask(t, h, 3, 3, "in_review")
	pendingTask(t, h, 4, 1, "done")
	pendingTask(t, h, 5, 1, "todo")
	pendingTask(t, h, 6, 1, "in_review")
	pendingTask(t, h, 7, 1, "in_review")
	execPending(t, h.db, `UPDATE tasks SET snoozed_until='2099-01-01T00:00:00Z' WHERE id=6`)
	execPending(t, h.db, `UPDATE tasks SET source='github_pr' WHERE id=7`)
	for _, tc := range []struct {
		scope    string
		project  int64
		statuses []string
		want     int
	}{
		{"workspace", 0, f.Statuses, 2}, {"project", 1, f.Statuses, 1}, {"anywhere", 0, f.Statuses, 3}, {"workspace", 0, []string{"in_review"}, 1},
	} {
		filter := f
		filter.Scope = tc.scope
		filter.ProjectID = tc.project
		filter.Statuses = tc.statuses
		got, err := h.pendingCandidates(filter, 0)
		if err != nil || len(got) != tc.want {
			t.Fatalf("%+v got %v (%v)", tc, got, err)
		}
	}
	// Codex's interactive --last fallback must not create a fresh batch session.
	execPending(t, h.db, `UPDATE tasks SET codex_session_id=NULL WHERE id=1`)
	_, reason, err := h.pendingCandidate(f, 1)
	if err != nil || reason != "No previous conversation to resume" {
		t.Fatal(reason, err)
	}
	sess, spawnErr := h.createSession(sessionCreateRequest{Kind: "agent", TaskID: 1, Resume: true, Pending: &f})
	if sess != nil || spawnErr == nil || spawnErr.status != 409 || len(h.mgr.List()) != 0 {
		t.Fatal("resume-only guard allowed a spawn", spawnErr)
	}
}
func TestPendingRechecksMovedAndSnoozedCards(t *testing.T) {
	h, f := pendingFixture(t)
	pendingTask(t, h, 1, 1, "in_review")
	q := &pendingQueue{ctx: context.Background(), sessions: h, poll: time.Millisecond, job: pendingJob{State: "running", Items: []pendingItem{{TaskID: 1, State: "queued"}}}}
	execPending(t, h.db, `UPDATE tasks SET status='done' WHERE id=1`)
	q.check(f, 0, 1)
	if q.job.Items[0].State != "skipped" || len(h.mgr.List()) != 0 {
		t.Fatal(q.job)
	}
	execPending(t, h.db, `UPDATE tasks SET status='in_review',snoozed_until='2099-01-01' WHERE id=1`)
	q.check(f, 0, 1)
	if q.job.Items[0].State != "skipped" || len(h.mgr.List()) != 0 {
		t.Fatal(q.job)
	}
}
func TestPendingActiveStatesAndInbox(t *testing.T) {
	h, f := pendingFixture(t)
	pendingTask(t, h, 1, 1, "in_progress")
	sess, err := h.mgr.Spawn(session.SpawnOpts{Kind: session.KindAgent, TaskID: 1, Cwd: t.TempDir(), AgentEngine: "custom", AgentArgs: []string{"sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	_, reason, err := h.pendingCandidate(f, 1)
	if err != nil || reason != "" {
		t.Fatal(reason, err)
	}
	sess.SetMonitorStatus(session.MonitorStatus{State: "waiting", ObservedAt: time.Now()})
	_, reason, err = h.pendingCandidate(f, 1)
	if err != nil || reason != "" {
		t.Fatal("waiting without inbox should remain eligible", reason, err)
	}
	execPending(t, h.db, `INSERT INTO inbox_questions(id,task_id,session_id,created_session_id,request_key,summary,context,question,options,status,created_at,updated_at,heartbeat_at) VALUES('q',1,?,?,'r','s','c','q','[]','pending',0,0,0)`, sess.Info().ID, sess.Info().ID)
	_, reason, err = h.pendingCandidate(f, 1)
	if err != nil || reason != "Agent already has a pending inbox message" {
		t.Fatal(reason, err)
	}
	sess.SetMonitorStatus(session.MonitorStatus{})
	if err = sess.WriteInput([]byte("x")); err != nil {
		t.Fatal(err)
	}
	_, reason, err = h.pendingCandidate(f, 1)
	if err != nil || reason != "Agent is working" {
		t.Fatal(reason, err)
	}
}
func TestPendingQueueConcurrencySpacingAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &pendingQueue{ctx: ctx, interval: 50 * time.Millisecond, poll: time.Millisecond, job: pendingJob{ID: "run-1", State: "running", Items: []pendingItem{{TaskID: 1, State: "queued"}, {TaskID: 2, State: "queued"}, {TaskID: 3, State: "queued"}, {TaskID: 4, State: "queued"}}}}
	starts := make(chan time.Time, 4)
	release := make(chan struct{}, 4)
	finished := make(chan struct{})
	go func() {
		q.run(func(i int, _ int64) { starts <- time.Now(); <-release; q.set(i, "done", "") })
		close(finished)
	}()
	receive := func() time.Time {
		select {
		case at := <-starts:
			return at
		case <-time.After(2 * time.Second):
			t.Fatal("worker never started")
			return time.Time{}
		}
	}
	first, second := receive(), receive()
	if second.Sub(first) < 45*time.Millisecond {
		t.Fatal("starts not spaced", second.Sub(first))
	}
	select {
	case <-starts:
		t.Fatal("more than two concurrent checks")
	case <-time.After(80 * time.Millisecond):
	}
	release <- struct{}{}
	receive()
	rec := httptest.NewRecorder()
	q.cancel(rec, httptest.NewRequest("POST", "/api/pending/cancel", strings.NewReader(`{"id":"run-1"}`)))
	release <- struct{}{}
	release <- struct{}{}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("queue never finished")
	}
	if q.job.State != "cancelled" || q.job.Items[3].State != "cancelled" {
		t.Fatal(q.job)
	}
	select {
	case <-starts:
		t.Fatal("cancelled task started")
	default:
	}
}
func TestPendingSpacingSurvivesConsecutiveRuns(t *testing.T) {
	q := &pendingQueue{ctx: context.Background(), interval: 70 * time.Millisecond, poll: time.Millisecond}
	var started []time.Time
	for n := 0; n < 2; n++ {
		q.job = pendingJob{State: "running", Items: []pendingItem{{TaskID: 1, State: "queued"}}}
		q.run(func(i int, _ int64) { started = append(started, time.Now()); q.set(i, "done", "") })
	}
	if started[1].Sub(started[0]) < 65*time.Millisecond {
		t.Fatal("separate runs bypassed startup spacing", started)
	}
}

func TestPendingValidationAndOverlappingRun(t *testing.T) {
	h, _ := pendingFixture(t)
	q := &pendingQueue{ctx: context.Background(), sessions: h, job: pendingJob{State: "running"}}
	for _, body := range []string{`{}`, `{"scope":"anywhere","statuses":["done"],"terminal_states":["active"]}`, `{"scope":"project","statuses":["in_review"],"terminal_states":["closed"]}`} {
		rec := httptest.NewRecorder()
		q.start(rec, httptest.NewRequest("POST", "/api/pending", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	q.start(rec, httptest.NewRequest("POST", "/api/pending", strings.NewReader(`{"scope":"anywhere","statuses":["in_review"],"terminal_states":["active"]}`)))
	if rec.Code != 409 {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
func TestPendingClosedOnlyResumesAndDeliversPrompt(t *testing.T) {
	h, f := pendingFixture(t)
	pendingTask(t, h, 1, 1, "in_review")
	// A real PTY stub supplies a ready input surface, then records raw input.
	// It stays open after answering, matching the requested terminal lifecycle.
	dir := t.TempDir()
	script := filepath.Join(dir, "agent")
	received := filepath.Join(dir, "received")
	argv := filepath.Join(dir, "argv")
	code := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argv + "'\nstty -echo -icanon min 1 time 0\nprintf '› \\n'\ncat > '" + received + "'\n"
	if err := os.WriteFile(script, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	execPending(t, h.db, `UPDATE agents SET command=? WHERE engine='codex'`, script)
	f.TerminalStates = []string{"closed"}
	q := &pendingQueue{ctx: context.Background(), sessions: h, poll: time.Millisecond, job: pendingJob{State: "running", Items: []pendingItem{{TaskID: 1, State: "checking"}}}}
	done := make(chan struct{})
	go func() { q.check(f, 0, 1); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(received)
		if strings.Contains(string(data), pendingPrompt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompt not delivered: %q", data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, err := os.ReadFile(argv)
	if err != nil || !strings.Contains(string(data), "resume\nsaved-conversation") {
		t.Fatal(string(data), err)
	}
	infos := h.mgr.ListByTask(1)
	if len(infos) != 1 {
		t.Fatal(infos)
	}
	sess, _ := h.mgr.Get(infos[0].ID)
	sess.SetMonitorStatus(session.MonitorStatus{State: "idle", ObservedAt: time.Now()})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("check did not finish")
	}
	if q.job.Items[0].State != "done" || sess.Info().Status != session.StatusRunning {
		t.Fatal(q.job, sess.Info())
	}
	var status string
	if err = h.db.QueryRow(`SELECT status FROM tasks WHERE id=1`).Scan(&status); err != nil || status != "in_review" {
		t.Fatal(status, err)
	}
	rec := httptest.NewRecorder()
	q.status(rec, httptest.NewRequest("GET", "/api/pending", nil))
	var job pendingJob
	if err = json.Unmarshal(rec.Body.Bytes(), &job); err != nil || job.Items[0].State != "done" {
		t.Fatal(rec.Body.String(), err)
	}
}

func TestPendingStaleCancelDoesNotAffectNewRun(t *testing.T) {
	q := &pendingQueue{ctx: context.Background(), job: pendingJob{ID: "run-2", State: "running", Items: []pendingItem{{State: "queued"}}}}
	rec := httptest.NewRecorder()
	q.cancel(rec, httptest.NewRequest("POST", "/api/pending/cancel", strings.NewReader(`{"id":"run-1"}`)))
	if rec.Code != 409 || q.job.State != "running" || q.job.Items[0].State != "queued" {
		t.Fatal(rec.Code, q.job)
	}
	rec = httptest.NewRecorder()
	q.cancel(rec, httptest.NewRequest("POST", "/api/pending/cancel", strings.NewReader(`{"id":"run-2"}`)))
	if rec.Code != 200 || q.job.State != "cancelling" || q.job.Items[0].State != "cancelled" {
		t.Fatal(rec.Code, q.job)
	}
}

func TestPendingActualDispatchSpacingAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &pendingQueue{ctx: ctx, interval: 60 * time.Millisecond}
	if !q.waitDispatch() {
		t.Fatal("initial dispatch rejected")
	}
	start := time.Now()
	if !q.waitDispatch() || time.Since(start) < 55*time.Millisecond {
		t.Fatal("actual dispatches were not spaced")
	}
	cancel()
	if q.waitDispatch() {
		t.Fatal("dispatch allowed after shutdown")
	}
}

func TestPendingRevalidatesAfterDispatchDelay(t *testing.T) {
	h, f := pendingFixture(t)
	pendingTask(t, h, 1, 1, "in_review")
	sess, err := h.createSession(sessionCreateRequest{TaskID: 1, Kind: "agent", Resume: true, Pending: &f, PendingBeforeSpawn: func() bool {
		execPending(t, h.db, `UPDATE tasks SET status='done' WHERE id=1`)
		return true
	}})
	if sess != nil || err == nil || err.status != 409 || len(h.mgr.List()) != 0 {
		t.Fatal("moved card resumed after dispatch delay", sess, err)
	}
}

type pendingSlowWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
}

func (w *pendingSlowWriter) Write(body []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.ResponseRecorder.Write(body)
}

func TestPendingSlowProgressClientDoesNotBlockWorkers(t *testing.T) {
	q := &pendingQueue{ctx: context.Background(), job: pendingJob{State: "running", Items: []pendingItem{{State: "queued"}}}}
	writer := &pendingSlowWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() { q.status(writer, httptest.NewRequest("GET", "/api/pending", nil)); close(done) }()
	<-writer.entered
	updated := make(chan struct{})
	go func() { q.set(0, "running", ""); close(updated) }()
	select {
	case <-updated:
	case <-time.After(time.Second):
		close(writer.release)
		<-done
		t.Fatal("slow client blocked worker")
	}
	close(writer.release)
	<-done
	var snapshot pendingJob
	if err := json.Unmarshal(writer.Body.Bytes(), &snapshot); err != nil || snapshot.Items[0].State != "queued" {
		t.Fatal("response did not retain its snapshot", snapshot, err)
	}
}

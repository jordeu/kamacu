package inbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kamacu/internal/store"
)

func fixture(t *testing.T) (*Service, Identity) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'Project','/tmp/example')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Task',0)`, `INSERT INTO tasks(id,project_id,title,position) VALUES(2,1,'Other task',1)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(db, func(id string) Session {
		switch id {
		case "one", "two":
			return Session{TaskID: 1, Live: true}
		case "other":
			return Session{TaskID: 2, Live: true}
		}
		return Session{}
	})
	t.Cleanup(svc.Close)
	return svc, Identity{1, "one"}
}
func create(t *testing.T, s *Service, who Identity, key string) Question {
	t.Helper()
	r, err := s.Create(context.Background(), who, Create{Key: key, Abstract: "Compact message", Summary: "Task summary", Context: "Tradeoffs and recommendation", Question: "Which option?", Options: []Option{{"a", "Option A"}, {"b", "Option B"}}})
	if err != nil {
		t.Fatal(err)
	}
	return r.Question
}
func TestCreateValidationIdentityAndIdempotency(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	q := create(t, s, who, "stable")
	if got := create(t, s, who, "stable"); got.ID != q.ID {
		t.Fatal("retry duplicated question")
	}
	if got := create(t, s, who, "another"); got.ID != q.ID {
		t.Fatal("second pending question created")
	}
	for _, bad := range []Identity{{0, "one"}, {2, "one"}, {1, "missing"}} {
		if _, err := s.Create(ctx, bad, Create{}); !errors.Is(err, ErrIdentity) {
			t.Fatalf("identity: %v", err)
		}
	}
	for _, c := range []Create{{Key: "k", Abstract: "Compact message", Summary: "s", Question: "q"}, {Key: "k", Abstract: "Compact message", Summary: "s", Context: "c", Question: "q", Options: []Option{{"a", "A"}, {"a", "B"}}}} {
		if _, err := s.Create(ctx, who, c); !errors.Is(err, ErrInvalid) {
			t.Fatalf("validation: %v", err)
		}
	}
}
func TestAnswerRaceAndAcknowledgment(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	q := create(t, s, who, "race")
	var wg sync.WaitGroup
	results := make(chan Result, 2)
	errs := make(chan error, 2)
	for _, source := range []*Identity{nil, &who} {
		wg.Add(1)
		go func(source *Identity) {
			defer wg.Done()
			answer := "inbox"
			if source != nil {
				answer = "terminal"
			}
			r, err := s.Answer(ctx, source, q.ID, q.Revision, answer, "")
			results <- r
			errs <- err
		}(source)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	accepted := 0
	winner := ""
	for r := range results {
		if r.Outcome == "accepted" {
			accepted++
		}
		if winner == "" {
			winner = r.Question.Answer
		}
		if r.Question.Answer != winner {
			t.Fatal("answers disagreed")
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d answers", accepted)
	}
	r, err := s.Acknowledge(ctx, who, q.ID, q.Revision)
	if err != nil || r.Question.AcknowledgedAt == nil {
		t.Fatalf("ack: %+v %v", r, err)
	}
	page, err := s.List(ctx, nil, true, 0)
	if err != nil || len(page.Items) != 1 || page.PendingCount != 0 {
		t.Fatalf("archive: %+v %v", page, err)
	}
}
func TestPendingSurvivesWithoutPolling(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	q := create(t, s, who, "no-poll")
	now := s.now()
	s.now = func() time.Time { return now.Add(24 * time.Hour) }
	page, err := s.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("pending: %+v %v", page, err)
	}
	r, err := s.Answer(ctx, nil, q.ID, q.Revision, "late answer", "")
	if err != nil || r.Outcome != "accepted" {
		t.Fatalf("answer: %+v %v", r, err)
	}
}

func expireSession(t *testing.T, s *Service, sid string) {
	t.Helper()
	lookup := s.lookup
	s.lookup = func(id string) Session {
		if id == sid {
			return Session{}
		}
		return lookup(id)
	}
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.lookup = lookup
}

func TestSessionTransferAndDismissal(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	clock := time.Now()
	s.now = func() time.Time { return clock }
	q := create(t, s, who, "transfer")
	expireSession(t, s, who.SessionID)
	next := Identity{1, "two"}
	r, err := s.Reopen(ctx, next, q.ID, q.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.Answer(ctx, nil, q.ID, q.Revision, "old browser click", "")
	if err != nil || stale.Outcome != "stale_revision" || stale.Question.Status != "pending" {
		t.Fatalf("stale form: %+v %v", stale, err)
	}
	if _, err = s.Answer(ctx, &who, q.ID, r.Question.Revision, "stale owner", ""); !errors.Is(err, ErrOwner) {
		t.Fatalf("old owner: %v", err)
	}
	if _, err = s.Reopen(ctx, Identity{2, "other"}, q.ID, r.Question.Revision, false); !errors.Is(err, ErrOwner) {
		t.Fatalf("other task: %v", err)
	}
	r, err = s.Dismiss(ctx, q.ID, r.Question.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reopen(ctx, next, q.ID, r.Question.Revision, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("dismissed reopened: %v", err)
	}
	r, err = s.Reopen(ctx, next, q.ID, r.Question.Revision, true)
	if err != nil || r.Question.Status != "pending" {
		t.Fatalf("user reopen: %+v %v", r, err)
	}
	var history int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM inbox_events WHERE question_id=?`, q.ID).Scan(&history); err != nil || history != 5 {
		t.Fatalf("history=%d err=%v", history, err)
	}
}
func TestWaitWakesOnAnswerAndCancellationPreservesQuestion(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	q := create(t, s, who, "wait")
	waitCtx, cancel := context.WithCancel(ctx)
	ch := make(chan error, 1)
	go func() { _, err := s.Wait(waitCtx, who, q.ID, 20*time.Second); ch <- err }()
	cancel()
	select {
	case err := <-ch:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait ignored cancellation")
	}
	page, err := s.List(ctx, nil, false, 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("cancel archived question: %+v %v", page, err)
	}
	resultCh := make(chan Result, 1)
	go func() {
		r, err := s.Wait(ctx, who, q.ID, 20*time.Second)
		if err != nil {
			ch <- err
		} else {
			resultCh <- r
		}
	}()
	if _, err = s.Answer(ctx, nil, q.ID, q.Revision, "", "b"); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-resultCh:
		if r.Question.Answer != "Option B" {
			t.Fatal(r)
		}
	case err := <-ch:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("answer did not wake waiter")
	}
}
func TestStoppedSessionAndRestartRecovery(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	done := make(chan struct{})
	s.lookup = func(id string) Session {
		select {
		case <-done:
			return Session{}
		default:
			return Session{TaskID: 1, Live: true, Done: done}
		}
	}
	q := create(t, s, who, "stopped")
	close(done)
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		got, err := s.get(ctx, q.ID)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "unanswered" {
			if got.Reason != "session_stopped" {
				t.Fatal(got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session exit did not archive")
		}
		time.Sleep(time.Millisecond)
	}
	// A new process has no surviving task agents; durable pending rows must be archived.
	s.lookup = func(string) Session { return Session{TaskID: 1, Live: true} }
	create(t, s, who, "restart")
	restored := New(s.db, func(string) Session { return Session{} })
	defer restored.Close()
	if err := restored.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := restored.List(ctx, nil, true, 0)
	if err != nil || len(page.Items) != 2 || page.PendingCount != 0 {
		t.Fatalf("recovery: %+v %v", page, err)
	}
}
func TestAnswerExpiryRaceAndReopenPendingGuard(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	clock := time.Now()
	s.now = func() time.Time { return clock }
	q := create(t, s, who, "old")
	expireSession(t, s, who.SessionID)
	r, err := s.Answer(ctx, nil, q.ID, q.Revision, "late answer", "")
	if err != nil || r.Outcome != "already_resolved" || r.Question.Status != "unanswered" {
		t.Fatalf("late answer: %+v %v", r, err)
	}
	create(t, s, who, "new")
	if _, err = s.Reopen(ctx, who, q.ID, q.Revision, false); !errors.Is(err, ErrConflict) {
		t.Fatal("reopened second pending", err)
	}
}

func TestResumedSessionAcknowledgesDurableAnswer(t *testing.T) {
	s, original := fixture(t)
	ctx := context.Background()
	q := create(t, s, original, "recover-answer")
	if _, err := s.Answer(ctx, nil, q.ID, q.Revision, "Use option A", ""); err != nil {
		t.Fatal(err)
	}
	// Simulate the original process exiting before it received the inbox answer.
	s.lookup = func(id string) Session {
		switch id {
		case "two":
			return Session{TaskID: 1, Live: true}
		case "other":
			return Session{TaskID: 2, Live: true}
		}
		return Session{}
	}
	successor := Identity{TaskID: 1, SessionID: "two"}
	page, err := s.List(ctx, &successor, false, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Answer != "Use option A" {
		t.Fatalf("recovery: %+v %v", page, err)
	}
	if _, err = s.Acknowledge(ctx, Identity{2, "other"}, q.ID, q.Revision); !errors.Is(err, ErrOwner) {
		t.Fatalf("cross-task acknowledgment: %v", err)
	}
	if _, err = s.Acknowledge(ctx, original, q.ID, q.Revision); !errors.Is(err, ErrIdentity) {
		t.Fatalf("dead session acknowledgment: %v", err)
	}
	stale, err := s.Acknowledge(ctx, successor, q.ID, q.Revision+1)
	if err != nil || stale.Outcome != "stale_revision" || stale.Question.AcknowledgedAt != nil {
		t.Fatalf("stale ack: %+v %v", stale, err)
	}
	got, err := s.Acknowledge(ctx, successor, q.ID, q.Revision)
	if err != nil || got.Question.AcknowledgedAt == nil {
		t.Fatalf("ack: %+v %v", got, err)
	}
	if got.Question.SessionID != original.SessionID || got.Question.Answer != "Use option A" || got.Question.Status != "answered" {
		t.Fatalf("ack altered answer ownership: %+v", got)
	}
	again, err := s.Acknowledge(ctx, successor, q.ID, q.Revision)
	if err != nil || *again.Question.AcknowledgedAt != *got.Question.AcknowledgedAt {
		t.Fatalf("non-idempotent ack: %+v %v", again, err)
	}
	var count int
	var actor string
	if err = s.db.QueryRow(`SELECT COUNT(*),session_id FROM inbox_events WHERE question_id=? AND kind='acknowledged'`, q.ID).Scan(&count, &actor); err != nil || count != 1 || actor != "two" {
		t.Fatalf("ack history: %d %q %v", count, actor, err)
	}
}

func TestSuccessorCannotAcknowledgeUnansweredQuestions(t *testing.T) {
	s, owner := fixture(t)
	ctx := context.Background()
	q := create(t, s, owner, "pending-ack")
	successor := Identity{TaskID: 1, SessionID: "two"}
	if _, err := s.Acknowledge(ctx, successor, q.ID, q.Revision); !errors.Is(err, ErrOwner) {
		t.Fatalf("pending ack: %v", err)
	}
	if _, err := s.Dismiss(ctx, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Acknowledge(ctx, successor, q.ID, q.Revision); !errors.Is(err, ErrOwner) {
		t.Fatalf("dismissed ack: %v", err)
	}
}

func TestScratchpadFeedbackLifecycleAndIsolation(t *testing.T) {
	s, task := fixture(t)
	original := s.lookup
	s.lookup = func(id string) Session {
		if id == "scratch" || id == "resumed" {
			return Session{Global: true, Live: true}
		}
		if id == "taskless" {
			return Session{Live: true}
		}
		return original(id)
	}
	ctx := context.Background()
	who := Identity{TaskID: 0, SessionID: "scratch"}
	q := create(t, s, who, "global")
	if q.TaskID != 0 || q.ProjectID != 0 || q.TaskTitle != "Scratchpad" {
		t.Fatalf("scope: %+v", q)
	}
	if again := create(t, s, who, "global"); again.ID != q.ID {
		t.Fatal("retry duplicated")
	}
	for _, bad := range []Identity{{0, "taskless"}, {0, "one"}, {1, "scratch"}, {-1, "scratch"}} {
		if _, err := s.List(ctx, &bad, false, 0); !errors.Is(err, ErrIdentity) {
			t.Fatalf("identity %+v: %v", bad, err)
		}
	}
	if _, err := s.Answer(ctx, &task, q.ID, q.Revision, "wrong scope", ""); !errors.Is(err, ErrOwner) {
		t.Fatalf("cross scope: %v", err)
	}
	taskQ := create(t, s, task, "regular")
	page, err := s.List(ctx, &who, false, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != q.ID {
		t.Fatalf("list: %+v %v", page, err)
	}
	if _, err = s.Answer(ctx, nil, q.ID, q.Revision, "", "a"); err != nil {
		t.Fatal(err)
	}
	result, err := s.Wait(ctx, who, q.ID, 0)
	if err != nil || result.Question.Answer != "Option A" {
		t.Fatalf("poll: %+v %v", result, err)
	}
	resumed := Identity{0, "resumed"}
	if _, err = s.Acknowledge(ctx, resumed, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	q2 := create(t, s, who, "expire")
	expireSession(t, s, who.SessionID)
	result, err = s.Wait(ctx, who, q2.ID, 0)
	if err != nil || result.Question.Status != "unanswered" {
		t.Fatalf("expiry: %+v %v", result, err)
	}
	result, err = s.Reopen(ctx, resumed, q2.ID, result.Question.Revision, false)
	if err != nil || result.Question.SessionID != "resumed" {
		t.Fatalf("reopen: %+v %v", result, err)
	}
	if _, err = s.Answer(ctx, &resumed, q2.ID, result.Question.Revision, "terminal answer", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DELETE FROM tasks WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.get(ctx, q.ID); err != nil {
		t.Fatalf("scratchpad deleted with task: %v", err)
	}
	if _, err = s.get(ctx, taskQ.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("task cascade: %v", err)
	}
}

func TestCompletionActionsAndNextPrompt(t *testing.T) {
	for _, tc := range []struct{ name, answer, option, reason string }{
		{"next prompt", "Please add tests", "", ""},
		{"exit", "", "exit", "exit"},
		{"text is not exit", CompletionExit, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, who := fixture(t)
			ctx := context.Background()
			r, err := s.Complete(ctx, who, Create{Key: "finished", Abstract: "Compact message", Summary: "Implemented feature", Context: "Tests passed"})
			if err != nil {
				t.Fatal(err)
			}
			q := r.Question
			if q.Kind != "completion" || len(q.Options) != 1 || q.Options[0].ID != "exit" {
				t.Fatalf("completion: %+v", q)
			}
			retry, err := s.Complete(ctx, who, Create{Key: "finished", Abstract: "Compact message", Summary: "Implemented feature", Context: "Tests passed"})
			if err != nil || retry.Question.ID != q.ID {
				t.Fatalf("retry: %+v %v", retry, err)
			}
			r, err = s.Answer(ctx, nil, q.ID, q.Revision, tc.answer, tc.option)
			if err != nil || r.Question.Status != "answered" || r.Question.Reason != tc.reason {
				t.Fatalf("answer: %+v %v", r, err)
			}
			r, err = s.Wait(ctx, who, q.ID, 0)
			if err != nil || r.Question.Reason != tc.reason {
				t.Fatalf("poll: %+v %v", r, err)
			}
			if _, err = s.Acknowledge(ctx, who, q.ID, q.Revision); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAbstractValidationAndPersistence(t *testing.T) {
	for _, abstract := range []string{"", "   ", strings.Repeat("x", 181), strings.Repeat("x", 480), strings.Repeat("x", 481), strings.Repeat("界", 480), strings.Repeat("界", 481)} {
		t.Run(fmt.Sprintf("length-%d", len(abstract)), func(t *testing.T) {
			s, who := fixture(t)
			c := Create{Key: "abstract", Abstract: abstract, Summary: "Summary", Context: "Context", Question: "Proceed?"}
			r, err := s.Create(context.Background(), who, c)
			if strings.TrimSpace(abstract) == "" || len([]rune(abstract)) > 480 {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("expected invalid abstract, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			page, err := s.List(context.Background(), nil, false, 0)
			if err != nil || r.Question.Abstract != abstract || len(page.Items) != 1 || page.Items[0].Abstract != abstract {
				t.Fatalf("abstract not persisted: %+v %v", page, err)
			}
		})
	}
}

func TestGetMessageOutsideFirstPage(t *testing.T) {
	s, who := fixture(t)
	ctx := context.Background()
	q := create(t, s, who, "oldest")
	if _, err := s.Answer(ctx, nil, q.ID, q.Revision, "Yes", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		_, err := s.db.ExecContext(ctx, `INSERT INTO inbox_questions(id, task_id, session_id, created_session_id, request_key, summary, context, question, options, status, created_at, updated_at, heartbeat_at) VALUES(?,1,'one','one',?,'Summary','Context','Question','[]','answered',1,?,1)`, fmt.Sprintf("newer-%d", i), fmt.Sprintf("newer-%d", i), s.now().UnixMilli()+int64(i)+1)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.List(ctx, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || len(page.Items) != 50 {
		t.Fatalf("expected full first page: %+v", page)
	}
	for _, item := range page.Items {
		if item.ID == q.ID {
			t.Fatal("target unexpectedly on first page")
		}
	}
	got, err := s.Get(ctx, q.ID)
	if err != nil || got.ID != q.ID || got.Status != "answered" {
		t.Fatalf("get older archived message: %+v %v", got, err)
	}
}

func TestPendingCompletionsSnapshot(t *testing.T) {
	s, _ := fixture(t)
	s.lookup = func(id string) Session {
		if id == "scratch" {
			return Session{Global: true, Live: true}
		}
		if id == "other" {
			return Session{TaskID: 2, Live: true}
		}
		return Session{TaskID: 1, Live: true}
	}
	ctx := context.Background()
	complete := func(who Identity) Question {
		t.Helper()
		result, err := s.Complete(ctx, who, Create{Key: "done", Abstract: "Done", Summary: "Done", Context: "Verified"})
		if err != nil {
			t.Fatal(err)
		}
		return result.Question
	}
	for i := 0; i < 60; i++ {
		complete(Identity{1, fmt.Sprintf("session-%d", i)})
	}
	create(t, s, Identity{1, "question"}, "question")
	complete(Identity{2, "other"})
	scratch := complete(Identity{0, "scratch"})
	archived := complete(Identity{1, "archived"})
	if _, err := s.Dismiss(ctx, archived.ID, archived.Revision); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.PendingCompletions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 60 {
		t.Fatalf("got %d completions, want 60", len(snapshot))
	}
	for _, q := range snapshot {
		if q.TaskID != 1 || q.Kind != "completion" || q.Status != "pending" {
			t.Fatalf("unexpected message: %+v", q)
		}
	}
	// Later reports are not part of the captured IDs. Reopening a captured
	// completion must still be protected by the ordinary dismiss revision guard.
	later := complete(Identity{1, "later"})
	first := snapshot[0]
	if _, err := s.Dismiss(ctx, first.ID, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reopen(ctx, Identity{1, first.SessionID}, first.ID, first.Revision, true); err != nil {
		t.Fatal(err)
	}
	for _, q := range snapshot {
		if _, err := s.Dismiss(ctx, q.ID, q.Revision); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := s.PendingCompletions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("want later and reopened reports, got %+v", remaining)
	}
	for _, q := range remaining {
		if q.ID != later.ID && q.ID != first.ID {
			t.Fatalf("unexpected remaining message: %+v", q)
		}
	}
	global, err := s.PendingCompletions(ctx, 0)
	if err != nil || len(global) != 1 || global[0].ID != scratch.ID {
		t.Fatalf("scratchpad snapshot: %+v, %v", global, err)
	}
	if _, err := s.PendingCompletions(ctx, -1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid task: %v", err)
	}
}

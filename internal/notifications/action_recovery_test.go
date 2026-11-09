package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inboxaction"
)

func TestJournalRetriesTheSameResultAfterConnectionWait(t *testing.T) {
	for _, mutation := range []bool{false, true} {
		name := "read"
		if mutation {
			name = "mutation"
		}
		t.Run(name, func(t *testing.T) {
			s, _, _, _, source := directFixture(t)
			ctx := context.Background()
			q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
				t.Fatal(err)
			}
			ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action, history: []trace{}}
			calls := 0
			released := make(chan struct{})
			result, err := ex.step(ctx, ex.seq.Actions[0], "run/test", "test_recording", map[string]any{"task_id": 1}, mutation, func() (string, error) {
				calls++
				held, e := s.DB.Conn(ctx)
				if e != nil {
					return "", e
				}
				go func() { defer close(released); time.Sleep(5300 * time.Millisecond); held.Close() }()
				return `{"id":1}`, nil
			})
			<-released
			if err != nil || calls != 1 || result != `{"id":1}` {
				t.Fatal(result, err, calls)
			}
			var state string
			if err = s.DB.QueryRow(`SELECT state FROM notification_action_steps WHERE run_id=? AND step_key='run/test'`, q.ID).Scan(&state); err != nil || state != "succeeded" {
				t.Fatal(state, err)
			}
		})
	}
}

func TestLostReadResultAutomaticallyResumesWithoutAnotherInboxQuestion(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "review", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7}})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	// A readiness result was lost after confirmed setup. This is exactly the
	// observed PR 1950 failure, without any prompt-submission intent.
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?;INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,task_id,session_id,started_at) VALUES(?,'review/pr/7/start','review','start_task_agent',1,'{"task_id":2}','succeeded','{"id":"surviving","taskId":2}',2,'surviving',1),(?,'review/pr/7/ready','review','wait_ready',0,'{"task_id":2,"session_id":"surviving"}','running','',0,'',2)`, q.ID, q.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	// Register the actual review task as the one the successful open will return.
	if _, err := s.DB.Exec(`UPDATE tasks SET project_id=1,source='github_pr',pr_number=7 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	r.sessions["surviving"] = ActionSession{ID: "surviving", TaskID: 2, Revision: 1}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || len(pending.Items) != 0 || len(m.prompts) != 0 {
		t.Fatal(out, r.submitted, pending, m.prompts)
	}
	for _, call := range tools.calls {
		if call == "start_task_agent" {
			t.Fatal("known session startup repeated")
		}
	}
}

func TestRecordingBackoffIsDurableAndBounded(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action, retrySafe: true}
	cause := &recordingError{phase: "read_result", err: context.DeadlineExceeded}
	if err := ex.deferRecovery(cause); err != nil {
		t.Fatal(err)
	}
	var state string
	var attempts int
	var due int64
	s.DB.QueryRow(`SELECT state,recovery_attempts,next_attempt FROM notification_runs WHERE id=?`, q.ID).Scan(&state, &attempts, &due)
	if state != "queued" || attempts != 1 || due <= time.Now().UnixMilli() {
		t.Fatal(state, attempts, due)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	var unchanged int
	s.DB.QueryRow(`SELECT recovery_attempts FROM notification_runs WHERE id=?`, q.ID).Scan(&unchanged)
	if unchanged != attempts {
		t.Fatal("restart reset retry budget", unchanged)
	}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running',recovery_attempts=? WHERE id=?`, maxActionRecoveries, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := ex.deferRecovery(cause); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if out.DeliveryStatus != "failed" || len(pending.Items) != 1 || pending.Items[0].Reason != "operational_failure" {
		t.Fatal(out, pending)
	}
	if strings.Contains(pending.Items[0].Question, "?") || !strings.Contains(pending.Items[0].Question, "No reply is required") {
		t.Fatal(pending.Items[0])
	}
}

func TestHistoryCacheFailureDoesNotInvalidateConfirmedExecution(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_history_cache BEFORE UPDATE OF history ON notification_runs BEGIN SELECT RAISE(ABORT,'cache unavailable');END`); err != nil {
		t.Fatal(err)
	}
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || len(m.prompts) != 0 {
		t.Fatal(out, r.submitted, m.prompts)
	}
}

func TestLegacyReadOnlyUncertaintyIsReconciledButHandledNoticesAreNot(t *testing.T) {
	for _, handled := range []bool{false, true} {
		name := "pending"
		if handled {
			name = "already_handled"
		}
		t.Run(name, func(t *testing.T) {
			s, _, _, _, source := directFixture(t)
			ctx := context.Background()
			q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
				t.Fatal(err)
			}
			if err := s.finishRun(ctx, q.ID, "uncertain", "Inbox action uncertain: cannot record step result: context deadline exceeded", nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,'run/ready','run','wait_ready',0,'{}','running',1)`, q.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.deliverActionAttention(ctx, q.ID); err != nil {
				t.Fatal(err)
			}
			pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			notice := pending.Items[0]
			if handled {
				if _, err := s.Inbox.Dismiss(ctx, notice.ID, notice.Revision); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			var state string
			s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&state)
			want := "queued"
			if handled {
				want = "uncertain"
			}
			if state != want {
				t.Fatal(state, want)
			}
			if !handled {
				archived, _ := s.Inbox.Get(ctx, notice.ID)
				if archived.Status != "unanswered" || archived.Reason != "automatic_recovery" {
					t.Fatal(archived)
				}
				// A real later failure restores the same card with current facts, not a
				// duplicate or the obsolete uncertain-outcome explanation.
				if err := s.finishRun(ctx, q.ID, "blocked", "A real terminal condition needs attention", nil); err != nil {
					t.Fatal(err)
				}
				if err := s.deliverActionAttention(ctx, q.ID); err != nil {
					t.Fatal(err)
				}
				refreshed, _ := s.Inbox.Get(ctx, notice.ID)
				if refreshed.Status != "pending" || refreshed.Reason == "automatic_recovery" || strings.Contains(refreshed.Context, "may already have taken effect") {
					t.Fatal(refreshed)
				}
			}
		})
	}
}

func TestRecordingFailureCannotQueueAnUnresolvedMutation(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,'run/submit','run','send_session_message',1,'{}','running',1)`, q.ID); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action}
	if err := ex.deferRecovery(&recordingError{phase: "read_result", err: context.DeadlineExceeded}); err != nil {
		t.Fatal(err)
	}
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&state)
	if state != "uncertain" {
		t.Fatal(state)
	}
}

func TestFailedMutationRecordingNeverBecomesAutomaticReadRecovery(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER lose_mutation_result BEFORE UPDATE ON notification_action_steps WHEN OLD.kind='test_mutation' BEGIN SELECT RAISE(ABORT,'result unavailable');END`); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action}
	calls := 0
	_, err := ex.step(ctx, ex.seq.Actions[0], "run/mutate", "test_mutation", map[string]any{"task_id": 1}, true, func() (string, error) { calls++; return `{"id":1}`, nil })
	if !errors.Is(err, errActionUncertain) || calls != 1 {
		t.Fatal(err, calls)
	}
	if err = ex.finish(err); err != nil {
		t.Fatal(err)
	}
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&state)
	if state != "uncertain" {
		t.Fatal(state)
	}
}

func TestCleanShutdownDuringReadQueuesRecoveryWithoutAnInboxQuestion(t *testing.T) {
	s, m, _, _, source := directFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action, context: ctx}
	_, err := ex.step(ctx, ex.seq.Actions[0], "run/ready", "wait_ready", map[string]any{}, false, func() (string, error) { cancel(); return "", ErrActionBlocked })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = ex.finish(err); err != nil {
		t.Fatal(err)
	}
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&state)
	pending, _ := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if state != "queued" || len(pending.Items) != 0 || len(m.prompts) != 0 {
		t.Fatal(state, pending, m.prompts)
	}
}

func TestUnexecutedIntentKeepsItsCanonicalStepKey(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,failure_class,started_at) VALUES(?,'run/test','run','test_mutation',1,'{"task_id":1}','blocked','intent',1),(?,'run/complete','run','operation_complete',0,'{}','blocked','intent',2)`, q.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action}
	calls := 0
	for i := 0; i < 2; i++ {
		if _, err := ex.step(ctx, ex.seq.Actions[0], "run/test", "test_mutation", map[string]any{"task_id": 1}, true, func() (string, error) { calls++; return `{"id":1}`, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ex.step(ctx, ex.seq.Actions[0], "run/complete", "operation_complete", map[string]any{}, false, func() (string, error) { return `{"id":1}`, nil }); err != nil {
		t.Fatal(err)
	}
	var canonical, aliases int
	s.DB.QueryRow(`SELECT count(*) FROM notification_action_steps WHERE run_id=? AND state='succeeded' AND step_key IN ('run/test','run/complete')`, q.ID).Scan(&canonical)
	s.DB.QueryRow(`SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key LIKE '%/recheck_%'`, q.ID).Scan(&aliases)
	if calls != 1 || canonical != 2 || aliases != 0 {
		t.Fatal(calls, canonical, aliases)
	}
}

func TestRecoveryRecognizesCompletedWorkBeforeExhaustion(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running',recovery_attempts=? WHERE id=?`, maxActionRecoveries, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,started_at) VALUES(?,'run/complete','run','operation_complete',0,'{}','succeeded','{"id":1}',1)`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if out.DeliveryStatus != "succeeded" || len(pending.Items) != 0 {
		t.Fatal(out, pending)
	}
}

func TestRestartPreservesPermanentGuardFailureWithoutRetry(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?;INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,failure_class,started_at) VALUES(?,'run/submit','run','send_session_message',1,'{"task_id":1}','blocked','Inbox action requires attention: the started session received new input','guard',1)`, q.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(pending.Items) != 1 || len(m.prompts) != 0 || len(r.submitted) != 0 || len(tools.calls) != 0 {
		t.Fatal(pending, m.prompts, r.submitted, tools.calls)
	}
	for _, option := range pending.Items[0].Options {
		if option.ID == "retry_remaining" {
			t.Fatal("restart restored an impossible retry")
		}
	}
}

func TestMutationRechecksTargetAfterPersistingIntent(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	// Model a task changed by another writer while the intent write was waiting.
	if _, err := s.DB.Exec(`CREATE TRIGGER change_target_after_intent AFTER INSERT ON notification_action_steps WHEN NEW.kind='test_mutation_guard' BEGIN UPDATE tasks SET status='done' WHERE id=1;END`); err != nil {
		t.Fatal(err)
	}
	ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action, retrySafe: true}
	calls := 0
	_, err := ex.step(ctx, ex.seq.Actions[0], "run/guard", "test_mutation_guard", map[string]any{"task_id": 1}, true, func() (string, error) { calls++; return `{"id":1}`, nil })
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_action_steps WHERE run_id=? AND step_key='run/guard'`, q.ID).Scan(&state)
	if !errors.Is(err, ErrActionBlocked) || errors.Is(err, errActionUncertain) || calls != 0 || state != "blocked" || ex.retrySafe {
		t.Fatal(err, calls, state, ex.retrySafe)
	}
}

func TestLostCompletionMarkerKeepsItsCanonicalProof(t *testing.T) {
	for _, class := range []string{"interrupted_read", "read_result"} {
		t.Run(class, func(t *testing.T) {
			s, _, _, _, source := directFixture(t)
			ctx := context.Background()
			q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,failure_class,started_at) VALUES(?,'run/complete','run','operation_complete',0,'{}','blocked',?,1)`, q.ID, class); err != nil {
				t.Fatal(err)
			}
			ex := &actionExecutor{s: s, q: q, seq: *q.Options[0].Action}
			calls := 0
			for i := 0; i < 2; i++ {
				if _, err := ex.step(ctx, ex.seq.Actions[0], "run/complete", "operation_complete", map[string]any{}, false, func() (string, error) { calls++; return `{"id":1}`, nil }); err != nil {
					t.Fatal(err)
				}
			}
			var count int
			s.DB.QueryRow(`SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key='run/complete' AND state='succeeded'`, q.ID).Scan(&count)
			if calls != 1 || count != 1 {
				t.Fatal(calls, count)
			}
		})
	}
}

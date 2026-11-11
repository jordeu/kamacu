package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
)

type observingRuntime struct {
	*directRuntime
	terminal string
}

func (r *observingRuntime) ObserveHandoff(ctx context.Context, id string) (HandoffObservation, error) {
	v, e := r.Session(ctx, id)
	return HandoffObservation{ActionSession: v, Live: e == nil, Status: "waiting", Terminal: r.terminal}, e
}
func handoffFixture(t *testing.T, extra ...inboxaction.Action) (*Service, *fakeModel, *observingRuntime, inbox.Question) {
	t.Helper()
	s, m, r, _, source := directFixture(t)
	o := &observingRuntime{directRuntime: r, terminal: "Fixed all three findings, ran tests and pushed commit abc. Watch CI?"}
	s.Actions = o
	r.sessions["agent"] = ActionSession{ID: "agent", TaskID: 1}
	r.readyErr = fmt.Errorf("%w: agent needs attention", ErrActionBlocked)
	actions := append([]inboxaction.Action{{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Fix all three findings, run tests and push"}}, extra...)
	q := directCard(t, s, source, actions...)
	runDirect(t, s, q)
	var notice string
	if err := s.DB.QueryRow(`SELECT attention_question_id FROM notification_runs WHERE id=?`, q.ID).Scan(&notice); err != nil {
		t.Fatal(err)
	}
	n, err := s.Inbox.Get(context.Background(), notice)
	if err != nil {
		t.Fatal(err)
	}
	return s, m, o, n
}
func assessPending(t *testing.T, s *Service) {
	t.Helper()
	ctx := context.Background()
	if err := s.reconcileHandoffs(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE notification_handoffs SET changed_at=1,assessed_at=0`); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcileHandoffs(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestHandoffManualCompletionResolvesNoticeWithoutSending(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1: fixes were pushed", Explanation: "The requested fixes, tests and push were handled manually. Watching CI is a separate decision.", Question: "No further instructions are needed for task 1.", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	assessPending(t, s)
	got, err := s.Inbox.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unanswered" || got.Reason != "handoff_resolved" || len(r.submitted) != 0 || len(m.prompts) != 1 {
		t.Fatal(got, r.submitted, m.prompts)
	}
	assessPending(t, s)
	if len(m.prompts) != 1 {
		t.Fatal("assessment repeated")
	}
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, n.AttentionSourceID).Scan(&state)
	if state == "succeeded" {
		t.Fatal("reconciliation fabricated delivery")
	}
}
func TestHandoffDecisionReplacesRetryWithSpecificQuestion(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	r.terminal = "Task 1: Should I push commit abc or keep it local?"
	m.responses = []any{handoffAssessment{Outcome: "decision_needed", Summary: "Task 1 is waiting for permission to push", Explanation: "The requested push has not happened.", Question: "Open task 1 and choose whether to push commit abc.", Evidence: "Should I push commit abc or keep it local?"}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || !strings.Contains(got.Question, "commit abc") || got.Revision <= n.Revision {
		t.Fatal(got)
	}
	for _, opt := range got.Options {
		if opt.ID == "retry_remaining" || strings.HasPrefix(opt.ID, "send_pending_") {
			t.Fatal("unsafe retry", opt)
		}
	}
	assessPending(t, s)
	if len(m.prompts) != 1 {
		t.Fatal("unchanged evidence re-assessed")
	}
}
func TestHandoffChangedDuringAssessmentCannotResolveNotice(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 complete", Explanation: "Work completed", Question: "No work remains for task 1.", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	m.onGenerate = func(string) {
		v := r.sessions["agent"]
		v.Input++
		r.sessions["agent"] = v
		r.terminal = "Actually the push failed."
	}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || got.Revision != n.Revision {
		t.Fatal(got)
	}
}
func TestHandoffModelFailureProducesHonestNoticeWithoutRepeatedCalls(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	m.respond = func(string) (any, error) { return nil, errors.New("offline") }
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if !strings.Contains(got.Summary, "could not be reassessed") || got.Status != "pending" {
		t.Fatal(got)
	}
	assessPending(t, s)
	if len(m.prompts) != 1 {
		t.Fatal("failed assessment repeated")
	}
}
func TestHandoffOutstandingRequiresExplicitGuardedSingleInstruction(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	r.terminal = "Task 1: no fixes made; still waiting for the instructions."
	m.responses = []any{handoffAssessment{Outcome: "outstanding", Summary: "Task 1 still needs the fixes", Explanation: "No fixes were started.", Question: "Send the pending instructions to task 1?", Evidence: "no fixes made; still waiting for the instructions."}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if len(r.submitted) != 0 {
		t.Fatal("automatic send")
	}
	var option *inbox.Option
	for i := range got.Options {
		if strings.HasPrefix(got.Options[i].ID, "send_pending_") {
			option = &got.Options[i]
		}
	}
	if option == nil || option.Action.Guards.ResumeRunID != "" || len(option.Action.Actions) != 1 || option.Action.Actions[0].SessionID != "agent" {
		t.Fatal(got.Options)
	}
	// New terminal input after publication makes the newly authorized send stale.
	v := r.sessions["agent"]
	v.Input++
	r.sessions["agent"] = v
	if _, err := s.Inbox.Answer(context.Background(), nil, got.ID, got.Revision, "", option.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.submitted) != 0 {
		t.Fatal("sent after external input")
	}
}

func TestHandoffTrackingReopensSameNoticeForConcreteDecision(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	r.terminal = "Task 1: fixing the three findings now."
	m.responses = []any{handoffAssessment{Outcome: "being_handled", Summary: "Task 1 is addressing the findings", Explanation: "The same requested work is in progress.", Question: "Task 1 is working; no reply is needed.", Evidence: "fixing the three findings now."}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "unanswered" {
		t.Fatal(got)
	}
	r.terminal = "Task 1: findings fixed. Should I push abc?"
	m.responses = []any{handoffAssessment{Outcome: "decision_needed", Summary: "Task 1 needs a push decision", Explanation: "The push is still pending.", Question: "Open task 1 and decide whether to push abc.", Evidence: "Should I push abc?"}}
	assessPending(t, s)
	got, _ = s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || !strings.Contains(got.Question, "push abc") {
		t.Fatal(got)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions WHERE status='pending'`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate notices", count)
	}
}
func TestHandoffResolvedSiblingCannotHideUnfinishedInstruction(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	var run string
	s.DB.QueryRow(`SELECT run_id FROM notification_handoffs LIMIT 1`).Scan(&run)
	if err := s.recordHandoff(context.Background(), run, "second", 1, "agent", "Do a separate second thing", "not ready"); err != nil {
		t.Fatal(err)
	}
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 fixes complete", Explanation: "Fixes were pushed.", Question: "No further fixes are needed.", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	// Assess only the first handoff to observe the intermediate state.
	s.reconcileHandoffs(context.Background())
	s.DB.Exec(`UPDATE notification_handoffs SET changed_at=1 WHERE step_key!='second'`)
	if err := s.reconcileHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" {
		t.Fatal("unfinished sibling hidden", got)
	}
}
func TestHandoffBackfillsExistingReadinessNoticeAndPreservesAnswers(t *testing.T) {
	s, _, _, n := handoffFixture(t)
	if _, err := s.DB.Exec(`DELETE FROM notification_handoffs`); err != nil {
		t.Fatal(err)
	}
	if err := s.backfillHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM notification_handoffs`).Scan(&count)
	if count != 1 {
		t.Fatal("old notice not imported", count)
	}
	s.DB.Exec(`DELETE FROM notification_handoffs`)
	if _, err := s.Inbox.Answer(context.Background(), nil, n.ID, n.Revision, "", "ignore"); err != nil {
		t.Fatal(err)
	}
	if err := s.backfillHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT count(*) FROM notification_handoffs`).Scan(&count)
	if count != 0 {
		t.Fatal("answered history resurrected", count)
	}
}
func TestHandoffCannotUseTaskStatusAsCompletionEvidence(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "All done", Explanation: "Task is in progress", Question: "No action needed.", Evidence: "in_progress"}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || !strings.Contains(got.Summary, "could not be reassessed") {
		t.Fatal(got)
	}
}

func TestHandoffExplicitResendUsesExistingSessionWithoutModel(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	r.terminal = "Task 1: no fixes made; waiting for instructions."
	m.responses = []any{handoffAssessment{Outcome: "outstanding", Summary: "Task 1 needs the fixes", Explanation: "No fixes were started.", Question: "Send the pending instructions to task 1?", Evidence: "no fixes made; waiting for instructions."}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	r.readyErr = nil
	var id string
	for _, option := range got.Options {
		if strings.HasPrefix(option.ID, "send_pending_") {
			id = option.ID
		}
	}
	if id == "" {
		t.Fatal(got.Options)
	}
	if _, err := s.Inbox.Answer(context.Background(), nil, got.ID, got.Revision, "", id); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.submitted) != 1 || len(m.prompts) != 1 {
		t.Fatal(r.submitted, m.prompts)
	}
}
func TestHandoffRedrawDoesNotInvalidateAssessment(t *testing.T) {
	s, m, r, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 done", Explanation: "Fixes were handled.", Question: "No more instructions needed for task 1.", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	m.onGenerate = func(string) { v := r.sessions["agent"]; v.Revision++; r.sessions["agent"] = v }
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "unanswered" {
		t.Fatal(got)
	}
}

func TestHandoffReviewResendRetainsReviewWorkflow(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	observer := &observingRuntime{directRuntime: r, terminal: "Review has not started; waiting for review instructions."}
	s.Actions = observer
	r.readyErr = fmt.Errorf("%w: no safe prompt", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7}, Instructions: "Recheck only the new tests and report remaining findings"})
	runDirect(t, s, q)
	var notice string
	s.DB.QueryRow(`SELECT attention_question_id FROM notification_runs WHERE id=?`, q.ID).Scan(&notice)
	m.responses = []any{handoffAssessment{Outcome: "outstanding", Summary: "The review still needs its prompt", Explanation: "The review did not start.", Question: "Send the review instructions to the linked review task?", Evidence: "Review has not started; waiting for review instructions."}}
	assessPending(t, s)
	got, err := s.Inbox.Get(context.Background(), notice)
	if err != nil {
		t.Fatal(err)
	}
	var seq *inboxaction.Sequence
	for _, opt := range got.Options {
		if strings.HasPrefix(opt.ID, "send_pending_") {
			seq = opt.Action
		}
	}
	if seq == nil || len(seq.Actions) != 1 || seq.Actions[0].Kind != "start_reviews" || len(seq.Actions[0].PRNumbers) != 1 || seq.Actions[0].PRNumbers[0] != 7 || len(seq.Guards.Sessions) != 1 {
		t.Fatal(got.Options)
	}
	if seq.Actions[0].Instructions != "Recheck only the new tests and report remaining findings" || len(seq.Guards.HandoffSessions) != 1 {
		t.Fatal(seq)
	}
	var optionID string
	for _, option := range got.Options {
		if strings.HasPrefix(option.ID, "send_pending_") {
			optionID = option.ID
		}
	}
	r.readyErr = nil
	starts := 0
	for _, call := range tools.calls {
		if call == "start_task_agent" {
			starts++
		}
	}
	tools.before = func(name string) {
		if name == "list_sessions" {
			for id, v := range r.sessions {
				delete(r.sessions, id)
				r.sessions["replacement"] = ActionSession{ID: "replacement", TaskID: v.TaskID}
				break
			}
		}
	}
	if _, err = s.Inbox.Answer(context.Background(), nil, got.ID, got.Revision, "", optionID); err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := 0
	for _, call := range tools.calls {
		if call == "start_task_agent" {
			after++
		}
	}
	if after != starts {
		t.Fatal("replacement agent was started", tools.calls)
	}

	if len(r.submitted) != 0 {
		t.Fatal("automatic review submission")
	}
}

func TestHandoffUncertainDeliveryIsNeverClassifiedAsDelivered(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	var run, key string
	s.DB.QueryRow(`SELECT run_id,step_key FROM notification_handoffs`).Scan(&run, &key)
	_, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,?,'continue','send_session_message',1,'{}','uncertain',1)`, run, key+"/submit")
	if err != nil {
		t.Fatal(err)
	}
	assessPending(t, s)
	var state string
	s.DB.QueryRow(`SELECT state FROM notification_handoffs`).Scan(&state)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if state != "delivery_unknown" || got.Status != "pending" || len(m.prompts) != 0 {
		t.Fatal(state, got, m.prompts)
	}
}
func TestHandoffInterruptedAssessmentDoesNotRepeatInference(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	if err := s.reconcileHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := s.DB.Exec(`INSERT INTO notification_handoff_assessments(handoff_id,fingerprint,created_at) SELECT id,fingerprint,1 FROM notification_handoffs;UPDATE notification_handoffs SET changed_at=1`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.reconcileHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || !strings.Contains(got.Summary, "interrupted") || len(m.prompts) != 0 {
		t.Fatal(got, m.prompts)
	}
}

func TestHandoffCompletionPreservesOtherUnfinishedOperations(t *testing.T) {
	s, m, _, n := handoffFixture(t, inboxaction.Action{ID: "later", Kind: "update_task", ProjectID: 1, TaskID: 1, Title: "Follow-up title"})
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 fixes handled", Explanation: "The fixes were pushed.", Question: "No more fix instructions are needed.", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || !strings.Contains(got.Context, "update_task (later)") {
		t.Fatal(got)
	}
}

func TestHandoffCompletedSiblingKeepsConcreteDecision(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	var run string
	if err := s.DB.QueryRow(`SELECT run_id FROM notification_handoffs`).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if err := s.recordHandoff(context.Background(), run, "second", 1, "agent", "Push the second commit", "waiting"); err != nil {
		t.Fatal(err)
	}
	other := handoffAssessment{Outcome: "decision_needed", Summary: "Task 1 needs a decision about the second commit", Explanation: "The second push is pending.", Question: "Open task 1 and choose whether to push the second commit.", Evidence: "Second commit is waiting"}
	if _, err := s.DB.Exec(`UPDATE notification_handoffs SET state='decision_needed',assessment=? WHERE step_key='second'`, mustJSON(other)); err != nil {
		t.Fatal(err)
	}
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "First fixes complete", Explanation: "First fixes pushed", Question: "No first fixes remain", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	// The new sibling first gathers evidence; only the original is due.
	if err := s.reconcileHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE notification_handoffs SET changed_at=1 WHERE step_key!='second'`)
	if err := s.reconcileHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Inbox.Get(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" || got.Question != other.Question || got.Summary != other.Summary {
		t.Fatal(got)
	}
}
func TestHandoffRecordingFailureStillFinishesExecution(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	s.Actions = &observingRuntime{directRuntime: r, terminal: "Agent needs a choice"}
	r.sessions["agent"] = ActionSession{ID: "agent", TaskID: 1}
	r.readyErr = fmt.Errorf("%w: waiting for user choice", ErrActionBlocked)
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_handoff BEFORE INSERT ON notification_handoffs BEGIN SELECT RAISE(ABORT,'handoff table unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Finish the requested changes"})
	runDirect(t, s, q)
	run, err := s.ActionRun(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State == "running" || run.State == "queued" {
		t.Fatal("reconciliation recording stranded execution", run.State)
	}
	var notice string
	s.DB.QueryRow(`SELECT attention_question_id FROM notification_runs WHERE id=?`, q.ID).Scan(&notice)
	if notice == "" || len(r.submitted) != 0 {
		t.Fatal(notice, r.submitted)
	}
}
func TestHandoffAssessmentCannotOverrideNewDeliveryIntent(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 complete", Explanation: "Fixes pushed", Question: "No changes remain", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	m.onGenerate = func(string) {
		_, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) SELECT run_id,step_key||'/submit','continue','send_session_message',1,'{}','running',1 FROM notification_handoffs`)
		if err != nil {
			t.Fatal(err)
		}
	}
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "pending" || got.Revision != n.Revision {
		t.Fatal("assessment hid a new delivery attempt", got)
	}
}
func TestHandoffAssessmentWaitsForActionWorkerWithoutAnotherInference(t *testing.T) {
	s, m, _, n := handoffFixture(t)
	m.responses = []any{handoffAssessment{Outcome: "resolved", Summary: "Task 1 complete", Explanation: "Fixes pushed", Question: "No changes remain", Evidence: "Fixed all three findings, ran tests and pushed commit abc."}}
	locked := false
	m.onGenerate = func(string) { s.actionMu.Lock(); locked = true }
	defer func() {
		if locked {
			s.actionMu.Unlock()
		}
	}()
	assessPending(t, s)
	got, _ := s.Inbox.Get(context.Background(), n.ID)
	if got.Revision != n.Revision {
		t.Fatal(got)
	}
	s.actionMu.Unlock()
	locked = false
	assessPending(t, s)
	got, _ = s.Inbox.Get(context.Background(), n.ID)
	if got.Status != "unanswered" || len(m.prompts) != 1 {
		t.Fatal(got, m.prompts)
	}
}

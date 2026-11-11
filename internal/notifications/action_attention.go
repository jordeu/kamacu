package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
)

// Failures need a visible reply opportunity, not just an archived status. Retry
// is only offered when every attempted mutation has a definite outcome.
func (ex *actionExecutor) attention(ctx context.Context, state, result string) error {
	q, err := ex.s.Inbox.Get(ctx, ex.q.ID)
	if err != nil {
		return err
	}
	options := []inbox.Option{}
	var unknown int
	if err = ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND (state='uncertain' OR (is_mutation=1 AND state='running'))`, q.ID).Scan(&unknown); err != nil {
		return err
	}

	run, err := ex.s.ActionRun(ctx, q.ID)
	if err != nil {
		return err
	}
	if len(result) > 12000 {
		result = strings.ToValidUTF8(result[:12000], "") + "\n[More details are in Original execution steps.]"
	}
	canRetry := ex.retrySafe && unknown == 0 && state != "uncertain" && ex.seq.Validate() == nil

	uncertain := unknown > 0 || state == "uncertain"
	operational := run.RecoveryClass == "exhausted"
	if operational {
		canRetry = false
	}
	var replacement *inbox.Option
	copy := actionAttentionCopy(run, uncertain, canRetry, result)
	stale := false
	diagnosed := false
	if !uncertain {
		detail, option, e := ex.staleTaskAttention(ctx, run)
		if e != nil {
			return e
		}
		if detail != nil {
			copy = *detail
			replacement = option
			canRetry = false
			stale = true
		}
	}
	if !stale && !uncertain && (operational || copy.summary == "Your requested action could not finish" || strings.Contains(copy.summary, "could not complete the requested step")) {
		var diagnosis *failureDiagnosis
		// Concrete deterministic guard explanations need no model call. Unknown
		// internal/transport failures get one read-only diagnosis before a notice.
		if !strings.Contains(result, ErrActionBlocked.Error()+": ") {
			diagnosis = ex.s.diagnoseActionFailure(ctx, q, run, result, canRetry)
		}
		copy.summary = concreteFailureFallback(run, result)
		copy.question = copy.summary + "."
		if diagnosis != nil {
			diagnosed = true
			canRetry = canRetry && diagnosis.RetryRecommended
			copy.summary = diagnosis.Summary
			copy.context = diagnosis.Explanation + "\n\nNext step: " + diagnosis.NextStep + "\n\nRecorded failure: " + result
			copy.question = diagnosis.NextStep
		}
	}
	if operational {
		replacement = nil
		canRetry = false
		copy.summary = "Automatic action recovery stopped: database recording failed"
		copy.question = "Kamacu could not finish automatic recovery. No reply is required."
		details := result
		if diagnosed {
			details = copy.context
		}
		copy.context = "The database recording failure persisted beyond the automatic recovery budget. Recorded task/session setup is preserved. No uncertain mutation will be repeated. This is an operational failure, not a request for instructions.\n\n" + details
	}

	// Readiness failures have a durable pending handoff. Reassess the task before
	// offering any retry: the user may already have handled its instructions.
	if _, ok := ex.s.Actions.(handoffObserver); ok && !uncertain && !stale && !operational {
		var pending int
		if e := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_handoffs WHERE run_id=?`, q.ID).Scan(&pending); e != nil {
			return e
		}
		if pending > 0 {
			canRetry = false
			replacement = nil
			copy.summary = fmt.Sprintf("%d instruction(s) paused while the task agent needed attention", pending)
			copy.question = "Kamacu is checking whether the linked tasks still need these instructions."
			copy.context = "No pending instruction was sent. Kamacu will compare the saved instruction with the agent's current work and update this notice. Open a linked task to see its current terminal; no retry is needed while this check runs."
		}
	}

	if replacement != nil {
		options = append(options, *replacement)
	}

	if canRetry {
		retry := ex.seq
		retry.Guards.ResumeRunID = q.ID
		options = append(options, inbox.Option{ID: "retry_remaining", Label: copy.retryLabel, Action: &retry, ActionSummary: copy.retrySummary})
	}
	for _, link := range q.LinkedTasks {
		if len(options) >= 3 {
			break
		}
		seq := inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{{ID: "open", Kind: "open_task", ProjectID: link.ProjectID, TaskID: link.TaskID}}}
		options = append(options, inbox.Option{ID: fmt.Sprintf("open_%d", link.TaskID), Label: fmt.Sprintf("Inspect task %d", link.TaskID), Action: &seq, ActionSummary: seq.Summary()})
	}
	ignore := inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{{ID: "ignore", Kind: "ignore"}}}
	options = append(options, inbox.Option{ID: "ignore", Label: "Dismiss this notice", Action: &ignore, ActionSummary: "Close this notice. Keep existing tasks and sessions; do not start or cancel work."})

	context := copy.context + "\n\nOriginal request: " + q.Question + "\n" + q.Summary
	_, err = ex.s.Inbox.PublishNotificationAttention(ctx, q.ID, inbox.Create{Informational: operational, Abstract: copy.summary, Summary: copy.summary, Context: context, Question: copy.question, Options: options})
	return err
}

func (ex *actionExecutor) copyCompletedSteps(ctx context.Context) error {
	parent := ex.seq.Guards.ResumeRunID
	if parent == ex.q.ID {
		return errors.New("cannot resume an action from itself")
	}
	tx, err := ex.s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw, state, group, attentionQuestion string
	if err = tx.QueryRowContext(ctx, `SELECT r.action,r.state,COALESCE(NULLIF(q.notification_group,''),q.id),r.attention_question_id FROM notification_runs r JOIN inbox_questions q ON q.id=r.id WHERE r.id=? AND q.origin='notification'`, parent).Scan(&raw, &state, &group, &attentionQuestion); err != nil {
		return err
	}
	var currentGroup string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(notification_group,''),id) FROM inbox_questions WHERE id=?`, ex.q.ID).Scan(&currentGroup); err != nil {
		return err
	}
	if (group != currentGroup && attentionQuestion != ex.q.ID) || (state != "blocked" && state != "failed" && state != "partial") {
		return errors.New("only a stopped action from the same conversation can be resumed")
	}
	var original inboxaction.Sequence
	if err = json.Unmarshal([]byte(raw), &original); err != nil {
		return err
	}
	original.Guards.ResumeRunID = ""
	copy := ex.seq
	copy.Guards.ResumeRunID = ""
	if mustJSON(original) != mustJSON(copy) {
		return errors.New("retry cannot change the original saved action")
	}
	var unknown int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND (state='uncertain' OR (is_mutation=1 AND state='running'))`, parent).Scan(&unknown); err != nil {
		return err
	}
	if unknown > 0 {
		return errors.New("uncertain actions cannot be retried automatically")
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,task_id,session_id,started_at,finished_at) SELECT ?,step_key,operation_id,kind,is_mutation,arguments,state,result,task_id,session_id,started_at,finished_at FROM notification_action_steps WHERE run_id=? AND state='succeeded' AND kind!='diagnose_failure'`, ex.q.ID, parent); err != nil {
		return err
	}
	return tx.Commit()
}

// Deliver outbox notices both immediately and on worker/restart recovery. The
// database, rather than an in-memory error path, decides whether delivery is due.
func (s *Service) deliverPendingActionAttention(ctx context.Context) (bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM notification_runs WHERE attention_pending=1 AND (attention_question_id='' OR EXISTS(SELECT 1 FROM inbox_questions a WHERE a.id=notification_runs.attention_question_id AND a.reason='automatic_recovery' AND a.status='unanswered')) ORDER BY updated_at,id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, s.deliverActionAttention(ctx, id)
}
func (s *Service) deliverActionAttention(ctx context.Context, id string) error {
	q, err := s.Inbox.Get(ctx, id)
	if err != nil {
		return err
	}
	var raw, state string
	var retryable bool
	if err = s.DB.QueryRowContext(ctx, `SELECT action,state,attention_retryable FROM notification_runs WHERE id=?`, id).Scan(&raw, &state, &retryable); err != nil {
		return err
	}
	ex := &actionExecutor{s: s, q: q}
	if err = strictRuleJSON([]byte(raw), &ex.seq); err == nil && ex.seq.Validate() == nil {
		ex.retrySafe = retryable
	}
	return ex.attention(ctx, state, q.ExecutionResult)
}

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
	if ex.retrySafe && unknown == 0 && state != "uncertain" && ex.seq.Validate() == nil {
		retry := ex.seq
		retry.Guards.ResumeRunID = q.ID
		options = append(options, inbox.Option{ID: "retry_remaining", Label: "Retry remaining safe steps", Action: &retry, ActionSummary: "Retry after fixing the reported condition. Reuse recorded successful steps; do not repeat completed handoffs."})
	}
	for _, link := range q.LinkedTasks {
		if len(options) >= 3 {
			break
		}
		seq := inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{{ID: "open", Kind: "open_task", ProjectID: link.ProjectID, TaskID: link.TaskID}}}
		options = append(options, inbox.Option{ID: fmt.Sprintf("open_%d", link.TaskID), Label: fmt.Sprintf("Open task %d", link.TaskID), Action: &seq, ActionSummary: seq.Summary()})
	}
	ignore := inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{{ID: "ignore", Kind: "ignore"}}}
	options = append(options, inbox.Option{ID: "ignore", Label: "Leave this for me to handle", Action: &ignore, ActionSummary: ignore.Summary()})
	if len(result) > 12000 {
		result = strings.ToValidUTF8(result[:12000], "") + "\n[Full details are preserved in the original execution trace.]"
	}
	context := result + "\n\nOriginal request: " + q.Question + "\n" + q.Summary
	if unknown == 0 && state != "uncertain" {
		context += "\nFix the reported condition before retrying. Recorded successes will not be repeated."
	} else {
		context += "\nAn action may already have taken effect. Inspect the linked tasks/sessions before giving fresh instructions. There is no automatic retry."
	}
	_, err = ex.s.Inbox.PublishNotificationAttention(ctx, q.ID, inbox.Create{Abstract: "An Inbox action needs attention", Summary: "An Inbox action needs attention: " + state, Context: context, Question: "The action stopped. How would you like to handle it?", Options: options})
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
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,task_id,session_id,started_at,finished_at) SELECT ?,step_key,operation_id,kind,is_mutation,arguments,state,result,task_id,session_id,started_at,finished_at FROM notification_action_steps WHERE run_id=? AND state='succeeded'`, ex.q.ID, parent); err != nil {
		return err
	}
	return tx.Commit()
}

// Deliver outbox notices both immediately and on worker/restart recovery. The
// database, rather than an in-memory error path, decides whether delivery is due.
func (s *Service) deliverPendingActionAttention(ctx context.Context) (bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM notification_runs WHERE attention_pending=1 AND attention_question_id='' ORDER BY updated_at,id LIMIT 1`).Scan(&id)
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

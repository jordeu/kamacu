package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kamacu/internal/inboxaction"
)

const maxActionRecoveries = 5

func recoveryDelay(attempt int) time.Duration {
	delays := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		attempt = len(delays)
	}
	return delays[attempt-1]
}

func (ex *actionExecutor) deferRecovery(cause *recordingError) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exhausted := !transientStorageError(cause.err)
	err := ex.s.journalWrite(ctx, "schedule_recovery", func(c context.Context, tx *sql.Tx) error {
		var attempts int

		provenKey := ""
		if cause.phase == "intent" || cause.phase == "no_effect" {
			provenKey = cause.key
		}
		var unknown int
		if e := tx.QueryRowContext(c, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND is_mutation=1 AND state IN ('running','uncertain') AND step_key!=?`, ex.q.ID, provenKey).Scan(&unknown); e != nil {
			return e
		}
		if unknown > 0 {
			return errActionUncertain
		}

		var currentState, currentClass string
		if e := tx.QueryRowContext(c, `SELECT recovery_attempts,state,recovery_class FROM notification_runs WHERE id=?`, ex.q.ID).Scan(&attempts, &currentState, &currentClass); e != nil {
			return e
		}
		// A prior commit may have succeeded even if its acknowledgement timed out.
		// Do not count the same scheduling operation twice.
		if currentState == "queued" && currentClass == cause.phase && attempts > 0 {
			return nil
		}
		exhausted = exhausted || attempts >= maxActionRecoveries
		// When intent persistence failed, work was never invoked in this process.
		// A lost read result is likewise safe to read again, even after restart.
		query := `UPDATE notification_action_steps SET state='blocked',failure_class=?,result=?,finished_at=? WHERE run_id=? AND state='running'`
		if cause.phase == "intent" || cause.phase == "no_effect" {
			query += ` AND step_key=?`
		} else {
			query += ` AND is_mutation=0`
		}
		args := []any{cause.phase, cause.Error(), time.Now().UnixMilli(), ex.q.ID}
		if cause.phase == "intent" || cause.phase == "no_effect" {
			args = append(args, cause.key)
		}
		if cause.phase == "no_effect" {
			_, e := tx.ExecContext(c, query, "guard", cause.outcome.Error(), time.Now().UnixMilli(), ex.q.ID, cause.key)
			return e
		}
		if _, e := tx.ExecContext(c, query, args...); e != nil {
			return e
		}
		if exhausted {
			_, e := tx.ExecContext(c, `UPDATE notification_runs SET recovery_class='exhausted',next_attempt=0 WHERE id=?`, ex.q.ID)
			return e
		}
		next := time.Now().Add(recoveryDelay(attempts + 1)).UnixMilli()
		if _, e := tx.ExecContext(c, `UPDATE notification_runs SET state='queued',recovery_class=?,recovery_attempts=recovery_attempts+1,next_attempt=?,attention_pending=0,updated_at=? WHERE id=?`, cause.phase, next, time.Now().UnixMilli(), ex.q.ID); e != nil {
			return e
		}
		_, e := tx.ExecContext(c, `UPDATE inbox_questions SET delivery_status='queued',execution_result=?,revision=revision+1,updated_at=? WHERE id=?`, recoveryProgress(cause.phase), time.Now().UnixMilli(), ex.q.ID)
		return e
	})
	if errors.Is(err, errActionUncertain) {
		return ex.finish(errActionUncertain)
	}
	if err != nil {
		return err
	} // The idle worker/restart reconciles the durable intent.
	if cause.phase == "no_effect" {
		return ex.finish(cause.outcome)
	}
	if !exhausted {
		return nil
	}
	result := fmt.Sprintf("Automatic recovery stopped after repeated database recording failures: %v. Recorded task/session setup is preserved. No further action was started.", cause)
	if err = ex.s.finishRun(ctx, ex.q.ID, "failed", result, ex.history, false); err != nil {
		return err
	}
	delivery := ex.context
	if delivery == nil {
		delivery = context.Background()
	}
	return ex.s.deliverActionAttention(delivery, ex.q.ID)
}

// Called only with the action worker idle (or before it starts). Unknown
// mutations require attention; interrupted reads can be repeated automatically.
func (s *Service) recoverActionRuns(ctx context.Context, legacy bool) error {
	query := `SELECT r.id,r.action,r.recovery_attempts,r.attention_question_id,q.execution_result FROM notification_runs r JOIN inbox_questions q ON q.id=r.id WHERE r.state='running'`
	if legacy {
		query += ` OR (r.state='uncertain' AND q.execution_result LIKE '%cannot record step result:%' AND EXISTS(SELECT 1 FROM notification_action_steps x WHERE x.run_id=r.id AND x.is_mutation=0 AND x.state='running') AND NOT EXISTS(SELECT 1 FROM notification_action_steps x WHERE x.run_id=r.id AND x.is_mutation=1 AND x.state IN ('running','uncertain')) AND (r.attention_question_id='' OR EXISTS(SELECT 1 FROM inbox_questions a WHERE a.id=r.attention_question_id AND a.status='pending')) AND NOT EXISTS(SELECT 1 FROM notification_runs child WHERE json_extract(child.action,'$.guards.resume_run_id')=r.id))`
	}
	var due bool
	if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS("+query+")").Scan(&due); err != nil {
		return err
	}
	if !due {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	type interrupted struct {
		id, action, notice, result string
		attempts                   int
	}
	runs := []interrupted{}
	for rows.Next() {
		var r interrupted
		if err = rows.Scan(&r.id, &r.action, &r.attempts, &r.notice, &r.result); err != nil {
			rows.Close()
			return err
		}
		runs = append(runs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range runs {
		var seq inboxaction.Sequence
		valid := json.Unmarshal([]byte(r.action), &seq) == nil && seq.Validate() == nil
		if _, err = tx.ExecContext(ctx, `UPDATE notification_action_steps SET state=CASE WHEN is_mutation=1 THEN 'uncertain' ELSE 'blocked' END,failure_class=CASE WHEN is_mutation=1 THEN 'mutation_uncertain' ELSE 'interrupted_read' END,result='Step interrupted; mutation outcomes must be inspected, read-only checks may be repeated.',finished_at=? WHERE run_id=? AND state='running' AND kind!='diagnose_failure'`, time.Now().UnixMilli(), r.id); err != nil {
			return err
		}
		var unknown, blocked, mutations int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(is_mutation=1 AND state IN ('running','uncertain')),0),COALESCE(sum(state IN ('blocked','failed') AND failure_class NOT IN ('interrupted_read','read_result','intent') AND kind!='diagnose_failure'),0),COALESCE(sum(is_mutation=1 AND state='succeeded'),0) FROM notification_action_steps WHERE run_id=?`, r.id).Scan(&unknown, &blocked, &mutations); err != nil {
			return err
		}
		var blockedReasons string
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(group_concat(result,char(10)),'') FROM notification_action_steps WHERE run_id=? AND state IN ('failed','blocked') AND kind!='diagnose_failure'`, r.id).Scan(&blockedReasons); err != nil {
			return err
		}
		permanent := permanentActionGuard(fmt.Errorf("%w: %s\n%s", ErrActionBlocked, blockedReasons, r.result))

		completed := 0
		if valid {
			for _, a := range seq.Actions {
				var n int
				if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key=? AND operation_id=? AND kind='operation_complete' AND state='succeeded'`, r.id, a.ID+"/complete", a.ID).Scan(&n); err != nil {
					return err
				}
				completed += n
			}
		}

		state := "queued"
		class := "interrupted_read"
		result := recoveryProgress(class)
		if unknown > 0 {
			state = "uncertain"
			class = "mutation_uncertain"
			result = "Execution was interrupted during a side effect. Inspect its recorded steps before giving further instructions."
		} else if valid && completed == len(seq.Actions) {
			state = "succeeded"
			class = ""
			result = "All saved operations were already completed. Confirmed execution was recovered without repeating work."
		} else if !valid || blocked > 0 || permanent {
			state = "blocked"
			class = ""
			if mutations > 0 {
				state = "partial"
			}
			result = "Execution was interrupted. Check the recorded problem before continuing."
		} else if r.attempts >= maxActionRecoveries {
			state = "failed"
			class = "exhausted"
			result = "Automatic recovery exhausted its retry budget. No reply is required; inspect the operational failure details."
		}
		if r.action == "{}" {
			result = "Reply interpretation was interrupted before work started. Provide the reply again or handle it yourself."
		}
		pending := state != "queued" && state != "succeeded"
		attempts := r.attempts
		next := int64(0)
		if state == "queued" {
			attempts++
			if !legacy {
				next = time.Now().Add(recoveryDelay(attempts)).UnixMilli()
			}
			// Restart recovery may run immediately; subsequent recording failures back off.
		}
		if (state == "queued" || state == "succeeded") && r.notice != "" {
			if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET status='unanswered',reason='automatic_recovery',revision=revision+1,updated_at=? WHERE id=? AND status='pending'`, time.Now().UnixMilli(), r.notice); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE notification_runs SET state=?,recovery_class=?,recovery_attempts=?,next_attempt=?,attention_pending=?,attention_retryable=?,updated_at=? WHERE id=?`, state, class, attempts, next, pending, valid && unknown == 0 && !permanent && seq.Guards.ResumeRunID == "" && class != "exhausted", time.Now().UnixMilli(), r.id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET delivery_status=?,execution_result=?,revision=revision+1,updated_at=? WHERE id=?`, state, result, time.Now().UnixMilli(), r.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

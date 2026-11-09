package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"kamacu/internal/diagnostics"
	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"kamacu/internal/settings"
)

var ErrActionBlocked = errors.New("Inbox action requires attention")
var errActionUncertain = errors.New("action may have taken effect")

type ActionStep struct {
	Key         string          `json:"key"`
	OperationID string          `json:"operation_id"`
	Kind        string          `json:"kind"`
	Arguments   json.RawMessage `json:"arguments"`
	State       string          `json:"state"`
	Result      string          `json:"result"`
	TaskID      int64           `json:"task_id"`
	SessionID   string          `json:"session_id"`
	StartedAt   int64           `json:"started_at"`
	FinishedAt  *int64          `json:"finished_at"`
}
type ActionRun struct {
	State         string          `json:"state"`
	RecoveryClass string          `json:"recovery_class,omitempty"`
	Action        json.RawMessage `json:"action"`
	Steps         []ActionStep    `json:"steps"`
}

func (s *Service) ActionRun(ctx context.Context, id string) (ActionRun, error) {
	out := ActionRun{Steps: []ActionStep{}}
	var raw string
	if err := s.DB.QueryRowContext(ctx, `SELECT state,action,recovery_class FROM notification_runs WHERE id=?`, id).Scan(&out.State, &raw, &out.RecoveryClass); err != nil {
		return out, err
	}
	out.Action = json.RawMessage(raw)
	rows, err := s.DB.QueryContext(ctx, `SELECT step_key,operation_id,kind,arguments,state,result,task_id,session_id,started_at,finished_at FROM notification_action_steps WHERE run_id=? ORDER BY started_at,rowid`, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var step ActionStep
		var args string
		if err = rows.Scan(&step.Key, &step.OperationID, &step.Kind, &args, &step.State, &step.Result, &step.TaskID, &step.SessionID, &step.StartedAt, &step.FinishedAt); err != nil {
			return out, err
		}
		step.Arguments = json.RawMessage(args)
		out.Steps = append(out.Steps, step)
	}
	return out, rows.Err()
}

type actionExecutor struct {
	context   context.Context
	retrySafe bool
	s         *Service
	q         inbox.Question
	seq       inboxaction.Sequence
	sources   map[string]Source
	mutated   bool
	completed int
	history   []trace
}

func (s *Service) executeNext(ctx context.Context) error {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	if err := s.recoverActionRuns(ctx, false); err != nil {
		return err
	}
	if delivered, err := s.deliverPendingActionAttention(ctx); err != nil || delivered {
		return err
	}
	var id string
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM notification_runs WHERE state='queued' AND next_attempt<=? ORDER BY updated_at,id LIMIT 1`, time.Now().UnixMilli()).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	q, err := s.Inbox.Get(ctx, id)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE notification_runs SET state='running',updated_at=? WHERE id=? AND state='queued'`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ctx = diagnostics.WithGroup(ctx, diagnostics.Group{Kind: "reply", ID: id})
	ex := &actionExecutor{context: ctx, s: s, q: q, sources: map[string]Source{}, history: []trace{}}
	if err = s.Inbox.FinishNotification(ctx, id, "running", "Preparing the saved Inbox action."); err != nil {
		return err
	}
	var raw string
	if err = s.DB.QueryRowContext(ctx, `SELECT action FROM notification_runs WHERE id=?`, id).Scan(&raw); err != nil {
		return ex.finish(err)
	}
	if raw == "{}" {
		var question *inbox.Create
		ex.seq, question, err = s.planActionReply(ctx, q)
		if err != nil {
			return ex.finish(err)
		}
		if question != nil {
			if _, err = s.Inbox.PublishNotificationFollowup(ctx, q.ID, *question); err != nil {
				return ex.finish(err)
			}
			return s.finishRun(ctx, id, "succeeded", "A clarification question is waiting in your Inbox. No work was started.", ex.history)
		}
		if _, err = s.DB.ExecContext(ctx, `UPDATE notification_runs SET action=? WHERE id=?`, mustJSON(ex.seq), id); err != nil {
			return ex.finish(err)
		}
	} else if err = strictRuleJSON([]byte(raw), &ex.seq); err != nil {
		return ex.finish(err)
	}
	if err = ex.seq.Validate(); err != nil {
		return ex.finish(err)
	}
	ex.retrySafe = true
	if ex.seq.Guards.ResumeRunID != "" {
		if err = ex.copyCompletedSteps(ctx); err != nil {
			ex.retrySafe = false
			return ex.finish(err)
		}
	}

	rows, err := s.DB.QueryContext(ctx, `SELECT kind,result FROM notification_action_steps WHERE run_id=? AND state='succeeded' AND kind NOT IN ('operation_complete','diagnose_failure') ORDER BY started_at,rowid`, id)
	if err != nil {
		return ex.finish(err)
	}
	for rows.Next() {
		var t trace
		if err = rows.Scan(&t.Tool, &t.Result); err != nil {
			rows.Close()
			return ex.finish(err)
		}
		ex.history = append(ex.history, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ex.finish(err)
	}

	var mutations int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND is_mutation=1 AND state='succeeded'`, id).Scan(&mutations); err != nil {
		return ex.finish(err)
	}
	ex.mutated = mutations > 0
	if err = ex.seq.Validate(); err != nil {
		return ex.finish(err)
	}
	if len(ex.seq.Actions) == 1 && ex.seq.Actions[0].Kind == "ignore" {
		return s.finishRun(ctx, id, "succeeded", "You chose to leave these updates alone. No work was started.", ex.history)
	}
	if len(ex.seq.Actions) == 1 && ex.seq.Actions[0].Kind == "open_task" {
		if _, err := ex.operation(ctx, ex.seq.Actions[0]); err != nil {
			return ex.finish(err)
		}
		ex.completed = 1
		return ex.finish(nil)
	}
	knownComplete := 0
	for _, a := range ex.seq.Actions {
		var n int
		if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key=? AND state='succeeded' AND operation_id=? AND kind='operation_complete'`, id, a.ID+"/complete", a.ID).Scan(&n); err != nil {
			return ex.finish(err)
		}
		knownComplete += n
	}
	if knownComplete == len(ex.seq.Actions) {
		ex.completed = knownComplete
		return ex.finish(nil)
	}
	if err = ex.preflight(ctx); err != nil {
		if permanentActionGuard(err) {
			ex.retrySafe = false
		}
		return ex.finish(err)
	}
	refs := map[string]int64{}
	for _, original := range ex.seq.Actions {
		a := original
		var completedResult string
		err := s.DB.QueryRowContext(ctx, `SELECT result FROM notification_action_steps WHERE run_id=? AND step_key=? AND state='succeeded' AND operation_id=? AND kind='operation_complete'`, id, a.ID+"/complete", a.ID).Scan(&completedResult)
		if err == nil {
			task, _ := resultIdentity(completedResult)
			if task > 0 {
				refs[a.ID] = task
			}
			ex.completed++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ex.finish(err)
		}
		if a.TaskRef != "" {
			a.TaskID = refs[a.TaskRef]
			if a.TaskID <= 0 {
				return ex.finish(fmt.Errorf("missing result for task reference %s", a.TaskRef))
			}
		}
		task, err := ex.operation(ctx, a)
		if err != nil {
			return ex.finish(err)
		}
		if _, err = ex.step(ctx, a, a.ID+"/complete", "operation_complete", map[string]any{}, false, func() (string, error) { return mustJSON(map[string]any{"id": task}), nil }); err != nil {
			return ex.finish(err)
		}
		if task > 0 {
			refs[a.ID] = task
		}
		ex.completed++
	}
	return ex.finish(nil)
}
func (ex *actionExecutor) finish(cause error) error {
	var storage *recordingError
	if !errors.Is(cause, errActionUncertain) {
		if errors.As(cause, &storage) {
			return ex.deferRecovery(storage)
		}
		if cause != nil && transientStorageError(cause) && !errors.Is(cause, ErrActionBlocked) {
			return ex.deferRecovery(&recordingError{phase: "storage_read", err: cause})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state := "succeeded"
	result := fmt.Sprintf("Completed %d saved Inbox operation(s). No orchestration model was used for execution.", ex.completed)
	if cause != nil {
		state = "failed"
		if errors.Is(cause, errActionUncertain) {
			state = "uncertain"
		} else if ex.mutated {
			state = "partial"
		} else if errors.Is(cause, ErrActionBlocked) {
			state = "blocked"
		}
		result = fmt.Sprintf("Inbox action %s: %v", state, cause)
		if ex.mutated || state == "uncertain" {
			result += " Completed steps and task links are preserved. Inspect them before taking further action; no action will be automatically resent."
		}
	}
	if err := ex.s.finishRun(ctx, ex.q.ID, state, result, ex.history, ex.retrySafe); err != nil {
		return err
	}
	if cause != nil {
		deliveryCtx := ex.context
		if deliveryCtx == nil {
			deliveryCtx = context.Background()
		}
		return ex.s.deliverActionAttention(deliveryCtx, ex.q.ID)
	}
	return nil
}
func (ex *actionExecutor) guards(ctx context.Context) error {
	for id, expected := range ex.seq.Guards.Projects {
		var actual string
		if err := ex.s.DB.QueryRowContext(ctx, `SELECT COALESCE(github_repo,'') FROM projects WHERE id=?`, id).Scan(&actual); err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("%w: project repository changed", ErrActionBlocked)
		}
	}
	if ex.seq.Guards.RuleSetID != "" {
		set, err := ex.s.activeRuleSet(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: notification rules changed; refresh the message", ErrActionBlocked)
			}
			return err
		}
		if set.ID != ex.seq.Guards.RuleSetID {
			return fmt.Errorf("%w: notification rules changed; refresh the message", ErrActionBlocked)
		}
	}
	if ex.seq.Guards.ReviewSeed != "" {
		seed, err := settings.Get(ex.s.DB, settings.KeyPRReviewSeed)
		if err != nil {
			return err
		}
		if seed != ex.seq.Guards.ReviewSeed {
			return fmt.Errorf("%w: configured review prompt changed", ErrActionBlocked)
		}
	}
	for id, source := range ex.sources {
		if err := ex.s.sourceEnabled(ctx, id); err != nil {
			return fmt.Errorf("%w: %v", ErrActionBlocked, err)
		}
		if err := ex.s.checkIdentity(ctx, source); err != nil {
			return fmt.Errorf("%w: %v", ErrActionBlocked, err)
		}
	}
	return nil
}
func (ex *actionExecutor) preflight(ctx context.Context) error {
	for _, id := range ex.seq.Guards.SourceIDs {
		source, err := scanSource(ex.s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, id))
		if err != nil {
			return err
		}
		ex.sources[id] = source
	}
	if err := ex.guards(ctx); err != nil {
		return err
	}
	if ex.seq.Guards.ReviewSeed != "" {
		seed, err := settings.Get(ex.s.DB, settings.KeyPRReviewSeed)
		if err != nil {
			return err
		}
		if seed != ex.seq.Guards.ReviewSeed {
			return fmt.Errorf("%w: configured review prompt changed; refresh this message", ErrActionBlocked)
		}
	}
	for id := range ex.seq.Guards.TaskStatus {
		needed, err := ex.taskGuardRequired(ctx, id)
		if err != nil {
			return err
		}
		if !needed {
			continue
		}
		var status string
		if err := ex.s.DB.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id=?`, id).Scan(&status); err != nil {
			return err
		}
		expected, err := ex.expectedStatus(ctx, id, status)
		if err != nil {
			return err
		}
		if status != expected {
			completedMove := false
			for _, a := range ex.seq.Actions {
				if a.TaskID == id && a.Kind == "complete_task" && a.Status == status {
					var n int
					if err := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key=? AND state='succeeded'`, ex.q.ID, a.ID+"/move").Scan(&n); err != nil {
						return err
					}
					completedMove = n == 1
				}
			}
			if completedMove {
				continue
			}
			return fmt.Errorf("%w: task %d moved from %s to %s", ErrActionBlocked, id, expected, status)
		}
	}
	for id, guard := range ex.seq.Guards.Sessions {
		var stopped int
		if err := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND kind='close_session' AND state='succeeded' AND json_extract(arguments,'$.session_id')=?`, ex.q.ID, id).Scan(&stopped); err != nil {
			return err
		}
		if stopped > 0 {
			continue
		}
		expectedInput, err := ex.expectedInput(ctx, id, guard.Input)
		if err != nil {
			return err
		}

		if ex.s.Actions == nil {
			return errors.New("session actions unavailable")
		}
		sess, err := ex.s.Actions.Session(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrActionBlocked, err)
		}
		if sess.TaskID != guard.TaskID || sess.Input != expectedInput {
			return fmt.Errorf("%w: the specified session has received new input", ErrActionBlocked)
		}
	}
	return nil
}

// step records intent before a side effect. A recorded success is reusable, but
// an in-flight/uncertain mutation is never replayed, including after a restart.
func (ex *actionExecutor) step(ctx context.Context, a inboxaction.Action, key, kind string, args any, mutation bool, work func() (string, error)) (string, error) {
	var state, result, recordedOperation, recordedKind, failureClass string
	var err error
	baseKey := key
	for recheck := 0; ; recheck++ {
		err = ex.s.DB.QueryRowContext(ctx, `SELECT state,result,operation_id,kind,failure_class FROM notification_action_steps WHERE run_id=? AND step_key=?`, ex.q.ID, key).Scan(&state, &result, &recordedOperation, &recordedKind, &failureClass)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return "", err
		}
		if recordedOperation != a.ID || recordedKind != kind {
			return "", fmt.Errorf("%w: recorded step belongs to another operation", errActionUncertain)
		}
		if state == "succeeded" {
			// Verification and readiness are observations, not reusable side
			// effects. Refresh them when resuming unfinished work.
			if !mutation && (kind == "verify_review" || kind == "wait_ready" || kind == "list_sessions") {
				key = fmt.Sprintf("%s/recheck_%d", baseKey, recheck+1)
				continue
			}
			if mutation {
				ex.mutated = true
			}
			return result, nil
		}
		// Intent persistence is known not to have invoked work. Reuse its logical
		// key so completion/delivery lookups still recognize a later success.
		if state == "blocked" && failureClass == "intent" || !mutation && kind == "operation_complete" && retryableRead(state, failureClass) {
			break
		}
		if !mutation && retryableRead(state, failureClass) {
			key = fmt.Sprintf("%s/recheck_%d", baseKey, recheck+1)
			continue
		}
		if !mutation {
			return "", fmt.Errorf("%w: %s", ErrActionBlocked, result)
		}

		return "", fmt.Errorf("%w: step %s already has state %s", errActionUncertain, key, state)
	}

	now := time.Now().UnixMilli()
	if err = ex.s.journalWrite(ctx, "step_intent", func(c context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(c, `INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,?,?,?,?,?,'running',?) ON CONFLICT(run_id,step_key) DO UPDATE SET state='running',result='',failure_class='',started_at=excluded.started_at,finished_at=NULL WHERE (notification_action_steps.state='blocked' AND notification_action_steps.failure_class='intent') OR (notification_action_steps.kind='operation_complete' AND notification_action_steps.is_mutation=0 AND (notification_action_steps.state='running' OR (notification_action_steps.state IN ('blocked','failed') AND notification_action_steps.failure_class IN ('interrupted_read','read_result','intent'))))`, ex.q.ID, key, a.ID, kind, mutation, mustJSON(args), now)
		if e != nil {
			return e
		}
		var op, actualKind, argsJSON, actualState string
		var mut bool
		if e = tx.QueryRowContext(c, `SELECT operation_id,kind,arguments,state,is_mutation FROM notification_action_steps WHERE run_id=? AND step_key=?`, ex.q.ID, key).Scan(&op, &actualKind, &argsJSON, &actualState, &mut); e != nil {
			return e
		}
		if op != a.ID || actualKind != kind || argsJSON != mustJSON(args) || actualState != "running" || mut != mutation {
			return errActionUncertain
		}

		return e
	}); err != nil {
		return "", &recordingError{key: key, phase: "intent", err: err}
	}
	progress := a.ID
	if a.Kind == "start_reviews" && a.TaskName != "" {
		progress = a.TaskName
	}
	if mutation || kind == "wait_ready" {
		ex.progress(ctx, fmt.Sprintf("%s: %s", progress, kind))
	}

	// Recording intent and updating progress may wait for the database. Check
	// authorization and the target afterwards, immediately before the effect.
	result = ""
	var workErr error
	if mutation {
		workErr = ex.guards(ctx)
		if workErr == nil {
			workErr = ex.validateTarget(ctx, a)
		}
		if workErr != nil && transientStorageError(workErr) {
			return "", &recordingError{key: key, phase: "intent", err: workErr}
		}
		if workErr != nil && !errors.Is(workErr, ErrActionBlocked) {
			workErr = fmt.Errorf("%w: %v", ErrActionBlocked, workErr)
		}
	}
	if workErr == nil {
		result, workErr = work()
	}
	if !mutation && ctx.Err() != nil {
		workErr = ctx.Err()
	}
	if permanentActionGuard(workErr) {
		ex.retrySafe = false
	}
	state = "succeeded"
	if workErr != nil {
		state = "failed"
		if errors.Is(workErr, ErrActionBlocked) {
			state = "blocked"
		} else if mutation {
			state = "uncertain"
		}
	}

	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if mutation && (workErr == nil || state == "uncertain") {
		ex.mutated = true
	}
	recorded := result
	if workErr != nil {
		recorded = workErr.Error()
	}
	task, session := resultIdentity(result)
	resultClass := ""
	if !mutation && workErr != nil && transientStorageError(workErr) {
		resultClass = "read_result"
	}
	history := append(append([]trace{}, ex.history...), trace{Tool: kind, Result: result})
	if err = ex.s.journalWrite(recordCtx, "step_result", func(c context.Context, tx *sql.Tx) error {
		if res, e := tx.ExecContext(c, `UPDATE notification_action_steps SET state=?,result=?,task_id=?,session_id=?,finished_at=?,failure_class=? WHERE run_id=? AND step_key=?`, state, recorded, task, session, time.Now().UnixMilli(), resultClass, ex.q.ID, key); e != nil {
			return e
		} else if n, e := res.RowsAffected(); e != nil {
			return e
		} else if n != 1 {
			return sql.ErrNoRows
		}
		_, e := tx.ExecContext(c, `UPDATE notification_runs SET updated_at=? WHERE id=?`, time.Now().UnixMilli(), ex.q.ID)
		return e
	}); err != nil {
		if !mutation {
			return "", &recordingError{key: key, phase: "read_result", err: err}
		}
		if errors.Is(workErr, ErrActionBlocked) {
			return "", &recordingError{key: key, phase: "no_effect", err: err, outcome: workErr}
		}
		// The journal is authoritative if a commit succeeded despite a timeout.
		var actual, actualResult string
		if e := ex.s.DB.QueryRowContext(recordCtx, `SELECT state,result FROM notification_action_steps WHERE run_id=? AND step_key=?`, ex.q.ID, key).Scan(&actual, &actualResult); e == nil && actual == state && actualResult == recorded {
			slog.Debug("notification result confirmed after recording error", "run", ex.q.ID, "step", key)
		} else {
			return "", fmt.Errorf("%w: cannot confirm mutation result for %s: %v", errActionUncertain, kind, err)
		}
	}
	if workErr != nil {
		if state == "uncertain" {
			return "", fmt.Errorf("%w: %s: %v", errActionUncertain, kind, workErr)
		}
		return "", workErr
	}
	ex.history = history

	return result, nil
}
func resultIdentity(raw string) (int64, string) {
	var v struct {
		ID     json.RawMessage `json:"id"`
		TaskID int64           `json:"taskId"`
		Task   struct {
			ID int64 `json:"id"`
		} `json:"task"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil {
		return 0, ""
	}
	if v.Task.ID > 0 {
		return v.Task.ID, ""
	}
	var n int64
	if json.Unmarshal(v.ID, &n) == nil && n > 0 {
		return n, ""
	}
	var id string
	if json.Unmarshal(v.ID, &id) == nil {
		return v.TaskID, id
	}
	return v.TaskID, ""
}
func (ex *actionExecutor) tool(ctx context.Context, a inboxaction.Action, key, name string, args any, mutation bool) (string, error) {
	if ex.s.Tools == nil {
		return "", errors.New("Kamacu tools unavailable")
	}
	return ex.step(ctx, a, key, name, args, mutation, func() (string, error) { return ex.s.Tools.Call(ctx, name, json.RawMessage(mustJSON(args))) })
}
func (ex *actionExecutor) task(ctx context.Context, id, project int64) (source, status string, recurring bool, err error) {
	var owner int64
	err = ex.s.DB.QueryRowContext(ctx, `SELECT project_id,source,status,recurrent FROM tasks WHERE id=?`, id).Scan(&owner, &source, &status, &recurring)
	if err == nil && owner != project {
		err = fmt.Errorf("%w: task belongs to another project", ErrActionBlocked)
	}
	return
}
func (ex *actionExecutor) link(ctx context.Context, task int64, role string) error {
	var l inbox.TaskLink
	l.TaskID = task
	l.Role = role
	l.Source = "direct_action"
	l.Evidence = ex.seq.Summary()
	if err := ex.s.DB.QueryRowContext(ctx, `SELECT project_id,title,status FROM tasks WHERE id=?`, task).Scan(&l.ProjectID, &l.Title, &l.Status); err != nil {
		return err
	}
	return ex.s.Inbox.LinkNotificationTasks(ctx, ex.q.ID, []inbox.TaskLink{l})
}

func (ex *actionExecutor) operation(ctx context.Context, a inboxaction.Action) (int64, error) {
	switch a.Kind {
	case "start_reviews":
		failures := []error{}
		for index, n := range a.PRNumbers {
			reviewAction := a
			reviewAction.TaskName = fmt.Sprintf("Review %d of %d (PR %d)", index+1, len(a.PRNumbers), n)
			if err := ex.review(ctx, reviewAction, n); err != nil {
				failures = append(failures, fmt.Errorf("PR %d: %w", n, err))
				if ctx.Err() != nil {
					break
				}
			}
		}
		return 0, errors.Join(failures...)
	case "create_investigation":
		raw, err := ex.tool(ctx, a, a.ID+"/create", "create_task", map[string]any{"project_id": a.ProjectID, "title": a.TaskName, "description": a.Description}, true)
		if err != nil {
			return 0, err
		}
		id, _ := resultIdentity(raw)
		if id <= 0 {
			return 0, fmt.Errorf("%w: created task ID was not returned", errActionUncertain)
		}
		a.TaskID = id
		if _, _, _, err = ex.task(ctx, id, a.ProjectID); err != nil {
			return id, err
		}
		if err = ex.link(ctx, id, "investigation"); err != nil {
			return id, err
		}
		return id, ex.handoff(ctx, a, a.ID, a.Instructions, false)
	case "continue_task", "run_recurring_task":
		source, _, recurring, err := ex.task(ctx, a.TaskID, a.ProjectID)
		if err != nil {
			return a.TaskID, err
		}
		if a.Kind == "continue_task" && source == "github_pr" {
			return a.TaskID, fmt.Errorf("%w: use the PR review workflow", ErrActionBlocked)
		}
		if a.Kind == "run_recurring_task" && !recurring {
			return a.TaskID, fmt.Errorf("%w: task is no longer recurring", ErrActionBlocked)
		}
		role := "implementation"
		if recurring {
			role = "recurring"
		}
		if err = ex.link(ctx, a.TaskID, role); err != nil {
			return a.TaskID, err
		}
		return a.TaskID, ex.handoff(ctx, a, a.ID, a.Instructions, false)
	case "send_to_session":
		if _, _, _, err := ex.task(ctx, a.TaskID, a.ProjectID); err != nil {
			return 0, err
		}
		return a.TaskID, ex.submit(ctx, a, a.ID, a.SessionID, a.Instructions)
	case "close_session", "complete_task":
		if ex.s.Actions == nil {
			return 0, errors.New("session/task lifecycle actions unavailable")
		}
		source, status, _, err := ex.task(ctx, a.TaskID, a.ProjectID)
		if err == nil {
			status, err = ex.expectedStatus(ctx, a.TaskID, status)
		}
		if err != nil {
			return 0, err
		}
		if a.Kind == "complete_task" && source != "manual" {
			return 0, fmt.Errorf("%w: only manual tasks can be moved on the board", ErrActionBlocked)
		}
		if a.SessionID != "" {
			_, err = ex.step(ctx, a, a.ID+"/stop", "close_session", map[string]any{"session_id": a.SessionID, "task_id": a.TaskID}, true, func() (string, error) {
				sess, e := ex.s.Actions.Session(ctx, a.SessionID)
				if e != nil {
					return "", fmt.Errorf("%w: %v", ErrActionBlocked, e)
				}
				guard, ok := ex.seq.Guards.Sessions[a.SessionID]
				expectedInput, e := ex.expectedInput(ctx, a.SessionID, guard.Input)
				if e != nil {
					return "", e
				}
				if !ok || sess.TaskID != a.TaskID || sess.Input != expectedInput {
					return "", fmt.Errorf("%w: session changed since this option was offered", ErrActionBlocked)
				}
				e = ex.s.Actions.Stop(ctx, sess)
				return mustJSON(map[string]any{"taskId": a.TaskID, "id": a.SessionID, "stopped": e == nil}), e
			})
			if err != nil {
				return a.TaskID, err
			}
		} else if ex.s.TaskSession != nil && ex.s.TaskSession(a.TaskID) != "" {
			return a.TaskID, fmt.Errorf("%w: a new task session is running; refresh the completion action", ErrActionBlocked)
		}
		if a.Kind == "complete_task" {
			_, err = ex.step(ctx, a, a.ID+"/move", "move_task", map[string]any{"task_id": a.TaskID, "status": a.Status}, true, func() (string, error) {
				e := ex.s.Actions.Move(ctx, a.TaskID, status, a.Status)
				return mustJSON(map[string]any{"id": a.TaskID, "status": a.Status}), e
			})
		}
		return a.TaskID, err
	case "update_task":
		if _, _, _, err := ex.task(ctx, a.TaskID, a.ProjectID); err != nil {
			return 0, err
		}
		args := map[string]any{"task_id": a.TaskID}
		if a.Title != "" {
			args["title"] = a.Title
		}
		if a.Description != "" {
			args["description"] = a.Description
		}
		_, err := ex.tool(ctx, a, a.ID+"/update", "update_task", args, true)
		return a.TaskID, err
	case "associate_pr":
		if _, _, _, err := ex.task(ctx, a.TaskID, a.ProjectID); err != nil {
			return 0, err
		}
		var repo string
		if err := ex.s.DB.QueryRowContext(ctx, `SELECT github_repo FROM projects WHERE id=?`, a.ProjectID).Scan(&repo); err != nil {
			return 0, err
		}
		for _, n := range a.PRNumbers {
			if _, err := ex.tool(ctx, a, fmt.Sprintf("%s/link/%d", a.ID, n), "set_task_pr_link", map[string]any{"task_id": a.TaskID, "repository": repo, "number": n, "role": a.LinkRole, "evidence": "Approved Inbox action " + ex.q.ID + "; operation " + a.ID}, true); err != nil {
				return a.TaskID, err
			}
		}
		return a.TaskID, nil
	case "open_task":
		source, _, recurring, err := ex.task(ctx, a.TaskID, a.ProjectID)
		if err != nil {
			return 0, err
		}
		role := "implementation"
		if recurring {
			role = "recurring"
		}
		if source == "github_pr" {
			role = "review"
		}
		return a.TaskID, ex.link(ctx, a.TaskID, role)
	default:
		return 0, fmt.Errorf("unsupported operation %s", a.Kind)
	}
}

func (ex *actionExecutor) handoff(ctx context.Context, a inboxaction.Action, key, message string, review bool) error {
	var submitted int
	if err := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key=? AND state='succeeded' AND operation_id=? AND kind='send_session_message'`, ex.q.ID, key+"/submit", a.ID).Scan(&submitted); err != nil {
		return err
	}
	if submitted > 0 {
		return nil
	}
	raw, err := ex.tool(ctx, a, key+"/sessions", "list_sessions", map[string]any{"task_id": a.TaskID}, false)
	if err != nil {
		return err
	}
	var list []struct {
		ID     string `json:"id"`
		TaskID int64  `json:"taskId"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if err = json.Unmarshal([]byte(raw), &list); err != nil {
		return errors.New("cannot decode current task sessions")
	}
	sid := ""
	for _, sess := range list {
		if sess.TaskID == a.TaskID && sess.Kind == "agent" && sess.Status == "running" {
			if sid != "" {
				return fmt.Errorf("%w: multiple live agents", ErrActionBlocked)
			}
			sid = sess.ID
		}
	}
	if sid == "" {
		var resume bool
		if err = ex.s.DB.QueryRowContext(ctx, `SELECT CASE a.engine WHEN 'codex' THEN COALESCE(t.codex_session_id,'')!='' WHEN 'opencode' THEN COALESCE(t.opencode_session_id,'')!='' WHEN 'custom' THEN 0 ELSE COALESCE(t.claude_session_id,'')!='' END FROM tasks t JOIN projects p ON p.id=t.project_id JOIN agents a ON a.id=p.agent_id WHERE t.id=?`, a.TaskID).Scan(&resume); err != nil {
			return err
		}
		raw, err = ex.tool(ctx, a, key+"/start", "start_task_agent", map[string]any{"task_id": a.TaskID, "resume": resume}, true)
		if err != nil {
			return err
		}
		task, id := resultIdentity(raw)
		if task != a.TaskID || id == "" {
			return fmt.Errorf("%w: started session identity was not confirmed", errActionUncertain)
		}
		sid = id
	}
	if ex.s.Actions == nil {
		return errors.New("session readiness/submission service unavailable")
	}
	if review {
		var state string
		err = ex.s.DB.QueryRowContext(ctx, `SELECT state FROM notification_review_prompts WHERE task_id=?`, a.TaskID).Scan(&state)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state == "sending" {
			return fmt.Errorf("%w: initial review prompt delivery is uncertain; inspect task %d", ErrActionBlocked, a.TaskID)
		}
		if state != "sent" {
			_, err = ex.s.DB.ExecContext(ctx, `INSERT INTO notification_review_prompts(task_id,prompt,state) VALUES(?,?,'pending') ON CONFLICT(task_id) DO UPDATE SET prompt=excluded.prompt,state='pending' WHERE notification_review_prompts.state!='sending'`, a.TaskID, message)
			if err != nil {
				return err
			}
		} else {
			message = a.Instructions
			if strings.TrimSpace(message) == "" {
				message = "Continue the review using the configured review instructions. Inspect the latest changes and report findings locally for " + strings.Join(ex.seq.Guards.SourceURLs, "\n")
			}
		}
	}
	return ex.submit(ctx, a, key, sid, message)
}
func (ex *actionExecutor) submit(ctx context.Context, a inboxaction.Action, key, sid, message string) error {
	if ex.s.Actions == nil {
		return errors.New("session actions unavailable")
	}
	var registered, state, taskSource string
	if err := ex.s.DB.QueryRowContext(ctx, `SELECT source FROM tasks WHERE id=?`, a.TaskID).Scan(&taskSource); err != nil {
		return err
	}
	err := ex.s.DB.QueryRowContext(ctx, `SELECT prompt,state FROM notification_review_prompts WHERE task_id=?`, a.TaskID).Scan(&registered, &state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if state == "sending" {
		return fmt.Errorf("%w: initial review prompt delivery is uncertain", ErrActionBlocked)
	}
	if taskSource == "github_pr" && state == "" {
		return fmt.Errorf("%w: initial review instructions must use start_reviews", ErrActionBlocked)
	}
	if state == "pending" {
		message = registered
	}
	var ready ActionSession
	readyRaw, err := ex.step(ctx, a, key+"/ready", "wait_ready", map[string]any{"session_id": sid, "task_id": a.TaskID}, false, func() (string, error) {
		var e error
		ready, e = ex.s.Actions.Ready(ctx, sid, a.BusyPolicy)
		if e == nil && ready.TaskID != a.TaskID {
			e = fmt.Errorf("%w: session belongs to another task", ErrActionBlocked)
		}
		return mustJSON(ready), e
	})
	if err != nil {
		return err
	}
	_, err = ex.step(ctx, a, key+"/submit", "send_session_message", map[string]any{"session_id": sid, "task_id": a.TaskID, "message": message}, true, func() (string, error) {
		if ready.ID == "" {
			if e := json.Unmarshal([]byte(readyRaw), &ready); e != nil {
				return "", e
			}
		}
		if guard, ok := ex.seq.Guards.Sessions[sid]; ok {
			expectedInput, e := ex.expectedInput(ctx, sid, guard.Input)
			if e != nil {
				return "", e
			}
			if guard.TaskID != ready.TaskID || expectedInput != ready.Input {
				return "", fmt.Errorf("%w: session received new input", ErrActionBlocked)
			}
		}
		if _, saved := ex.seq.Guards.Sessions[sid]; !saved {
			var started int
			if e := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND kind='start_task_agent' AND state='succeeded' AND task_id=? AND session_id=?`, ex.q.ID, a.TaskID, sid).Scan(&started); e != nil {
				return "", e
			}
			if started > 0 {
				expected, e := ex.expectedInput(ctx, sid, 0)
				if e != nil {
					return "", e
				}
				if ready.Input != expected {
					return "", fmt.Errorf("%w: the started session received new input; inspect task %d before requesting more work", ErrActionBlocked, a.TaskID)
				}
			}
		}

		var pending string
		e := ex.s.DB.QueryRowContext(ctx, `SELECT state FROM notification_review_prompts WHERE task_id=?`, a.TaskID).Scan(&pending)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return "", e
		}
		if pending == "sending" {
			return "", fmt.Errorf("%w: initial review prompt delivery is uncertain", ErrActionBlocked)
		}
		if pending == "pending" {
			if _, e = ex.s.DB.ExecContext(ctx, `UPDATE notification_review_prompts SET state='sending' WHERE task_id=? AND state='pending'`, a.TaskID); e != nil {
				return "", e
			}
		}
		if e = ex.s.Actions.Submit(ctx, ready, message); e != nil {
			if errors.Is(e, ErrActionBlocked) && pending == "pending" {
				_, _ = ex.s.DB.ExecContext(context.WithoutCancel(ctx), `UPDATE notification_review_prompts SET state='pending' WHERE task_id=? AND state='sending'`, a.TaskID)
			}
			return "", e
		}
		if pending == "pending" {
			if _, e = ex.s.DB.ExecContext(ctx, `UPDATE notification_review_prompts SET state='sent' WHERE task_id=? AND state='sending'`, a.TaskID); e != nil {
				return "", e
			}
		}
		return mustJSON(map[string]any{"taskId": a.TaskID, "id": sid, "submitted": true, "input_after": ready.Input + 1}), nil
	})
	return err
}
func (ex *actionExecutor) review(ctx context.Context, a inboxaction.Action, n int64) error {
	key := fmt.Sprintf("%s/pr/%d", a.ID, n)
	var submitted int
	if err := ex.s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_action_steps WHERE run_id=? AND step_key=? AND state='succeeded' AND operation_id=? AND kind='send_session_message'`, ex.q.ID, key+"/submit", a.ID).Scan(&submitted); err != nil {
		return err
	}
	if submitted > 0 {
		return nil
	}
	var repo string
	if err := ex.s.DB.QueryRowContext(ctx, `SELECT github_repo FROM projects WHERE id=?`, a.ProjectID).Scan(&repo); err != nil {
		return err
	}
	var title string
	verifiedRaw, err := ex.step(ctx, a, key+"/verify", "verify_review", map[string]any{"project_id": a.ProjectID, "pr_number": n}, false, func() (string, error) {
		if len(ex.sources) == 0 {
			return "", fmt.Errorf("%w: review has no bound source identity", ErrActionBlocked)
		}
		for _, source := range ex.sources {
			event := Event{Repository: repo, URL: fmt.Sprintf("https://github.com/%s/pull/%d", repo, n), SubjectURL: fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", repo, n), SubjectType: "PullRequest"}
			provider := ex.s.Providers[source.Provider]
			var fresh Event
			var e error
			if reader, ok := provider.(interface {
				ActionPR(context.Context, Source, Event) (Event, error)
			}); ok {
				fresh, e = reader.ActionPR(ctx, source, event)
			} else {
				fresh, e = ex.s.enrichSubject(ctx, source, event)
			}
			if e != nil {
				return "", e
			}
			if fresh.SubjectState != "open" || fresh.PRFacts == nil || !fresh.PRFacts.Available || fresh.PRFacts.Stale || fresh.PRFacts.Draft == nil || fresh.PRFacts != nil && ((fresh.PRFacts.Draft != nil && *fresh.PRFacts.Draft) || strings.EqualFold(fresh.PRFacts.State, "merged") || strings.EqualFold(fresh.PRFacts.State, "closed")) {
				return "", fmt.Errorf("%w: PR %s is no longer an open, non-draft review target", ErrActionBlocked, event.URL)
			}
			title = fresh.SubjectTitle
			if strings.TrimSpace(title) == "" {
				return "", fmt.Errorf("%w: live PR title unavailable", ErrActionBlocked)
			}
			return mustJSON(map[string]any{"title": title, "url": event.URL, "state": fresh.SubjectState}), nil
		}
		return "", errors.New("no source for PR verification")
	})
	if err != nil {
		return err
	}
	if title == "" {
		var verified struct {
			Title string `json:"title"`
		}
		if err = json.Unmarshal([]byte(verifiedRaw), &verified); err != nil {
			return err
		}
		title = verified.Title
	}
	raw, err := ex.tool(ctx, a, key+"/open", "open_review", map[string]any{"project_id": a.ProjectID, "pr_number": n}, true)
	if err != nil {
		return err
	}
	id, _ := resultIdentity(raw)
	if id <= 0 {
		return fmt.Errorf("%w: review task identity unavailable", errActionUncertain)
	}
	var project, number int64
	var source string
	if err = ex.s.DB.QueryRowContext(ctx, `SELECT project_id,pr_number,source FROM tasks WHERE id=?`, id).Scan(&project, &number, &source); err != nil {
		return err
	}
	if project != a.ProjectID || number != n || source != "github_pr" {
		return fmt.Errorf("%w: wrong PR review task", ErrActionBlocked)
	}
	a.TaskID = id
	if err = ex.link(ctx, id, "review"); err != nil {
		return err
	}
	seed, err := settings.Get(ex.s.DB, settings.KeyPRReviewSeed)
	if err != nil {
		return err
	}
	if seed != ex.seq.Guards.ReviewSeed || strings.TrimSpace(seed) == "" {
		return fmt.Errorf("%w: configured review prompt changed", ErrActionBlocked)
	}
	prompt := strings.NewReplacer("<n>", fmt.Sprint(n), "<title>", title).Replace(seed)
	if strings.TrimSpace(a.Instructions) == "" {
		a.Instructions = "Continue the PR review using the configured review instructions. Inspect the latest changes and report findings locally."
	}
	a.Instructions += "\n\nReview target: " + fmt.Sprintf("https://github.com/%s/pull/%d", repo, n)
	return ex.handoff(ctx, a, key, prompt, true)
}

// Recheck roles/status immediately before each side effect, including after a
// readiness wait. Recorded moves by this same sequence advance its own guard.
func (ex *actionExecutor) validateTarget(ctx context.Context, a inboxaction.Action) error {
	if a.TaskID == 0 {
		return nil
	}
	source, status, recurring, err := ex.task(ctx, a.TaskID, a.ProjectID)
	if err != nil {
		return err
	}
	expected, err := ex.expectedStatus(ctx, a.TaskID, status)
	if err != nil {
		return err
	}
	if status != expected {
		return fmt.Errorf("%w: task status changed before %s", ErrActionBlocked, a.Kind)
	}

	if a.Kind == "run_recurring_task" && !recurring {
		return fmt.Errorf("%w: task is no longer recurring", ErrActionBlocked)
	}
	if (a.Kind == "continue_task" || a.Kind == "create_investigation") && source == "github_pr" {
		return fmt.Errorf("%w: implementation action cannot target a PR review task", ErrActionBlocked)
	}
	if a.Kind == "complete_task" && source != "manual" {
		return fmt.Errorf("%w: only manual tasks can be completed on the board", ErrActionBlocked)
	}
	if a.Kind == "start_reviews" {
		var pr int64
		if err := ex.s.DB.QueryRowContext(ctx, `SELECT COALESCE(pr_number,0) FROM tasks WHERE id=?`, a.TaskID).Scan(&pr); err != nil {
			return err
		}
		if source != "github_pr" || !slices.Contains(a.PRNumbers, pr) {
			return fmt.Errorf("%w: review action target changed", ErrActionBlocked)
		}
	}
	return nil
}

func (ex *actionExecutor) expectedStatus(ctx context.Context, id int64, fallback string) (string, error) {
	expected, ok := ex.seq.Guards.TaskStatus[id]
	if !ok {
		expected = fallback
	}
	var moved string
	err := ex.s.DB.QueryRowContext(ctx, `SELECT json_extract(arguments,'$.status') FROM notification_action_steps WHERE run_id=? AND kind='move_task' AND state='succeeded' AND json_extract(arguments,'$.task_id')=? ORDER BY finished_at DESC,rowid DESC LIMIT 1`, ex.q.ID, id).Scan(&moved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil {
		expected = moved
	}
	// The normal start-agent API moves a manual todo task to in_progress. Accept
	// only that transition when this run recorded the start and still owns its
	// live session. Never adopt a move to done or an unrelated replacement agent.
	if expected == "todo" && fallback == "in_progress" && ex.s.TaskSession != nil {
		var raw, operation string
		e := ex.s.DB.QueryRowContext(ctx, `SELECT result,operation_id FROM notification_action_steps WHERE run_id=? AND kind='start_task_agent' AND state='succeeded' AND json_extract(arguments,'$.task_id')=? ORDER BY finished_at DESC,rowid DESC LIMIT 1`, ex.q.ID, id).Scan(&raw, &operation)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return "", e
		}
		task, sid := resultIdentity(raw)
		if e == nil && task == id && sid != "" && ex.s.TaskSession(id) == sid {
			for _, a := range ex.seq.Actions {
				if a.ID == operation && a.TaskID == id && (a.Kind == "continue_task" || a.Kind == "run_recurring_task") {
					expected = "in_progress"
					break
				}
			}
		}
	}

	return expected, nil
}

func (ex *actionExecutor) expectedInput(ctx context.Context, id string, fallback uint64) (uint64, error) {
	var input uint64
	err := ex.s.DB.QueryRowContext(ctx, `SELECT json_extract(result,'$.input_after') FROM notification_action_steps WHERE run_id=? AND kind='send_session_message' AND state='succeeded' AND json_extract(arguments,'$.session_id')=? AND json_extract(result,'$.input_after') IS NOT NULL ORDER BY finished_at DESC,rowid DESC LIMIT 1`, ex.q.ID, id).Scan(&input)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	return input, err
}

// Completed operations do not need their old task status to remain frozen.
// Keep the guard when another unfinished operation still references that task.
func (ex *actionExecutor) taskGuardRequired(ctx context.Context, id int64) (bool, error) {
	rows, err := ex.s.DB.QueryContext(ctx, `SELECT operation_id,result FROM notification_action_steps WHERE run_id=? AND kind='operation_complete' AND state='succeeded' AND step_key=operation_id||'/complete'`, ex.q.ID)
	if err != nil {
		return false, err
	}
	completed := map[string]string{}
	for rows.Next() {
		var op, result string
		if err = rows.Scan(&op, &result); err != nil {
			rows.Close()
			return false, err
		}
		completed[op] = result
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	refs := map[string]int64{}
	for _, a := range ex.seq.Actions {
		task := a.TaskID
		if a.TaskRef != "" {
			task = refs[a.TaskRef]
		}
		result, done := completed[a.ID]
		if done {
			if resolved, _ := resultIdentity(result); resolved > 0 {
				task = resolved
			}
		}
		refs[a.ID] = task
		if task == id && !done {
			return true, nil
		}
	}
	return false, nil
}

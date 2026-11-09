package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
)

type trace struct {
	Tool   string `json:"tool,omitempty"`
	Result string `json:"result,omitempty"`
}

func (s *Service) sourceEnabled(ctx context.Context, id string) error {
	var enabled bool
	var provider string
	if err := s.DB.QueryRowContext(ctx, `SELECT enabled,provider FROM notification_sources WHERE id=?`, id).Scan(&enabled, &provider); err != nil {
		return err
	}
	if !enabled {
		return errors.New("notification source is paused")
	}
	if provider == "github" {
		var v string
		err := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM settings WHERE key='github_integration'),'on')`).Scan(&v)
		if err != nil {
			return err
		}
		if v != "on" {
			return errors.New("GitHub integration is disabled")
		}
	}
	return nil
}
func (s *Service) saveHistory(ctx context.Context, id string, h []trace) error {
	raw, _ := json.Marshal(h)
	_, err := s.DB.ExecContext(ctx, `UPDATE notification_runs SET history=?,updated_at=? WHERE id=?`, string(raw), time.Now().UnixMilli(), id)
	return err
}
func (s *Service) finishRun(ctx context.Context, id, state, result string, h []trace, canRetry ...bool) error {
	retryable := len(canRetry) > 0 && canRetry[0]
	// Write the result before the terminal run state: recovery is conservative
	// if the process stops between these writes.
	if err := s.Inbox.FinishNotification(ctx, id, state, result); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE notification_runs SET state=?,updated_at=?,attention_retryable=?,attention_pending=CASE WHEN ? IN ('failed','blocked','partial','uncertain') AND (attention_question_id='' OR EXISTS(SELECT 1 FROM inbox_questions a WHERE a.id=notification_runs.attention_question_id AND a.reason='automatic_recovery' AND a.status='unanswered')) THEN 1 ELSE attention_pending END WHERE id=?`, state, time.Now().UnixMilli(), retryable, state, id)
	if err != nil {
		return err
	}
	// History is a derived cache. Its failure must not invalidate a confirmed
	// journal outcome or prevent the run from reaching its terminal state.
	cacheCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if e := s.saveHistory(cacheCtx, id, h); e != nil {
		slog.Warn("notification history cache update deferred", "run", id, "error", e)
	}
	return nil
}

// conversation retains what the user saw and the actual tools/results from
// earlier turns, rather than relying on the final prose summary to recover IDs.
func (s *Service) conversation(ctx context.Context, eventID int64, exclude ...string) (string, error) {
	excluded := ""
	if len(exclude) > 0 {
		excluded = exclude[0]
	}
	group := ""
	if excluded != "" {
		if err := s.DB.QueryRowContext(ctx, `SELECT notification_group FROM inbox_questions WHERE id=?`, excluded).Scan(&group); err != nil {
			return "", err
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT q.id,q.summary,q.context,q.question,q.options,q.answer,q.execution_result,q.status,q.reason,COALESCE(r.history,'[]'),COALESCE(r.action,'{}') FROM inbox_questions q JOIN notification_events e ON e.id=q.notification_id LEFT JOIN notification_runs r ON r.id=q.id WHERE ((?!='' AND q.notification_group=?) OR (?='' AND q.notification_group='' AND (e.source_id,e.external_id)=(SELECT source_id,external_id FROM notification_events WHERE id=?))) AND q.id!=? ORDER BY q.created_at DESC,q.rowid DESC LIMIT 10`, group, group, group, eventID, excluded)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, summary, context, question, options, answer, result, status, reason, history, action string
		if err = rows.Scan(&id, &summary, &context, &question, &options, &answer, &result, &status, &reason, &history, &action); err != nil {
			return "", err
		}
		out = append(out, map[string]any{"id": id, "summary": summary, "context": context, "question": question, "options": json.RawMessage(options), "answer": answer, "result": result, "status": status, "reason": reason, "execution_history": json.RawMessage(history), "executed_action": json.RawMessage(action)})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	raw, _ := json.Marshal(map[string]any{"messages": out, "history_may_be_incomplete": len(out) == 10})
	return string(raw), rows.Err()
}
func (s *Service) checkIdentity(ctx context.Context, source Source) error {
	provider, ok := s.Providers[source.Provider]
	if !ok {
		return errors.New("notification provider is unavailable")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	err := provider.CheckIdentity(checkCtx, source)
	if err != nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE notification_sources SET last_error=? WHERE id=?`, err.Error(), source.ID)
	}
	return err
}

package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"kamacu/internal/notifications"
	"kamacu/internal/session"
	"kamacu/internal/worktree"
)

type inboxActionRuntime struct {
	db  *sql.DB
	mgr *session.Manager
	wt  *worktree.Service
}

// ConfigureNotificationActions shares the terminal input, session shutdown and
// transactional board/recurrence primitives with the normal UI paths.
func ConfigureNotificationActions(s *notifications.Service, db *sql.DB, mgr *session.Manager, wt *worktree.Service) {
	s.Actions = &inboxActionRuntime{db: db, mgr: mgr, wt: wt}
}
func (r *inboxActionRuntime) Session(ctx context.Context, id string) (notifications.ActionSession, error) {
	sess, ok := r.mgr.Get(id)
	if !ok {
		return notifications.ActionSession{}, errors.New("specified session is no longer registered")
	}
	info := sess.Info()
	obs := sess.Observation()
	if info.Kind != session.KindAgent || info.TaskID <= 0 {
		return notifications.ActionSession{}, errors.New("action target is not a task agent")
	}
	return notifications.ActionSession{ID: id, TaskID: info.TaskID, Input: obs.Input, Revision: obs.Revision}, nil
}
func (r *inboxActionRuntime) Ready(ctx context.Context, id string, policy string) (notifications.ActionSession, error) {
	sess, ok := r.mgr.Get(id)
	if !ok {
		return notifications.ActionSession{}, fmt.Errorf("%w: session no longer exists", notifications.ErrActionBlocked)
	}
	obs, err := sess.WaitInboxReady(ctx, policy == "send_now")
	if err != nil {
		return notifications.ActionSession{}, fmt.Errorf("%w: %v", notifications.ErrActionBlocked, err)
	}
	info := sess.Info()
	return notifications.ActionSession{ID: id, TaskID: info.TaskID, Input: obs.Input, Revision: obs.Revision}, nil
}
func (r *inboxActionRuntime) Submit(ctx context.Context, expected notifications.ActionSession, message string) error {
	sess, ok := r.mgr.Get(expected.ID)
	if !ok || sess.Info().TaskID != expected.TaskID {
		return fmt.Errorf("%w: target session changed", notifications.ErrActionBlocked)
	}
	body, submit := wrapInputForWrite(message)
	err := sess.WriteObserved(expected.Revision, expected.Input, []byte(body), []byte(submit))
	if errors.Is(err, session.ErrObservedChanged) {
		return fmt.Errorf("%w: %v", notifications.ErrActionBlocked, err)
	}
	return err
}
func (r *inboxActionRuntime) Stop(ctx context.Context, expected notifications.ActionSession) error {
	sess, ok := r.mgr.Get(expected.ID)
	if !ok || sess.Info().TaskID != expected.TaskID {
		return fmt.Errorf("%w: target session changed", notifications.ErrActionBlocked)
	}
	err := sess.StopForInbox(ctx, expected.Input)
	if errors.Is(err, session.ErrObservedChanged) {
		return fmt.Errorf("%w: %v", notifications.ErrActionBlocked, err)
	}
	return err
}
func (r *inboxActionRuntime) Move(ctx context.Context, id int64, expected, status string) error {
	// A completion action must not mark a replacement/racing session as done.
	unlock := r.mgr.LockTaskLifecycle(id)
	defer unlock()
	for _, info := range r.mgr.ListByTask(id) {
		if info.Kind == session.KindAgent && info.Status == session.StatusRunning {
			return fmt.Errorf("%w: task has a running agent", notifications.ErrActionBlocked)
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current, source string
	if err = tx.QueryRowContext(ctx, `SELECT status,source FROM tasks WHERE id=?`, id).Scan(&current, &source); err != nil {
		return err
	}
	if current != expected || source != "manual" {
		return fmt.Errorf("%w: task status or role changed", notifications.ErrActionBlocked)
	}
	var pos float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=(SELECT project_id FROM tasks WHERE id=?) AND status=? AND id!=? AND source='manual'`, id, status, id).Scan(&pos); err != nil {
		return err
	}
	h := &taskHandlers{db: r.db, wt: r.wt}
	_, after, err := h.persistTaskMove(ctx, tx, id, status, pos)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if after != nil {
		after()
	}
	return nil
}

package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
)

const MoveToReview = "move_to_review"
const MoveToDone = "move_to_done"

func CompletionTarget(action string) string {
	switch action {
	case MoveToReview:
		return "in_review"
	case MoveToDone:
		return "done"
	default:
		return ""
	}
}

// CompletionOptions intersects model suggestions with possible forward moves.
func CompletionOptions(taskID int64, source, status string, suggested []string) []Option {
	options := []Option{}
	if taskID <= 0 || source != "manual" {
		return options
	}
	if (status == "todo" || status == "in_progress") && slices.Contains(suggested, MoveToReview) {
		options = append(options, Option{MoveToReview, "Move to review and close session"})
	}
	if (status == "todo" || status == "in_progress" || status == "in_review") && slices.Contains(suggested, MoveToDone) {
		options = append(options, Option{MoveToDone, "Move to done and close session"})
	}
	return options
}

// SetCompletionMover shares the board's move/recurrence transaction. The returned
// callback runs after commit and outside the inbox lock (e.g. worktree setup).
func (s *Service) SetCompletionMover(move func(context.Context, *sql.Tx, int64, string) (func(), error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moveCompletion = move
}

func (s *Service) prepareCompletion(ctx context.Context, who Identity, c *Create) error {
	var source, status string
	if who.TaskID > 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT source,status FROM tasks WHERE id=?`, who.TaskID).Scan(&source, &status); err != nil {
			return err
		}
	}
	var suggested []string
	for _, option := range c.Options {
		suggested = append(suggested, option.ID)
	}
	c.Options = CompletionOptions(who.TaskID, source, status, suggested)
	c.completionTaskStatus = status
	return nil
}

func (s *Service) moveCompletedTask(ctx context.Context, tx *sql.Tx, q Question, action string) (func(), error) {
	if s.moveCompletion == nil {
		return nil, fmt.Errorf("%w: task moves are unavailable", ErrConflict)
	}
	if err := s.valid(Identity{q.TaskID, q.SessionID}); err != nil {
		return nil, err
	}
	live := s.lookup(q.SessionID)
	if q.Origin == "observer" && (live.Revision != q.EvidenceRevision || live.Input != q.InputGeneration) {
		return nil, fmt.Errorf("%w: the terminal changed; review the current message or open the task", ErrConflict)
	}
	var source, status string
	if err := tx.QueryRowContext(ctx, `SELECT source,status FROM tasks WHERE id=?`, q.TaskID).Scan(&source, &status); err != nil {
		return nil, err
	}
	if status != q.CompletionTaskStatus || len(CompletionOptions(q.TaskID, source, status, []string{action})) != 1 {
		return nil, fmt.Errorf("%w: the task status changed; review the refreshed message or open the task", ErrConflict)
	}
	return s.moveCompletion(ctx, tx, q.TaskID, CompletionTarget(action))
}

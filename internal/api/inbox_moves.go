package api

import (
	"context"
	"database/sql"

	"kamacu/internal/inbox"
	"kamacu/internal/worktree"
)

// ConfigureInboxTaskMoves preserves the same timestamps, ordering and recurring
// task behavior as a board move, within the inbox answer transaction.
func ConfigureInboxTaskMoves(service *inbox.Service, db *sql.DB, wt *worktree.Service) {
	h := &taskHandlers{db: db, wt: wt}
	service.SetCompletionMover(func(ctx context.Context, tx *sql.Tx, id int64, status string) (func(), error) {
		var pos float64
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=(SELECT project_id FROM tasks WHERE id=?) AND status=? AND id!=? AND source='manual'`, id, status, id).Scan(&pos)
		if err != nil {
			return nil, err
		}
		_, after, err := h.persistTaskMove(ctx, tx, id, status, pos)
		return after, err
	})
}

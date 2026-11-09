package inbox

import "context"

// Links are navigation/provenance, not terminal ownership. Opening a linked task
// must never answer or dismiss a notification or send input to an agent.
func (s *Service) LinkNotificationTasks(ctx context.Context, questionID string, links []TaskLink) error {
	if len(links) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var origin string
	if err = tx.QueryRowContext(ctx, `SELECT origin FROM inbox_questions WHERE id=?`, questionID).Scan(&origin); err != nil {
		return err
	}
	if origin != "notification" {
		return ErrInvalid
	}
	changed := false
	for _, link := range links {
		var project int64
		if err = tx.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id=?`, link.TaskID).Scan(&project); err != nil {
			return err
		}
		if project != link.ProjectID {
			return ErrInvalid
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO notification_task_links(question_id,task_id,role,source,evidence) VALUES(?,?,?,?,?) ON CONFLICT(question_id,task_id) DO NOTHING`, questionID, link.TaskID, link.Role, link.Source, link.Evidence)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		changed = changed || n > 0

	}
	if changed {
		if _, err = tx.ExecContext(ctx, `UPDATE inbox_questions SET revision=revision+1,updated_at=? WHERE id=?`, s.now().UnixMilli(), questionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

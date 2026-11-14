package inbox

import "context"

// CatchUp captures the pending queue in one read. The browser keeps this pass
// stable while replies, skips, and newly arriving messages change the live inbox.
func (s *Service) CatchUp(ctx context.Context) ([]Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, selectQuestion+`WHERE q.status='pending' AND q.hidden=0 ORDER BY q.queue_position,q.rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Question{}
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

// History follows explicit task associations (including notification links),
// never text matches. Anchor on the selected message's creation and insertion
// order so later updates and messages cannot leak into its earlier history.
func (s *Service) History(ctx context.Context, id string, offset int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := Page{Items: []Question{}}
	if offset < 0 {
		return page, ErrInvalid
	}
	if _, err := s.get(ctx, id); err != nil {
		return page, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH selected AS (
 SELECT *,rowid AS insertion FROM inbox_questions WHERE id=?
), related AS (
 SELECT task_id FROM selected WHERE task_id IS NOT NULL
 UNION SELECT task_id FROM notification_task_links WHERE question_id=?
) `+selectQuestion+`WHERE q.hidden=0 AND EXISTS (
 SELECT 1 FROM selected current WHERE
 (q.created_at<current.created_at OR (q.created_at=current.created_at AND q.rowid<current.insertion)) AND (
 q.task_id IN (SELECT task_id FROM related)
 OR EXISTS (SELECT 1 FROM notification_task_links link WHERE link.question_id=q.id AND link.task_id IN (SELECT task_id FROM related))
 OR (current.task_id IS NULL AND current.origin!='notification' AND q.task_id IS NULL AND q.origin!='notification')
 OR (current.origin='notification' AND q.origin='notification' AND (
 (current.notification_id IS NOT NULL AND q.notification_id=current.notification_id)
 OR (current.notification_group!='' AND q.notification_group=current.notification_group)))
 )) ORDER BY q.created_at DESC,q.rowid DESC LIMIT 51 OFFSET ?`, id, id, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, q)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		page.HasMore = true
	}
	return page, nil
}

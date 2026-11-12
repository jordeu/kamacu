package notifications

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type NotificationRepair struct {
	EventID  int64  `json:"event_id"`
	Revision int64  `json:"revision"`
	BatchID  string `json:"batch_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
}

// Inspect only the latest completed decision for each source/subject. A newer
// complete decision or already-pending notification makes old repair obsolete.
const repairQuery = `WITH latest AS (
 SELECT e.id AS event_id,e.raw_revision AS revision,b.id AS batch_id,
 json_extract(e.payload,'$.url') AS url,json_extract(e.payload,'$.title') AS title,
 e.source_id,e.raw_status,e.state,
 COALESCE(json_extract(j.value,'$.source.evidence_complete'),0) AS complete,
 row_number() OVER(PARTITION BY e.source_id,json_extract(e.payload,'$.url') ORDER BY b.created_at DESC,e.id DESC) AS n
 FROM notification_batches b JOIN notification_batch_events be ON be.batch_id=b.id
 JOIN notification_events e ON e.id=be.event_id JOIN notification_sources src ON src.id=e.source_id,
 json_each(b.processor_snapshot,'$.subjects') j
 WHERE b.status='completed' AND b.created_at>=? AND src.enabled=1 AND src.retired=0
 AND json_extract(j.value,'$.source.url')=json_extract(e.payload,'$.url')
 AND json_extract(e.payload,'$.subject_type') IN ('PullRequest','Issue')
) SELECT event_id,revision,batch_id,url,title FROM latest l
 WHERE n=1 AND complete=0 AND raw_status='archived' AND state='ignored'
 AND NOT EXISTS(SELECT 1 FROM notification_events pending WHERE pending.source_id=l.source_id AND json_extract(pending.payload,'$.url')=l.url AND pending.raw_status='pending')
 ORDER BY event_id DESC LIMIT 100`

type repairReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func repairPreview(ctx context.Context, q repairReader) ([]NotificationRepair, error) {
	rows, err := q.QueryContext(ctx, repairQuery, time.Now().Add(-7*24*time.Hour).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationRepair{}
	for rows.Next() {
		var r NotificationRepair
		if err = rows.Scan(&r.EventID, &r.Revision, &r.BatchID, &r.URL, &r.Title); err != nil {
			return nil, err
		}
		r.Reason = "The last decision ignored this notification with incomplete source evidence."
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) NotificationRepairPreview(ctx context.Context) ([]NotificationRepair, error) {
	return repairPreview(ctx, s.DB)
}
func (s *Service) RepairNotifications(ctx context.Context, selected []NotificationRepair) error {
	if len(selected) == 0 || len(selected) > 100 {
		return errors.New("select between 1 and 100 repair candidates")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_batches WHERE status IN ('queued','running')`).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrProcessing
	}
	candidates, err := repairPreview(ctx, tx)
	if err != nil {
		return err
	}
	available := map[int64]NotificationRepair{}
	for _, c := range candidates {
		available[c.EventID] = c
	}
	seen := map[int64]bool{}
	for _, chosen := range selected {
		current, ok := available[chosen.EventID]
		if !ok || seen[chosen.EventID] || current.Revision != chosen.Revision || current.BatchID != chosen.BatchID {
			return errors.New("repair preview changed; check for missed notifications again")
		}
		seen[chosen.EventID] = true
		if _, err = tx.ExecContext(ctx, `UPDATE notification_events SET raw_status='pending',state='queued',raw_revision=raw_revision+1,last_error='',next_attempt=0 WHERE id=? AND raw_revision=?`, chosen.EventID, chosen.Revision); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO notification_activity_subjects(source_id,subject_url,account,repair_from) SELECT e.source_id,?,s.account,? FROM notification_events e JOIN notification_sources s ON s.id=e.source_id WHERE e.id=? ON CONFLICT(source_id,subject_url) DO UPDATE SET fetched_through='',reconciled_at=0,repair_from=excluded.repair_from`, current.URL, time.Now().Add(-7*24*time.Hour).UTC().Format("2006-01-02T15:04:05.000000000Z"), chosen.EventID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

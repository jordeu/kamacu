package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"kamacu/internal/inbox"
)

var ErrNoPending = errors.New("no pending notifications to process")
var ErrProcessing = errors.New("notifications are being processed; try again when processing finishes")

type RawNotification struct {
	Processing bool   `json:"processing,omitempty"`
	ID         int64  `json:"id"`
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	Status     string `json:"status"`
	Revision   int64  `json:"revision"`
	CreatedAt  int64  `json:"created_at"`
	Event      Event  `json:"event"`
}
type RawPage struct {
	Items        []RawNotification `json:"items"`
	PendingCount int               `json:"pending_count"`
	HasMore      bool              `json:"has_more"`
}
type Processing struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	Error             string `json:"error"`
	ActionCount       int    `json:"action_count"`
	NotificationCount int    `json:"notification_count"`
}
type Status struct {
	PendingCount int         `json:"pending_count"`
	Processing   *Processing `json:"processing"`
}

const processingColumns = `id,status,created_at,updated_at,error,action_count,(SELECT count(*) FROM notification_batch_events e WHERE e.batch_id=notification_batches.id)`

func scanProcessing(row interface{ Scan(...any) error }) (p Processing, err error) {
	err = row.Scan(&p.ID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.Error, &p.ActionCount, &p.NotificationCount)
	return
}
func (s *Service) Status(ctx context.Context) (Status, error) {
	var status Status
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&status.PendingCount); err != nil {
		return status, err
	}
	p, err := scanProcessing(s.DB.QueryRowContext(ctx, `SELECT `+processingColumns+` FROM notification_batches ORDER BY created_at DESC,rowid DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Processing = &p
	return status, nil
}
func (s *Service) ListRaw(ctx context.Context, status string, offset int) (RawPage, error) {
	page := RawPage{Items: []RawNotification{}}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&page.PendingCount); err != nil {
		return page, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.source_id,s.name,e.raw_status,e.raw_revision,e.created_at,e.payload,EXISTS(SELECT 1 FROM notification_batch_events m JOIN notification_batches b ON b.id=m.batch_id WHERE m.event_id=e.id AND b.status IN ('queued','running')) FROM notification_events e JOIN notification_sources s ON s.id=e.source_id WHERE e.raw_status=? ORDER BY e.id DESC LIMIT 51 OFFSET ?`, status, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var n RawNotification
		var payload string
		if err = rows.Scan(&n.ID, &n.SourceID, &n.SourceName, &n.Status, &n.Revision, &n.CreatedAt, &payload, &n.Processing); err != nil {
			return page, err
		}
		if err = json.Unmarshal([]byte(payload), &n.Event); err != nil {
			return page, err
		}
		page.Items = append(page.Items, n)
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		page.HasMore = true
	}
	return page, rows.Err()
}
func (s *Service) SetRawStatus(ctx context.Context, id int64, status string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_batch_events e JOIN notification_batches b ON b.id=e.batch_id WHERE e.event_id=? AND b.status IN ('queued','running')`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrProcessing
	}
	r, err := tx.ExecContext(ctx, `UPDATE notification_events SET raw_status=?,raw_revision=raw_revision+1 WHERE id=? AND raw_status!=?`, status, id, status)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_events WHERE id=?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
	}
	return tx.Commit()
}

// RequestProcessing records an exact snapshot of all pending notifications. A
// second click returns the active run, and later arrivals stay pending.
func (s *Service) RequestProcessing(ctx context.Context) (Processing, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Processing{}, err
	}
	defer tx.Rollback()
	active, err := scanProcessing(tx.QueryRowContext(ctx, `SELECT `+processingColumns+` FROM notification_batches WHERE status IN ('queued','running')`))
	if err == nil {
		return active, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Processing{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&count); err != nil {
		return Processing{}, err
	}
	if count == 0 {
		return Processing{}, ErrNoPending
	}
	now := time.Now().UnixMilli()
	p := Processing{ID: uuid.NewString(), Status: "queued", CreatedAt: now, UpdatedAt: now, NotificationCount: count}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notification_batches(id,status,created_at,updated_at) VALUES(?,'queued',?,?)`, p.ID, now, now); err != nil {
		return p, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notification_batch_events SELECT ?,id,raw_revision FROM notification_events WHERE raw_status='pending'`, p.ID); err != nil {
		return p, err
	}
	if err = inbox.CaptureNotificationBatchQuestions(ctx, tx, p.ID); err != nil {
		return p, err
	}

	return p, tx.Commit()
}

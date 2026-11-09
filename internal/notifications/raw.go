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
	Processing bool               `json:"processing,omitempty"`
	ID         int64              `json:"id"`
	SourceID   string             `json:"source_id"`
	SourceName string             `json:"source_name"`
	Status     string             `json:"status"`
	Revision   int64              `json:"revision"`
	CreatedAt  int64              `json:"created_at"`
	Decisions  []ActivityDecision `json:"decisions,omitempty"`
	Event      Event              `json:"event"`
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
	ModelConfigured bool        `json:"model_configured"`
	PendingCount    int         `json:"pending_count"`
	Processing      *Processing `json:"processing"`
}

const processingColumns = `id,status,created_at,updated_at,error,action_count,(SELECT count(*) FROM notification_batch_events e WHERE e.batch_id=notification_batches.id)`

func scanProcessing(row interface{ Scan(...any) error }) (p Processing, err error) {
	err = row.Scan(&p.ID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.Error, &p.ActionCount, &p.NotificationCount)
	return
}
func (s *Service) Status(ctx context.Context) (Status, error) {
	var status Status
	config, err := s.Config(ctx)
	if err != nil {
		return status, err
	}
	status.ModelConfigured = config.ProfileID != nil
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
	if err := rows.Err(); err != nil {
		return page, err
	}
	rows.Close()
	for i := range page.Items {
		decisions, err := s.EventDecisions(ctx, page.Items[i].ID)
		if err != nil {
			return page, err
		}
		page.Items[i].Decisions = decisions
	}
	return page, nil
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
	config, err := s.Config(ctx)
	if err != nil {
		return Processing{}, err
	}
	profile, err := s.notificationProfile(ctx)
	if err != nil {
		return Processing{}, err
	}
	frozen := mustJSON(map[string]any{"settings": config, "profile": profile})
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
	var rulesID sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT active_set_id FROM notification_rule_state WHERE id=1`).Scan(&rulesID); err != nil {
		return Processing{}, err
	}
	if !rulesID.Valid {
		return Processing{}, ErrNoActiveRules
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO notification_batches(id,status,created_at,updated_at,rule_set_id,processor_config) VALUES(?,'queued',?,?,?,?)`, p.ID, now, now, rulesID.String, frozen); err != nil {
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

func (s *Service) EventDecisions(ctx context.Context, id int64) ([]ActivityDecision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id,activity_id,event_ids,outcome,evaluations,facts,semantic,b.rule_set_id,r.revision,d.explanation,d.rule_ids,d.evidence_refs FROM notification_rule_decisions d JOIN notification_batches b ON b.id=d.batch_id JOIN notification_rule_sets r ON r.id=b.rule_set_id WHERE EXISTS(SELECT 1 FROM json_each(d.event_ids) WHERE value=?) ORDER BY d.rowid DESC LIMIT 30`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityDecision{}
	for rows.Next() {
		var d ActivityDecision
		var ids, evals, facts, semantic, ruleIDs, evidenceRefs string
		if err = rows.Scan(&d.BatchID, &d.ID, &ids, &d.Outcome, &evals, &facts, &semantic, &d.RuleSetID, &d.RuleSetRevision, &d.Reason, &ruleIDs, &evidenceRefs); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(ids), &d.EventIDs); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(evals), &d.Evaluations); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(facts), &d.Facts); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(semantic), &d.Semantic); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(ruleIDs), &d.RuleIDs); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(evidenceRefs), &d.EvidenceRefs); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

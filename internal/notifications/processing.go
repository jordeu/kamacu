package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"kamacu/internal/diagnostics"
)

func (s *Service) processNext(ctx context.Context) error {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM notification_batches WHERE status='queued' ORDER BY created_at LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_batches SET status='running',updated_at=? WHERE id=? AND status='queued'`, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil
	}
	err = s.processBatch(ctx, id)
	if err != nil {
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, recordErr := s.DB.ExecContext(recordCtx, `UPDATE notification_batches SET status='failed',error=?,updated_at=? WHERE id=? AND status='running'`, err.Error(), time.Now().UnixMilli(), id)
		return recordErr
	}
	return nil
}
func (s *Service) batchEvents(ctx context.Context, id string) ([]RawNotification, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.source_id,s.name,e.raw_status,e.raw_revision,e.created_at,e.payload FROM notification_batch_events b JOIN notification_events e ON e.id=b.event_id JOIN notification_sources s ON s.id=e.source_id WHERE b.batch_id=? ORDER BY e.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RawNotification{}
	for rows.Next() {
		var r RawNotification
		var payload string
		if err = rows.Scan(&r.ID, &r.SourceID, &r.SourceName, &r.Status, &r.Revision, &r.CreatedAt, &payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(payload), &r.Event); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) processBatch(ctx context.Context, id string) (resultErr error) {
	start := time.Now()
	ctx = diagnostics.WithGroup(ctx, diagnostics.Group{Kind: "batch", ID: id, StartedAt: start})
	ctx = withNotificationRun(ctx, id)
	defer func() {
		slog.Info("notification batch finished", "batch", id, "duration_ms", time.Since(start).Milliseconds(), "error", resultErr)
	}()
	run, cancel, config, err := s.modelContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	ctx = run
	var captured string
	if err = s.DB.QueryRowContext(ctx, `SELECT processor_config FROM notification_batches WHERE id=?`, id).Scan(&captured); err != nil {
		return err
	}
	profile, err := s.notificationProfile(ctx)
	if err != nil {
		return err
	}
	expected := mustJSON(map[string]any{"settings": config, "profile": profile})
	if captured != expected {
		return ErrRulesChanged
	}
	rules, err := s.batchRuleSet(ctx, id)
	if err != nil {
		return err
	}
	if err = s.checkRuleSet(ctx, rules); err != nil {
		return err
	}
	events, err := s.batchEvents(ctx, id)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return ErrNoPending
	}
	if _, err = s.Inbox.NotificationBatchQuestions(ctx, id); err != nil {
		return err
	}
	snapshot, err := s.agentSnapshot(ctx, events, rules)
	if err != nil {
		return err
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE notification_batches SET processor_snapshot=? WHERE id=?`, mustJSON(snapshot), id); err != nil {
		return err
	}
	turn, trace, err := s.runNotificationAgentWithUpdates(ctx, &snapshot, config, true)
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer recordCancel()
	if _, recordErr := s.DB.ExecContext(recordCtx, `UPDATE notification_batches SET processor_snapshot=?,processor_result=?,processor_trace=? WHERE id=?`, mustJSON(snapshot), mustJSON(turn), mustJSON(trace), id); recordErr != nil {
		return recordErr
	}
	if err != nil {
		return err
	}
	actions, refs, decisions, err := s.agentActions(ctx, snapshot, turn)
	if err != nil {
		return err
	}
	if err = s.checkRuleSet(ctx, rules); err != nil {
		return err
	}
	freshConfig, err := s.Config(ctx)
	if err != nil {
		return err
	}
	freshProfile, err := s.notificationProfile(context.WithValue(ctx, notificationProfileKey{}, nil))
	if err != nil {
		return err
	}
	if mustJSON(freshConfig) != mustJSON(config) || freshProfile.Key() != profile.Key() {
		return ErrRulesChanged
	}
	checkedSources := map[string]bool{}
	for _, event := range events {
		if checkedSources[event.SourceID] {
			continue
		}
		checkedSources[event.SourceID] = true
		if err = s.sourceEnabled(ctx, event.SourceID); err != nil {
			return err
		}
		source, err := scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, event.SourceID))
		if err != nil {
			return err
		}
		if err = s.checkIdentity(ctx, source); err != nil {
			return err
		}
	}

	return s.Inbox.ApplyNotificationBatch(ctx, id, actions, refs, decisions...)
}

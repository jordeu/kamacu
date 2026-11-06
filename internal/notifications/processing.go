package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"kamacu/internal/diagnostics"
	"kamacu/internal/inbox"
)

type BatchAction struct {
	Plan     ActionPlan `json:"plan"`
	evidence []triageEvidence
	EventIDs []int64  `json:"event_ids"`
	Summary  string   `json:"summary"`
	Context  string   `json:"context"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}
type BatchDecision struct {
	Actions      []BatchAction `json:"actions"`
	DiscardIDs   []int64       `json:"discard_ids"`
	SupersedeIDs []string      `json:"supersede_ids"`
}

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

type notificationPlan struct {
	actions   []inbox.NotificationBatchAction
	refs      []inbox.NotificationQuestionRef
	groups    []preparedTriageGroup
	decisions []ActivityDecision
}

func (s *Service) processBatch(ctx context.Context, id string) (resultErr error) {
	start := time.Now()
	groupCtx, finishGroup := context.WithCancel(ctx)
	defer finishGroup()
	ctx = diagnostics.WithGroup(groupCtx, diagnostics.Group{Kind: "batch", ID: id, StartedAt: start})
	ctx = withNotificationRun(ctx, id)
	slog.Debug("notification batch started", "batch", id)
	defer func() {
		slog.Info("notification batch finished", "batch", id, "duration_ms", time.Since(start).Milliseconds(), "error", resultErr)
	}()

	events, err := s.batchEvents(ctx, id)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return ErrNoPending
	}
	// Validate the original dependency snapshot before sending any model requests.
	if _, err = s.Inbox.NotificationBatchQuestions(ctx, id); err != nil {
		return err
	}
	groups, err := s.notificationProjectGroups(ctx, events)
	if err != nil {
		return err
	}
	plans := make([]notificationPlan, len(groups))
	if err = parallelNotifications(ctx, len(groups), notificationProjectConcurrency, func(ctx context.Context, i int) error {
		ctx = context.WithValue(ctx, notificationProjectKey{}, groups[i][0].Event.Repository)
		return notificationStage(ctx, "project_prepare", func() error { return s.triageGroup(ctx, id, groups[i], &plans[i]) })
	}); err != nil {
		return err
	}
	// Preserve the previous shared finalization deadline across all project
	// workers, rather than granting each queued wave another five minutes.
	finalizeCtx, cancelFinalize := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelFinalize()
	if err = parallelNotifications(finalizeCtx, len(groups), notificationProjectConcurrency, func(ctx context.Context, i int) error {
		ctx = context.WithValue(ctx, notificationProjectKey{}, groups[i][0].Event.Repository)
		return notificationStage(ctx, "project_finalize", func() error { return s.finalizeTriage(ctx, id, &plans[i]) })
	}); err != nil {
		return err
	}
	var plan notificationPlan
	// Merge in input project order, regardless of worker completion order.
	for _, prepared := range plans {
		plan.actions = append(plan.actions, prepared.actions...)
		plan.refs = append(plan.refs, prepared.refs...)
		for _, group := range prepared.groups {
			for _, subject := range group.subjects {
				plan.decisions = append(plan.decisions, subject.decisions...)
			}
		}
		for _, group := range prepared.groups {
			if err = s.checkTriagePreferences(ctx, group); err != nil {
				return err
			}
		}
	}
	// A project may finish while other workers are still drafting. Check every
	// contributing source again at the shared publication barrier, not just
	// when its own project worker completes.
	if err = notificationStage(ctx, "source_validation", func() error {
		seenSources := map[string]bool{}
		for _, prepared := range plans {
			for _, group := range prepared.groups {
				for _, source := range group.sources {
					if seenSources[source.ID] {
						continue
					}
					seenSources[source.ID] = true
					if err := s.sourceEnabled(ctx, source.ID); err != nil {
						return err
					}
					if err := s.checkIdentity(ctx, source); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Publish once to preserve snapshot validation and avoid partial publication.
	refs := []inbox.NotificationQuestionRef{}
	seen := map[string]bool{}
	for _, ref := range plan.refs {
		if !seen[ref.ID] {
			refs = append(refs, ref)
			seen[ref.ID] = true
		}
	}
	return notificationStage(ctx, "publication", func() error {
		decisions := []inbox.NotificationDecision{}
		for _, d := range plan.decisions {
			decisions = append(decisions, inbox.NotificationDecision{Semantic: mustJSON(d.Semantic), ActivityID: d.ID, EventIDs: mustJSON(d.EventIDs), Outcome: d.Outcome, Evaluations: mustJSON(d.Evaluations), Facts: mustJSON(d.Facts)})
		}
		return s.Inbox.ApplyNotificationBatch(ctx, id, plan.actions, refs, decisions...)
	})
}

func validateBatch(d BatchDecision, events []RawNotification, questions []inbox.Question) ([]inbox.NotificationBatchAction, []inbox.NotificationQuestionRef, error) {
	allowed := map[int64]RawNotification{}
	for _, e := range events {
		allowed[e.ID] = e
	}
	covered := map[int64]bool{}
	discarded := map[int64]bool{}
	for _, id := range d.DiscardIDs {
		if _, ok := allowed[id]; !ok || discarded[id] {
			return nil, nil, errors.New("batch discarded an unknown or duplicate notification")
		}
		discarded[id] = true
		covered[id] = true
	}
	actions := []inbox.NotificationBatchAction{}
	seenActions := map[string]bool{}
	for _, a := range d.Actions {
		step := Step{Action: "ask", Summary: a.Summary, Context: a.Context, Question: a.Question, Options: a.Options}
		raw, _ := json.Marshal(step)
		if _, err := decodeStep(raw); err != nil {
			return nil, nil, err
		}
		if len(a.EventIDs) == 0 {
			return nil, nil, errors.New("batch action has no related notifications")
		}
		related := map[int64]bool{}
		for _, id := range a.EventIDs {
			_, ok := allowed[id]
			if !ok || discarded[id] || related[id] {
				return nil, nil, errors.New("batch action references an unknown, discarded, or duplicate notification")
			}
			related[id] = true
			covered[id] = true
		}
		// Event order and nil/empty choices do not distinguish an action. Keep the
		// first action's display order while rejecting repeated publication of the
		// same model output; different questions/contexts for the same IDs survive.
		canonical := a
		canonical.EventIDs = append([]int64(nil), a.EventIDs...)
		slices.Sort(canonical.EventIDs)
		if canonical.Options == nil {
			canonical.Options = []string{}
		}
		signature, _ := json.Marshal(canonical)
		if seenActions[string(signature)] {
			continue
		}
		seenActions[string(signature)] = true
		c := step.question()
		c.Context += batchActionDetails(a.EventIDs, allowed)
		if len(c.Context) > inboxCardContextLimit {
			return nil, nil, errors.New("notification context including subject links exceeds limits")
		}
		primary := allowed[a.EventIDs[0]]
		actions = append(actions, inbox.NotificationBatchAction{Links: a.Plan.Bindings, EventIDs: a.EventIDs, Request: c, Plan: mustJSON(a.Plan), SourceName: primary.SourceName, SourceURL: primary.Event.URL})
	}
	if len(covered) != len(allowed) {
		return nil, nil, errors.New("batch did not account for every pending notification")
	}
	questionMap := map[string]inbox.Question{}
	for _, q := range questions {
		questionMap[q.ID] = q
	}
	refs := []inbox.NotificationQuestionRef{}
	seen := map[string]bool{}
	for _, id := range d.SupersedeIDs {
		q, ok := questionMap[id]
		if !ok || seen[id] {
			return nil, nil, errors.New("batch superseded an unknown or duplicate inbox question")
		}
		seen[id] = true
		refs = append(refs, inbox.NotificationQuestionRef{ID: id, Revision: q.Revision})
	}
	return actions, refs, nil
}

// groupQuestions keeps existing actions scoped to threads in this subgroup.
func (s *Service) groupQuestions(ctx context.Context, batchID string, events []RawNotification) ([]inbox.Question, error) {
	questions, err := s.Inbox.NotificationBatchQuestions(ctx, batchID)
	if err != nil {
		return nil, err
	}
	threads := map[string]bool{}
	for _, event := range events {
		threads[event.SourceID+":"+event.Event.ExternalID] = true
	}
	out := []inbox.Question{}
	for _, q := range questions {
		rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT e.source_id,e.external_id FROM notification_events e WHERE e.id=? OR e.id IN (SELECT event_id FROM notification_question_events WHERE question_id=?)`, q.NotificationID, q.ID)
		if err != nil {
			return nil, err
		}
		related := false
		for rows.Next() {
			var source, external string
			if err = rows.Scan(&source, &external); err != nil {
				rows.Close()
				return nil, err
			}
			related = related || threads[source+":"+external]
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if related {
			out = append(out, q)
		}
	}
	return out, nil
}

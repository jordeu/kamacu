package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Check once after the model's first proposal. Refresh only changed subjects,
// using an uncached GitHub read scope so both version and content are current.
// The final model pass is bounded: a busy discussion cannot restart the batch
// indefinitely. Execution-time guards still protect any later user action.
func (s *Service) refreshChangedAgentSubjects(ctx context.Context, snapshot *AgentSnapshot) ([]AgentSubject, error) {
	s.setProcessingStage(ctx, "checking_updates", len(snapshot.Subjects))
	sources := map[string]Source{}
	for _, sub := range snapshot.Subjects {
		if len(sub.Notifications) == 0 {
			return nil, errors.New("subject has no source notifications")
		}
		id := sub.Notifications[0].SourceID
		if _, ok := sources[id]; ok {
			continue
		}
		source, err := scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, id))
		if err != nil {
			return nil, err
		}
		if err = s.sourceEnabled(ctx, id); err != nil {
			return nil, err
		}
		if err = s.checkIdentity(ctx, source); err != nil {
			return nil, err
		}
		sources[id] = source
	}
	changed := make([]bool, len(snapshot.Subjects))
	if err := parallelNotifications(ctx, len(snapshot.Subjects), notificationSubjectConcurrency, func(ctx context.Context, i int) (err error) {
		defer func() {
			if err == nil {
				s.advanceProcessing(ctx)
			}
		}()
		sub := &snapshot.Subjects[i]
		source := sources[sub.Notifications[0].SourceID]
		reader, ok := s.Providers[source.Provider].(interface {
			SubjectVersion(context.Context, Source, Event) (string, error)
		})
		if !ok || (sub.Event.SubjectUpdatedAt == "" && sub.Event.ActivityDelta == nil) {
			return nil
		}
		freshCtx := freshGitHubReadCache(ctx)
		version, err := reader.SubjectVersion(freshCtx, source, sub.Event)
		if err != nil {
			return err
		}
		if version == sub.Event.SubjectUpdatedAt && sub.Event.ActivityDelta == nil {
			return nil
		}
		fresh, err := s.enrichSubject(freshCtx, source, sub.Event)
		if err != nil {
			return err
		}
		if sub.Event.ActivityDelta != nil && activityFingerprint(fresh) == activityFingerprint(sub.Event) {
			return nil
		}
		// Stable subject/event identities preserve grouping and notification revisions.
		updateAgentSubject(sub, fresh)
		for j := range sub.Event.Evidence {
			sub.Event.Evidence[j].ID = fmt.Sprintf("%s/final/E%d", sub.ID, j+1)
		}
		changed[i] = true
		return nil
	}); err != nil {
		return nil, err
	}
	updates := []AgentSubject{}
	for i, sub := range snapshot.Subjects {
		if changed[i] {
			updates = append(updates, sub)
		}
	}
	return updates, nil
}

// Keep the original trace for auditing, but never let obsolete source reads
// establish facts after a refresh. Read-only task/session context remains usable.
func supersedeSourceReads(trace []AgentReadResult, updates []AgentSubject) {
	ids := map[string]bool{}
	for _, sub := range updates {
		ids[sub.ID] = true
	}
	for i := range trace {
		r := &trace[i]
		if (r.Name == "read_activity" || r.Name == "read_thread") && ids[r.SubjectID] {
			r.Superseded = true
		}
		if r.Name == "read_subject" {
			var args struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(r.Arguments), &args) == nil && ids[args.ID] {
				r.Superseded = true
			}
		}
		if slices.Contains([]string{"get_pr_context", "get_task_pr_context", "list_pending_reviews", "list_recently_reviewed"}, r.Name) {
			r.Superseded = true
		}
	}
}

// Do not duplicate refreshed source bodies in both BATCH and READ RESULTS.
// Full source updates and superseded read contents remain in the saved trace.
func agentTraceForPrompt(trace []AgentReadResult, updatesInBatch bool) []AgentReadResult {
	out := append([]AgentReadResult(nil), trace...)
	for i := range out {
		r := &out[i]
		if r.Superseded {
			r.Result = `{"superseded":true,"note":"Use the refreshed source evidence instead."}`
		}
		if updatesInBatch && r.Name == "source_updates" {
			var update struct {
				Subjects []AgentSubject `json:"subjects"`
				Proposal AgentTurn      `json:"previous_proposal"`
			}
			if json.Unmarshal([]byte(r.Result), &update) == nil {
				ids := []string{}
				for _, sub := range update.Subjects {
					ids = append(ids, sub.ID)
				}
				r.Result = mustJSON(map[string]any{"subject_ids": ids, "previous_proposal": update.Proposal, "source_details": "Refreshed activity and subject metadata are in BATCH. When BATCH is compact, use read_subject for full frozen source details."})
			}
		}
	}
	return out
}

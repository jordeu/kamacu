package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/store"
)

// InboxEvidence refreshes registered source evidence without using notification
// rules or acknowledging activity. The inbox agent owns its own decisions.
func (s *Service) InboxEvidence(ctx context.Context, q inbox.Question) (json.RawMessage, error) {
	ctx = freshGitHubReadCache(ctx)
	if q.Origin != "notification" {
		return json.RawMessage(`[]`), nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT e.source_id,e.payload FROM notification_events e WHERE e.id=? OR e.id IN (SELECT event_id FROM notification_question_events WHERE question_id=?) ORDER BY e.id`, q.NotificationID, q.ID)
	if err != nil {
		return nil, err
	}
	type input struct {
		source string
		event  Event
	}
	inputs := []input{}
	for rows.Next() {
		var in input
		var raw string
		if err = rows.Scan(&in.source, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &in.event); err != nil {
			rows.Close()
			return nil, err
		}
		inputs = append(inputs, in)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, errors.New("notification source evidence is unavailable")
	}
	out := []any{}
	seen := map[string]bool{}
	for _, in := range inputs {
		key := subjectKey(in.source, in.event)
		if seen[key] {
			continue
		}
		seen[key] = true
		source, err := scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, in.source))
		if err != nil {
			return nil, err
		}
		if err = s.sourceEnabled(ctx, source.ID); err != nil {
			return nil, err
		}
		if err = s.checkIdentity(ctx, source); err != nil {
			return nil, err
		}
		event, err := s.inboxEvidenceEvent(ctx, source, in.event)
		if err != nil {
			return nil, err
		}
		if !event.EvidenceComplete {
			return nil, errors.New("current notification evidence is incomplete")
		}
		if event.PRFacts != nil {
			event.PRFacts.ObservedAt = time.Time{}
		}
		// Exclude retrieval clocks/checkpoints from the equality check immediately
		// before authorization. Keep all source text, review facts and source identity.
		out = append(out, map[string]any{"source_id": source.ID, "notification_reason": event.Reason, "viewer": source.Account, "repository": event.Repository, "url": event.URL, "title": event.SubjectTitle, "state": event.SubjectState, "context": event.Context, "evidence": event.Evidence, "pr_facts": event.PRFacts, "warnings": event.ContextWarnings})
	}
	return json.Marshal(out)
}

// Inbox authorization needs a stable full read, independent of the notification
// collector's moving delta cursors and processed-activity flags. Do not advance
// that cache as a side effect of checking whether a reply is authorized.
func (s *Service) inboxEvidenceEvent(ctx context.Context, source Source, event Event) (Event, error) {
	var github GitHub
	switch provider := s.Providers[source.Provider].(type) {
	case GitHub:
		github = provider
	case *GitHub:
		github = *provider
	default:
		return s.enrichSubject(ctx, source, event)
	}
	github.DB = nil
	fresh, err := github.Enrich(ctx, source, event)
	if err != nil {
		return fresh, err
	}
	if event.SubjectType == "PullRequest" {
		repo, number, err := store.PRIdentity(event.URL, 0)
		if err != nil {
			return fresh, err
		}
		facts := github.PRContext(ctx, source, repo, number)
		fresh.PRFacts = &facts
	}
	return fresh, nil
}

package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// InboxEvidence refreshes registered source evidence without using notification
// rules or acknowledging activity. The inbox agent owns its own decisions.
func (s *Service) InboxEvidence(ctx context.Context, q inbox.Question) (json.RawMessage, error) {
	ctx = freshGitHubReadCache(ctx)
	if q.Origin != "notification" {
		return s.inboxReviewEvidence(ctx, q)
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
		// Full source reads avoid collection cursors; review-history restoration
		// is read-only and still needs the durable, account-scoped activity cache.
		github.DB = s.DB
		facts := github.PRContext(ctx, source, repo, number)
		fresh.PRFacts = &facts
	}
	return fresh, nil
}

// Review-session questions need the same verified historical facts as the
// notification that started the review, including when asking to publish it.
func (s *Service) inboxReviewEvidence(ctx context.Context, q inbox.Question) (json.RawMessage, error) {
	empty := json.RawMessage(`[]`)
	if q.Origin != "observer" || q.TaskID <= 0 {
		return empty, nil
	}
	links, err := store.PRLinks(ctx, s.DB, q.TaskID, "", 0)
	if err != nil {
		return nil, err
	}
	reviews := []store.PRLink{}
	for _, link := range links {
		if link.Role == "review" {
			reviews = append(reviews, link)
		}
	}
	if len(reviews) == 0 {
		return empty, nil
	}
	gate, err := settings.Get(s.DB, settings.KeyGithubIntegration)
	if err != nil {
		return nil, err
	}
	if gate != "on" {
		return empty, nil
	}
	sources, err := s.Sources(ctx)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, source := range sources {
		if !source.Enabled || source.Provider != "github" || source.Account == "" {
			continue
		}
		reader, ok := s.Providers[source.Provider].(interface {
			PRContext(context.Context, Source, string, int64) PRFacts
		})
		if !ok {
			continue
		}
		if err = s.sourceEnabled(ctx, source.ID); err != nil {
			return nil, err
		}
		if err = s.checkIdentity(ctx, source); err != nil {
			return nil, err
		}
		for _, link := range reviews {
			readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			facts := reader.PRContext(readCtx, source, link.Repository, link.Number)
			facts = (GitHub{DB: s.DB}).WithRecordedApprovalHistory(readCtx, source, facts)
			cancel()
			facts.ObservedAt = time.Time{}
			out = append(out, map[string]any{"source_id": source.ID, "viewer": source.Account, "repository": link.Repository, "url": link.URL, "pr_facts": facts})
		}
	}
	return json.Marshal(out)
}

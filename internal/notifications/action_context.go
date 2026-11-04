package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type actionContext struct {
	Events   []Event
	Sources  map[string]Source
	Projects map[string]json.RawMessage
	Subjects map[string]Event
}

func (s *Service) loadActionContext(ctx context.Context, questionID string) (actionContext, error) {
	out := actionContext{Sources: map[string]Source{}, Projects: map[string]json.RawMessage{}, Subjects: map[string]Event{}}
	ids, err := s.Inbox.RelatedNotificationIDs(ctx, questionID)
	if err != nil {
		return out, err
	}
	if len(ids) == 0 {
		return out, errors.New("notification action has no source notifications")
	}
	for _, id := range ids {
		var sourceID, payload string
		if err = s.DB.QueryRowContext(ctx, `SELECT source_id,payload FROM notification_events WHERE id=?`, id).Scan(&sourceID, &payload); err != nil {
			return out, err
		}
		var event Event
		if err = json.Unmarshal([]byte(payload), &event); err != nil {
			return out, err
		}
		if _, ok := out.Sources[sourceID]; !ok {
			source, e := scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM notification_sources WHERE id=?`, sourceID))
			if e != nil {
				return out, e
			}
			if e = s.sourceEnabled(ctx, sourceID); e != nil {
				return out, e
			}
			out.Sources[sourceID] = source
		}
		if _, ok := out.Projects[event.Repository]; !ok {
			known, e := s.projectContext(ctx, event.Repository)
			if e != nil {
				return out, e
			}
			out.Projects[event.Repository] = json.RawMessage(known)
		}
		out.Subjects[subjectKey(sourceID, event)] = event
		event.Context = ""
		out.Events = append(out.Events, event)
	}
	budget := min(16000, 80000/max(1, len(out.Subjects)))
	for key, event := range out.Subjects {
		sourceID, _, _ := strings.Cut(key, ":")
		source := out.Sources[sourceID]
		if e := s.checkIdentity(ctx, source); e != nil {
			return out, e
		}
		fresh, e := s.enrichSubject(ctx, source, event)
		if e != nil {
			return out, e
		}
		compactWorkDecoration(&fresh, min(workDecorationLimit, 32000/max(1, len(out.Subjects))))
		if len(fresh.Context) > budget {
			fresh.Context = strings.ToValidUTF8(fresh.Context[:budget], "") + "\n[truncated]"
		}
		out.Subjects[key] = fresh
	}
	return out, nil
}
func subjectKey(sourceID string, event Event) string {
	subject := event.SubjectURL
	if subject == "" {
		subject = event.URL
	}
	if subject == "" {
		subject = event.ExternalID
	}
	return sourceID + ":" + subject
}

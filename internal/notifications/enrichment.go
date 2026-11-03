package notifications

import (
	"context"
	"errors"
	"time"
)

// enrichSubject lets declared unavailable resources remain visible to triage,
// while authentication, identity, rate-limit and transport failures still abort.
// GitHub can retain verified subject state when only an optional comment is gone.
func (s *Service) enrichSubject(ctx context.Context, source Source, event Event) (Event, error) {
	provider, ok := s.Providers[source.Provider]
	if !ok {
		return event, errors.New("notification provider is unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	fresh, err := provider.Enrich(callCtx, source, event)
	if err == nil {
		return fresh, nil
	}
	if !errors.Is(err, ErrContextUnavailable) {
		return fresh, err
	}
	event.Context = ""
	event.SubjectState = ""
	event.SubjectTitle = ""
	event.ContextWarnings = []string{"Current subject context is unavailable. Use the raw notification metadata; do not assume the subject is resolved. Ask the user to inspect or clarify when actionability is uncertain."}
	return event, nil
}

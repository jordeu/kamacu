package notifications

import (
	"context"
	"kamacu/internal/store"
)

// Supply associations and session evidence without deciding ownership or work.
func (s *Service) refreshRoutingContext(ctx context.Context, e *Event) error {
	if e.SubjectType == "PullRequest" {
		if repo, n, err := store.PRIdentity(e.URL, 0); err == nil {
			work, err := store.RelatedPRWork(ctx, s.DB, repo, n)
			if err != nil {
				return err
			}
			sessions, err := store.RelatedPRSessions(ctx, s.DB, repo, n)
			if err != nil {
				return err
			}
			e.RelatedWork = work
			e.RelatedSessions = sessions
		}
	}
	return nil
}

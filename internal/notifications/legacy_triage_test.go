package notifications

import (
	"context"
	"kamacu/internal/settings"
)

// Single-subject property fixtures exercise the production project pipeline,
// rather than keeping a second copy of the retired classification logic.
func (s *Service) classifySubject(ctx context.Context, subject *triageSubject, custom string) error {
	return s.classifyProject(ctx, []*triageSubject{subject}, settings.NotificationPolicy{}, custom)
}
func (s *Service) draftTriageCards(ctx context.Context, decision *BatchDecision, subjects []*triageSubject, custom string) error {
	return s.draftProjectCards(ctx, decision, subjects, settings.NotificationPolicy{}, custom)
}

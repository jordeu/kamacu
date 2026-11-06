package notifications

import (
	"context"
)

// Single-subject property fixtures exercise the production project pipeline,
// rather than keeping a second copy of the retired classification logic.
func (s *Service) classifySubject(ctx context.Context, subject *triageSubject, custom string) error {
	return s.classifyProject(ctx, []*triageSubject{subject}, RuleSet{}, custom)
}
func (s *Service) draftTriageCards(ctx context.Context, decision *BatchDecision, subjects []*triageSubject, custom string) error {
	return s.draftProjectCards(ctx, decision, subjects, RuleSet{}, custom)
}

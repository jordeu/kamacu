package notifications

import (
	"context"
	"kamacu/internal/store"
	"slices"
	"time"
)

// Recognize the previously compiled condition so active rules benefit without
// being rewritten or reactivated. New compilations use the factual equivalent.
const sessionRoutingCondition = "the action required by this notification needs a new agent session rather than continuing an existing session"

func sessionRoutingFact(e Event) RuleFact {
	if e.SessionRouting != nil {
		return *e.SessionRouting
	}
	return uncachedSessionRoutingFact(e)
}

func uncachedSessionRoutingFact(e Event) RuleFact {
	unknown := RuleFact{Reason: "implementation ownership or session lifecycle is incomplete"}
	if e.WorkContextTruncated {
		return unknown
	}
	owners := []int64{}
	for _, work := range e.RelatedWork {
		if work.Link.Role == "implementation" && !slices.Contains(owners, work.Link.TaskID) {
			owners = append(owners, work.Link.TaskID)
		}
	}
	if len(owners) > 1 {
		return unknown
	}
	if len(owners) == 0 {
		return RuleFact{Known: true, Value: true, Reason: "no linked implementation owner; a new investigation needs a session"}
	}
	sessions := append([]store.SessionWork(nil), e.RelatedSessions...)
	for _, work := range e.RelatedWork {
		sessions = append(sessions, work.Sessions...)
	}
	for _, w := range sessions {
		if w.TaskID == nil || *w.TaskID != owners[0] {
			continue
		}
		if w.State == "exited" {
			continue
		}
		if w.StateStale || w.State == "unknown" || w.State == "" {
			continue
		}
		return RuleFact{Known: true, Value: false, Evidence: []string{w.SessionID}, Reason: "linked implementation owner has a current session"}
	}
	return unknown
}

// Resolve negative session availability from the process manager, not a bounded
// history query. Refresh local ownership/session data before publication even
// when the GitHub subject has not changed.
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
	e.WorkContextTruncated = false
	e.SessionRouting = nil
	routing := s.routingForRole(*e, "implementation")
	review := s.routingForRole(*e, "review")
	routing.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	review.ObservedAt = routing.ObservedAt
	e.SessionRouting = &routing
	e.ReviewSessionRouting = &review
	compactWorkDecoration(e, workDecorationLimit)
	return nil
}

func (s *Service) routingForRole(e Event, role string) RuleFact {
	work := []store.RelatedTask{}
	owners := []int64{}
	for _, item := range e.RelatedWork {
		if item.Link.Role == role {
			if !slices.Contains(owners, item.Link.TaskID) {
				owners = append(owners, item.Link.TaskID)
			}
			item.Link.Role = "implementation"
			work = append(work, item)
		}
	}
	e.RelatedWork = work
	if len(owners) == 1 && s.TaskHasSession != nil {
		return RuleFact{Known: true, Value: !s.TaskHasSession(owners[0]), Reason: "verified live " + role + "-task session availability"}
	}
	return uncachedSessionRoutingFact(e)
}

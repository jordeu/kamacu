package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"kamacu/internal/store"
)

// enrichSubject lets declared unavailable resources remain visible to triage,
// while authentication, identity, rate-limit and transport failures still abort.
// GitHub can retain verified subject state when only an optional comment is gone.
func (s *Service) enrichSubject(ctx context.Context, source Source, event Event) (Event, error) {
	ctx = withGitHubReadCache(ctx)
	provider, ok := s.Providers[source.Provider]
	if !ok {
		return event, errors.New("notification provider is unavailable")
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	fresh, err := provider.Enrich(callCtx, source, event)
	if err == nil {
		if event.SubjectType == "PullRequest" {
			if repo, n, e := store.PRIdentity(event.URL, 0); e == nil {
				if reader, ok := provider.(interface {
					PRContext(context.Context, Source, string, int64) PRFacts
				}); ok {
					facts := reader.PRContext(callCtx, source, repo, n)
					fresh.PRFacts = &facts
				}
			}
		}
		if err := s.refreshRoutingContext(ctx, &fresh); err != nil {
			return fresh, err
		}
		return fresh, nil
	}
	if !errors.Is(err, ErrContextUnavailable) {
		return fresh, err
	}
	event.PRFacts = nil
	event.Evidence = nil
	event.EvidenceComplete = false
	event.MentionScanComplete = false
	event.EvidenceObservedAt = time.Time{}
	event.RelatedWork = nil
	event.RelatedSessions = nil
	event.Context = ""
	event.SubjectState = ""
	event.SubjectTitle = ""
	event.SubjectUpdatedAt = ""
	event.ContextWarnings = []string{"Current source context could not be retrieved; actionability is not established by the raw notification reason alone."}
	return event, nil
}

// Keep only enough work context to choose the next read-only lookup. Full
// evidence stays in MCP responses; never duplicate transcripts in notification
// decoration. Budget the serialized JSON, including control-character escaping.
const workDecorationLimit = 8000

func compactWorkDecoration(event *Event, limit int) {
	compactFactDetails(event, limit)
	if len(event.RelatedWork) == 0 && len(event.RelatedSessions) == 0 {
		return
	}
	event.WorkContextLookup = "Use get_pr_context for all related work and get_task_pr_context/get_session_output for full session evidence."
	trim := func(text string, n int) string {
		if len(text) <= n {
			return text
		}
		event.WorkContextTruncated = true
		return strings.ToValidUTF8(text[:n], "") + "…"
	}
	compactSession := func(w store.SessionWork) store.SessionWork {
		w.Summary = trim(w.Summary, 400)
		w.Uncertainty = trim(w.Uncertainty, 200)
		if w.Evidence != "" {
			event.WorkContextTruncated = true
		}
		w.Evidence = ""
		return w
	}
	seen := map[string]bool{}
	for i := range event.RelatedWork {
		t := &event.RelatedWork[i]
		t.Title = trim(t.Title, 200)
		if t.Link.Evidence != "" {
			event.WorkContextTruncated = true
		}
		t.Link.Evidence = ""
		t.Link.Uncertainty = trim(t.Link.Uncertainty, 200)
		// Sessions are represented once in the flat collection, retaining task_id.
		for _, w := range t.Sessions {
			if !seen[w.SessionID] {
				seen[w.SessionID] = true
				event.RelatedSessions = append(event.RelatedSessions, compactSession(w))
			}
		}
		t.Sessions = nil
	}
	sessions := []store.SessionWork{}
	seen = map[string]bool{}
	for _, w := range event.RelatedSessions {
		if !seen[w.SessionID] {
			seen[w.SessionID] = true
			sessions = append(sessions, compactSession(w))
		}
	}
	event.RelatedSessions = sessions
	// Bound total decoration independently of comment/prose limits. Trim oldest
	// session observations first, then task rows, with an explicit lookup marker.
	size := func() int {
		b, _ := json.Marshal(struct {
			Tasks    []store.RelatedTask `json:"tasks"`
			Sessions []store.SessionWork `json:"sessions"`
		}{event.RelatedWork, event.RelatedSessions})
		return len(b)
	}
	// Put recent state observations first, regardless of task/session input order.
	sort.SliceStable(event.RelatedSessions, func(i, j int) bool {
		return event.RelatedSessions[i].StateObservedAt.After(event.RelatedSessions[j].StateObservedAt)
	})
	for size() > limit {
		event.WorkContextTruncated = true
		if len(event.RelatedSessions) > 0 {
			event.RelatedSessions = event.RelatedSessions[:len(event.RelatedSessions)-max(1, len(event.RelatedSessions)/2)]
		} else if len(event.RelatedWork) > 0 {
			event.RelatedWork = event.RelatedWork[:len(event.RelatedWork)-max(1, len(event.RelatedWork)/2)]
		} else {
			break
		}
	}
}

// The full paginated history remains available through get_pr_context. Optional
// detail arrays share the decoration budget; verified identity/status and the
// aggregate approval/completeness facts are never truncated.
func compactFactDetails(event *Event, limit int) {
	if event.PRFacts == nil {
		return
	}
	f := *event.PRFacts
	f.ViewerReviews = append([]ViewerReview(nil), f.ViewerReviews...)
	f.RecentCommits = append([]PRCommit(nil), f.RecentCommits...)
	if f.ViewerReviewCount == 0 {
		f.ViewerReviewCount = len(f.ViewerReviews)
	}
	if len(f.ViewerReviews) > 6 {
		history := f.ViewerReviews
		f.ViewerReviews = append([]ViewerReview(nil), history[len(history)-5:]...)
		// Keep an older positive review as direct evidence, independent of head.
		hasApproval := false
		for _, r := range f.ViewerReviews {
			hasApproval = hasApproval || r.State == "APPROVED"
		}
		if !hasApproval {
			for _, r := range history[:len(history)-5] {
				if r.State == "APPROVED" {
					f.ViewerReviews = append([]ViewerReview{r}, f.ViewerReviews...)
					break
				}
			}
		}
		f.ViewerReviewsTruncated = true
	}
	size := func() int {
		b, _ := json.Marshal(struct {
			Reviews []ViewerReview
			Commits []PRCommit
		}{f.ViewerReviews, f.RecentCommits})
		return len(b)
	}
	for size() > limit {
		if len(f.RecentCommits) > 0 {
			f.RecentCommits = f.RecentCommits[1:]
			f.RecentCommitsTruncated = true
		} else if len(f.ViewerReviews) > 0 {
			if len(f.ViewerReviews) > 2 {
				f.ViewerReviews = append(f.ViewerReviews[:1], f.ViewerReviews[2:]...)
			} else {
				f.ViewerReviews = f.ViewerReviews[1:]
			}
			f.ViewerReviewsTruncated = true
		} else {
			break
		}
	}
	event.PRFacts = &f
}

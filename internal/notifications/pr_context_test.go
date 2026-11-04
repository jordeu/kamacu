package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"kamacu/internal/store"
)

func TestPRContextRetainsEarlierApprovalAcrossPagesAndHeads(t *testing.T) {
	offline := false
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		p := args[len(args)-1]
		switch p {
		case "user":
			return response(`{"login":"alice"}`, ""), nil
		case "repos/acme/repo/pulls/7":
			return response(`{"state":"open","draft":false,"head":{"sha":"new-head"},"requested_reviewers":[{"login":"bob"}],"updated_at":"2026-11-03T12:00:00Z"}`, ""), nil
		case "repos/acme/repo/pulls/7/commits?per_page=10":
			return response(`[{"sha":"new-head","html_url":"https://github.com/acme/repo/commit/new-head","commit":{"message":"Fix race\n\nLong body","committer":{"date":"2026-11-03T12:00:00Z"}}}]`, ""), nil
		case "repos/acme/repo/pulls/7/reviews?per_page=100":
			return response(`[{"id":1,"user":{"login":"Alice"},"state":"APPROVED","commit_id":"old-head","submitted_at":"2026-11-01T10:00:00Z"}]`, `Link: <https://api.github.com/repos/acme/repo/pulls/7/reviews?per_page=100&page=2>; rel="next"`+"\r\n"), nil
		default:
			if offline {
				return nil, errors.New("offline")
			}
			return response(`[{"id":2,"user":{"login":"bob"},"state":"COMMENTED"}]`, ""), nil
		}
	}}
	facts := g.PRContext(context.Background(), Source{Account: "alice"}, "acme/repo", 7)
	if !facts.Available || facts.Stale || !facts.ReviewsComplete || facts.ViewerEverApproved == nil || !*facts.ViewerEverApproved || len(facts.ViewerReviews) != 1 || facts.HeadCommit != "new-head" || facts.ViewerReviews[0].CommitID != "old-head" || len(facts.RecentCommits) != 1 || facts.RecentCommits[0].Commit.Message != "Fix race" {
		t.Fatalf("lost evidence: %+v", facts)
	}
	offline = true
	facts = g.PRContext(context.Background(), Source{Account: "alice"}, "acme/repo", 7)
	if facts.ReviewsComplete || facts.Error == "" || facts.ViewerEverApproved == nil || !*facts.ViewerEverApproved {
		t.Fatalf("partial history lost positive evidence: %+v", facts)
	}
}
func TestPRContextUnknownIsNotNoApproval(t *testing.T) {
	g := GitHub{Run: func(_ context.Context, args ...string) ([]byte, error) {
		p := args[len(args)-1]
		if p == "user" {
			return response(`{"login":"alice"}`, ""), nil
		}
		if strings.Contains(p, "reviews?") {
			return nil, errors.New("offline")
		}
		return response(`{"state":"closed","merged":true,"draft":false,"head":{"sha":"head"}}`, ""), nil
	}}
	f := g.PRContext(context.Background(), Source{Account: "alice"}, "acme/repo", 7)
	if f.State != "merged" || f.ViewerEverApproved != nil || f.ReviewsComplete || f.Error == "" {
		t.Fatalf("unknown treated as no approval: %+v", f)
	}
	f = g.PRContext(context.Background(), Source{Account: "bob"}, "acme/repo", 7)
	if f.Available || !f.Stale || f.ViewerEverApproved != nil {
		t.Fatalf("identity failure: %+v", f)
	}
}
func TestNotificationDecorationPreservesWorkOutsideProse(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "implementation", Source: "explicit", Evidence: "Implementing", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.enrichSubject(ctx, source, event())
	if err != nil {
		t.Fatal(err)
	}
	fresh.Context = "[truncated]"
	data, _ := json.Marshal(fresh)
	if len(fresh.RelatedWork) != 1 || !strings.Contains(string(data), `"role":"implementation"`) {
		t.Fatalf("lost structured association: %s", data)
	}
	if mutation("get_pr_context") || mutation("get_task_pr_context") || !mutation("set_task_pr_link") {
		t.Fatal("incorrect triage tool permissions")
	}
}

type contextFactsProvider struct{ *fakeProvider }

func (p contextFactsProvider) Enrich(_ context.Context, _ Source, e Event) (Event, error) {
	e.Context = strings.Repeat("Large comment body. ", 4000)
	return e, nil
}
func (p contextFactsProvider) PRContext(_ context.Context, _ Source, _ string, _ int64) PRFacts {
	yes := true
	return PRFacts{Source: "github_api", Available: true, State: "open", HeadCommit: "new-head", Viewer: "alice", ReviewsComplete: true, ViewerEverApproved: &yes, ViewerReviews: []ViewerReview{{State: "APPROVED", CommitID: "old-head"}}}
}
func TestNotificationModelConsumesFactsAfterExcerptTruncation(t *testing.T) {
	s, model, _, source := fixture(t)
	s.Providers["github"] = contextFactsProvider{&fakeProvider{}}
	id := seed(t, s, source, event())
	result := process(t, s, model, BatchDecision{DiscardIDs: []int64{id}})
	if result.Status != "completed" || len(model.prompts) != 1 {
		t.Fatalf("processing failed: %+v", result)
	}
	prompt := model.prompts[0]
	for _, fact := range []string{`"pr_facts":`, `"viewer_ever_approved":true`, `"viewer":"alice"`, `"commit_id":"old-head"`, "Excerpt truncated"} {
		if !strings.Contains(prompt, fact) {
			t.Fatalf("missing %s after truncation", fact)
		}
	}
}

func TestWorkDecorationBudgetsAndDeduplicatesSessionEvidence(t *testing.T) {
	event := Event{PRFacts: &PRFacts{Viewer: "alice", State: "open", HeadCommit: "head", ReviewsComplete: true}}
	approved := true
	event.PRFacts.ViewerEverApproved = &approved
	for i := 0; i < 40; i++ {
		id := int64(i + 1)
		w := store.SessionWork{SessionID: fmt.Sprint(i), TaskID: &id, State: "working", Summary: strings.Repeat("\\\"\n", 1200), Evidence: strings.Repeat("evidence", 1000), Uncertainty: strings.Repeat("uncertainty", 150), StateObservedAt: time.Now().Add(time.Duration(i) * time.Second)}
		event.RelatedWork = append(event.RelatedWork, store.RelatedTask{Link: store.PRLink{TaskID: id, Repository: "acme/repo", Number: 7, Role: "review", Evidence: strings.Repeat("e", 8000)}, Sessions: []store.SessionWork{w}})
		event.RelatedSessions = append(event.RelatedSessions, w)
	}
	for _, limit := range []int{workDecorationLimit, 1000, 32} {
		compactWorkDecoration(&event, limit)
		b, _ := json.Marshal(struct {
			Tasks    []store.RelatedTask `json:"tasks"`
			Sessions []store.SessionWork `json:"sessions"`
		}{event.RelatedWork, event.RelatedSessions})
		if len(b) > limit {
			t.Fatalf("serialized decoration %d exceeds %d", len(b), limit)
		}
		if !event.WorkContextTruncated || event.WorkContextLookup == "" {
			t.Fatal("missing deeper lookup for omitted evidence")
		}
		seen := map[string]bool{}
		for _, w := range event.RelatedSessions {
			if seen[w.SessionID] || w.Evidence != "" {
				t.Fatal("duplicated session/transcript in decoration")
			}
			seen[w.SessionID] = true
		}
		for _, w := range event.RelatedWork {
			if len(w.Sessions) > 0 || w.Link.Evidence != "" {
				t.Fatal("full evidence leaked into decoration")
			}
		}
		if event.PRFacts.Viewer != "alice" || event.PRFacts.ViewerEverApproved == nil || !*event.PRFacts.ViewerEverApproved {
			t.Fatal("compaction removed essential approval facts")
		}
	}
}

func TestNotificationProcessingBoundsManySessionsAndKeepsLookupFacts(t *testing.T) {
	s, model, _, source := fixture(t)
	ctx := context.Background()
	s.Providers["github"] = contextFactsProvider{&fakeProvider{}}
	if err := store.PutPRLink(ctx, s.DB, store.PRLink{TaskID: 1, Repository: "acme/repo", Number: 7, Role: "review", Source: "explicit", Evidence: "Review task", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	idTask := int64(1)
	for i := 0; i < 20; i++ {
		if err := store.PutSessionWork(ctx, s.DB, store.SessionWork{SessionID: fmt.Sprint(i), TaskID: &idTask, State: "idle", Summary: strings.Repeat("work ", 700), Evidence: strings.Repeat("TRANSCRIPT_SENTINEL ", 400), ObservedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	id := seed(t, s, source, event())
	result := process(t, s, model, BatchDecision{DiscardIDs: []int64{id}})
	if result.Status != "completed" || len(model.prompts) != 1 {
		t.Fatalf("work decoration prevented processing: %+v", result)
	}
	prompt := model.prompts[0]
	if strings.Contains(prompt, "TRANSCRIPT_SENTINEL") {
		t.Fatal("full session transcripts leaked into notification prompt")
	}
	_, snapshot, ok := strings.Cut(prompt, "\nSNAPSHOT (data):\n")
	if !ok {
		t.Fatal("missing input snapshot")
	}
	var payload struct {
		Subjects map[string]Event `json:"subject_context"`
	}
	if err := json.Unmarshal([]byte(snapshot), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Subjects) != 1 {
		t.Fatal("missing subject decoration")
	}
	for _, e := range payload.Subjects {
		if !e.WorkContextTruncated || e.WorkContextLookup == "" || e.PRFacts == nil || e.PRFacts.ViewerEverApproved == nil || !*e.PRFacts.ViewerEverApproved {
			t.Fatalf("lost essential facts/lookup: %+v", e)
		}
		if len(e.RelatedWork) != 1 || e.RelatedWork[0].Link.TaskID != 1 || e.RelatedWork[0].Link.Role != "review" {
			t.Fatal("lost task association", e.RelatedWork)
		}
		b, _ := json.Marshal(struct {
			Tasks    []store.RelatedTask `json:"tasks"`
			Sessions []store.SessionWork `json:"sessions"`
		}{e.RelatedWork, e.RelatedSessions})
		if len(b) > workDecorationLimit {
			t.Fatalf("work payload too large: %d", len(b))
		}
	}
}

func TestNotificationBoundsReviewDetailsWithoutLosingApproval(t *testing.T) {
	yes := true
	facts := &PRFacts{State: "open", Viewer: "alice", ViewerEverApproved: &yes, ReviewsComplete: true, HeadCommit: "new"}
	for i := 0; i < 1000; i++ {
		state := "COMMENTED"
		if i == 0 {
			state = "APPROVED"
		}
		facts.ViewerReviews = append(facts.ViewerReviews, ViewerReview{ID: int64(i), State: state, CommitID: "old-head", URL: strings.Repeat("url", 100)})
	}
	event := Event{PRFacts: facts}
	compactWorkDecoration(&event, workDecorationLimit)
	if len(facts.ViewerReviews) != 1000 {
		t.Fatal("notification compaction changed full MCP history")
	}
	if !event.PRFacts.ViewerReviewsTruncated || event.PRFacts.ViewerReviewCount != 1000 || !event.PRFacts.ReviewsComplete || !*event.PRFacts.ViewerEverApproved || event.PRFacts.ViewerReviews[0].State != "APPROVED" {
		t.Fatal("lost earlier approval or completeness semantics", event.PRFacts)
	}
	compactWorkDecoration(&event, 32)
	if event.PRFacts.Viewer != "alice" || !*event.PRFacts.ViewerEverApproved || event.PRFacts.HeadCommit != "new" || len(event.PRFacts.ViewerReviews) != 0 {
		t.Fatal("essential facts lost when detail budget exhausted", event.PRFacts)
	}
}

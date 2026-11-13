package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func statusFixture(t *testing.T) statusPRRaw {
	t.Helper()
	var p statusPRRaw
	err := json.Unmarshal([]byte(`{"state":"OPEN","headRefOid":"head","author":{"login":"author","__typename":"User"},"commits":{"nodes":[{"commit":{"oid":"head","statusCheckRollup":{"state":"SUCCESS","contexts":{"nodes":[{"__typename":"CheckRun","name":"Tests","status":"COMPLETED","conclusion":"SUCCESS"}]}}}}]}}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func statusReviewFor(login, kind, state, body, head string, at int) statusReview {
	r := statusReview{Author: statusActor{Login: login, Kind: kind}, State: state, Body: body, SubmittedAt: fmt.Sprintf("2026-11-13T10:%02d:00Z", at)}
	r.Commit.OID = head
	return r
}
func statusComplete() map[string]bool {
	return map[string]bool{"Reviews": true, "Requests": true, "Threads": true, "Comments": true, "Checks": true}
}
func TestPRStatusHumanDecisions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		reviews   []statusReview
		requested bool
		want      string
	}{
		{"current approval", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1)}, false, "approved"},
		{"old approval", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "old", 1)}, false, "waiting"},
		{"re-request", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1)}, true, "waiting"},
		{"dismissed", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("alice", "User", "DISMISSED", "", "head", 2)}, false, "waiting"},
		{"comment does not erase approval", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("alice", "User", "COMMENTED", "Looks fine", "head", 2)}, false, "approved"},
		{"comment does not erase changes", []statusReview{statusReviewFor("alice", "User", "CHANGES_REQUESTED", "", "old", 1), statusReviewFor("alice", "User", "COMMENTED", "Thanks", "head", 2)}, false, "changes"},
		{"comment only", []statusReview{statusReviewFor("alice", "User", "COMMENTED", "Looks fine", "head", 1)}, false, "none"},
		{"uninvolved commenter is not an outstanding request", []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("bob", "User", "COMMENTED", "Thanks", "head", 2)}, false, "approved"},
		{"bot cannot approve for humans", []statusReview{statusReviewFor("robot", "Bot", "APPROVED", "", "head", 1)}, false, "none"},
		{"author cannot approve their own work", []statusReview{statusReviewFor("author", "User", "APPROVED", "", "head", 1)}, false, "none"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := statusFixture(t)
			p.Reviews.Nodes = tt.reviews
			if tt.requested {
				p.Requests.Nodes = []statusRequest{{Reviewer: statusActor{Login: "alice", Kind: "User"}}}
			}
			got := reducePRStatus(p, statusComplete())
			if got.Humans.State != tt.want {
				t.Fatalf("humans=%+v want %s", got.Humans, tt.want)
			}
		})
	}
	p := statusFixture(t)
	p.Requests.Nodes = []statusRequest{{Reviewer: statusActor{Kind: "Team", Slug: "owners"}}}
	got := reducePRStatus(p, statusComplete())
	if got.Humans.State != "waiting" || got.Humans.Reviewers[0].Login != "team:owners" {
		t.Fatal(got.Humans)
	}
}
func TestPRStatusBotOutcomes(t *testing.T) {
	for _, tt := range []struct{ name, state, body, head, want string }{
		{"comment-only clean", "COMMENTED", "No new findings", "head", "clean"},
		{"markdown clean", "COMMENTED", "## No new findings\n\nCommit verified", "head", "clean"},
		{"other comments are feedback", "COMMENTED", "Please check this condition", "head", "comments"},
		{"old clean result", "COMMENTED", "No new findings", "old", "stale"},
		{"negation is not clean", "COMMENTED", "Not true: no new findings", "head", "comments"},
		{"formal change request remains explicit", "CHANGES_REQUESTED", "No new findings", "head", "changes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := statusFixture(t)
			p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("robot", "Bot", tt.state, tt.body, tt.head, 2)}
			out := reducePRStatus(p, statusComplete())
			if out.Humans.State != "approved" || out.Bots.State != tt.want {
				t.Fatalf("humans=%+v bots=%+v", out.Humans, out.Bots)
			}
		})
	}
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("robot", "Bot", "COMMENTED", "No new findings", "head", 2)}
	var thread statusThread
	if err := json.Unmarshal([]byte(`{"isResolved":false,"isOutdated":true,"comments":{"nodes":[{"author":{"login":"robot","__typename":"Bot"},"commit":{"oid":"old"}}]}}`), &thread); err != nil {
		t.Fatal(err)
	}
	p.Threads.Nodes = []statusThread{thread}
	out := reducePRStatus(p, statusComplete())
	if out.Bots.State != "comments" || out.Bots.Reviewers[0].Findings != 1 {
		t.Fatal("old unresolved finding was hidden by a clean run", out.Bots)
	}
	p.Threads.Nodes[0].IsResolved = true
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "clean" {
		t.Fatal(out.Bots)
	}
	p.Threads.Nodes = nil
	p.Comments.Nodes = []statusComment{{Author: statusActor{Login: "robot", Kind: "Bot"}, Body: "No new findings", CreatedAt: "2026-11-13T11:00:00Z"}}
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "unknown" {
		t.Fatal("untied issue comment cannot imply current clean result", out.Bots)
	}
	p.Comments.Nodes[0].Body = "No new findings\n\nhead"
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "clean" {
		t.Fatal(out.Bots)
	}
	complete := statusComplete()
	complete["Threads"] = false
	out = reducePRStatus(p, complete)
	if out.Bots.State != "unknown" || out.Humans.State != "none" {
		t.Fatal("partial bot history was treated as clean", out)
	}
}
func TestPRStatusChecksAndIncompleteData(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1)}
	for _, tt := range []struct{ raw, want string }{{"SUCCESS", "pass"}, {"PENDING", "pending"}, {"FAILURE", "fail"}, {"ERROR", "fail"}, {"EXPECTED", "pending"}, {"NEW_UNKNOWN_STATE", "unknown"}} {
		p.Commits.Nodes[0].Commit.Rollup.State = tt.raw
		got := reducePRStatus(p, statusComplete())
		if got.CI.State != tt.want {
			t.Fatal(tt, got.CI)
		}
	}
	p.Commits.Nodes[0].Commit.Rollup = nil
	got := reducePRStatus(p, statusComplete())
	if got.CI.State != "none" {
		t.Fatal(got.CI)
	}
	complete := statusComplete()
	complete["Reviews"] = false
	got = reducePRStatus(p, complete)
	if got.Humans.State != "unknown" {
		t.Fatal("incomplete human approval history", got)
	}
	complete = statusComplete()
	complete["Checks"] = false
	got = reducePRStatus(p, complete)
	if got.CI.State != "unknown" {
		t.Fatal(got.CI)
	}
}
func statusEnvelope(t *testing.T, p statusPRRaw) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": p}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestPRStatusPagination(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Page = statusPage{HasNext: true, Cursor: "page-2"}
	p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1)}
	// Struct fields without custom tags still decode case-insensitively, while
	// wire query field names use GitHub's exact spellings.
	first := statusEnvelope(t, p)
	second := statusFixture(t)
	second.Reviews.Nodes = []statusReview{statusReviewFor("bob", "User", "CHANGES_REQUESTED", "", "head", 2)}
	calls := 0
	fetch := StatusFetcher{Query: func(_ context.Context, v map[string]any) ([]byte, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		if v["reviewsCursor"] != "page-2" || v["readThreads"] != false {
			t.Fatalf("bad pagination variables: %v", v)
		}
		return statusEnvelope(t, second), nil
	}}
	out, err := fetch.Fetch(t.Context(), "Acme/Repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || out.Humans.State != "changes" || len(out.Humans.Reviewers) != 2 || out.Repository != "acme/repo" {
		t.Fatal(calls, out)
	}
	calls = 0
	second.Head = "changed"
	_, err = fetch.Fetch(t.Context(), "acme/repo", 7)
	if err == nil {
		t.Fatal("head changes must not combine unrelated snapshots")
	}
	fetch.Query = func(context.Context, map[string]any) ([]byte, error) {
		return []byte(`{"errors":[{"message":"denied"}]}`), nil
	}
	if _, err = fetch.Fetch(t.Context(), "acme/repo", 7); err == nil {
		t.Fatal("GraphQL errors treated as no data")
	}
}
func TestPRStatusCacheDedupAndStale(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	fail := false
	svc := NewPRStatusService(func(context.Context, string, int64) (PRStatus, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		if fail {
			return PRStatus{}, errors.New("offline")
		}
		v := UnknownPRStatus("acme/repo", 7, "")
		v.CI.State = "pass"
		v.Humans.State = "approved"
		stamp := now
		v.FetchedAt = &stamp
		return v, nil
	}, func() time.Time { return now })
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); svc.Get(t.Context(), "acme/repo", 7) }()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate concurrent requests", calls.Load())
	}
	now = now.Add(61 * time.Second)
	fail = true
	out := svc.Get(t.Context(), "acme/repo", 7)
	if !out.Stale || out.Error != "offline" || out.Humans.State != "approved" {
		t.Fatal(out)
	}
	svc.Get(t.Context(), "acme/repo", 7)
	if calls.Load() != 2 {
		t.Fatal("failed attempts were not throttled")
	}
}

func TestPRStatusBotChecksUseLatestRunAndStickyComment(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("robot", "Bot", "COMMENTED", "No new findings", "old", 1)}
	var checks []statusCheckRaw
	if err := json.Unmarshal([]byte(`[{"__typename":"CheckRun","name":"Review","databaseId":1,"status":"IN_PROGRESS","startedAt":"2026-11-13T10:00:00Z","checkSuite":{"app":{"slug":"robot"}}},{"__typename":"CheckRun","name":"Review","databaseId":2,"status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-11-13T11:00:00Z","checkSuite":{"app":{"slug":"robot"}}}]`), &checks); err != nil {
		t.Fatal(err)
	}
	p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes = checks
	out := reducePRStatus(p, statusComplete())
	if out.Bots.State == "running" || len(out.CI.Jobs) != 1 {
		t.Fatal("superseded pending run affected current bot outcome", out)
	}
	checks[1].Status = "QUEUED"
	checks[1].StartedAt = ""
	checks[1].Conclusion = ""
	p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes = checks
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "running" {
		t.Fatal("new queued bot run not detected", out.Bots)
	}
	p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes = nil
	p.Comments.Nodes = []statusComment{{Author: statusActor{Login: "robot", Kind: "Bot"}, Body: "No new findings\n\nhead", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2026-11-13T11:00:00Z"}}
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "clean" {
		t.Fatal("updated sticky bot outcome was ignored", out.Bots)
	}
	var thread statusThread
	if err := json.Unmarshal([]byte(`{"comments":{"nodes":[{"author":null}]}}`), &thread); err != nil {
		t.Fatal(err)
	}
	p.Threads.Nodes = []statusThread{thread}
	out = reducePRStatus(p, statusComplete())
	if out.Bots.State != "unknown" {
		t.Fatal("unknown unresolved thread author treated as clear", out.Bots)
	}
}

func TestPRStatusFormalBotApprovalIsNotInventedNoFindings(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("robot", "Bot", "APPROVED", "Approved with non-blocking notes", "head", 1)}
	out := reducePRStatus(p, statusComplete())
	if out.Bots.State != "complete" || out.Bots.Reviewers[0].State != "approved" || out.Humans.State != "none" {
		t.Fatal(out)
	}
	complete := statusComplete()
	complete["Checks"] = false
	p.Commits.Nodes[0].Commit.Rollup.State = "FAILURE"
	out = reducePRStatus(p, complete)
	if out.CI.State != "fail" {
		t.Fatal("known native failure hidden by incomplete details", out.CI)
	}
}

func TestPRStatusPartialBotErrorsDoNotEraseHumanAndCI(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("robot", "Bot", "COMMENTED", "No new findings", "head", 2)}
	var payload map[string]any
	if err := json.Unmarshal(statusEnvelope(t, p), &payload); err != nil {
		t.Fatal(err)
	}
	payload["errors"] = []any{map[string]any{"path": []any{"repository", "pullRequest", "reviewThreads"}, "message": "Unavailable"}}
	raw, _ := json.Marshal(payload)
	fetch := StatusFetcher{Query: func(context.Context, map[string]any) ([]byte, error) { return raw, nil }}
	out, err := fetch.Fetch(t.Context(), "acme/repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if out.Humans.State != "approved" || out.CI.State != "pass" || out.Bots.State != "unknown" {
		t.Fatal(out)
	}
}

func TestPRStatusBotPaginationFailuresKeepCompletedPrimary(t *testing.T) {
	for _, connection := range []string{"comments", "threads"} {
		for _, failure := range []string{"transport", "deadline", "malformed"} {
			t.Run(connection+"/"+failure, func(t *testing.T) {
				p := statusFixture(t)
				p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1), statusReviewFor("robot", "Bot", "COMMENTED", "No new findings", "head", 2)}
				if connection == "comments" {
					p.Comments.Page = statusPage{HasNext: true, Cursor: "more"}
				} else {
					p.Threads.Page = statusPage{HasNext: true, Cursor: "more"}
				}
				first := statusEnvelope(t, p)
				observed := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
				now := observed
				calls := 0
				fetch := StatusFetcher{Now: func() time.Time { return now }, Query: func(_ context.Context, v map[string]any) ([]byte, error) {
					calls++
					if calls == 1 {
						return first, nil
					}
					if v["readReviews"] != false || v["readRequests"] != false || v["readChecks"] != false {
						t.Fatal("primary sections are not complete", v)
					}
					now = observed.Add(time.Second)
					switch failure {
					case "deadline":
						return nil, context.DeadlineExceeded
					case "malformed":
						return []byte("not JSON"), nil
					default:
						return nil, errors.New("offline")
					}
				}}
				out, err := fetch.Fetch(t.Context(), "acme/repo", 7)
				if err != nil {
					t.Fatal(err)
				}
				if calls != 2 || out.Humans.State != "approved" || out.CI.State != "pass" || out.Bots.State != "unknown" || out.Error == "" || out.Stale {
					t.Fatal(calls, out)
				}
				if out.FetchedAt == nil || !out.FetchedAt.Equal(observed) {
					t.Fatal("partial snapshot must retain observation time", out.FetchedAt)
				}
			})
		}
	}
}

func TestPRStatusPaginationFailureDoesNotInventCompleteHumanHistory(t *testing.T) {
	p := statusFixture(t)
	p.Reviews.Nodes = []statusReview{statusReviewFor("alice", "User", "APPROVED", "", "head", 1)}
	p.Reviews.Page = statusPage{HasNext: true, Cursor: "more"}
	first := statusEnvelope(t, p)
	calls := 0
	fetch := StatusFetcher{Query: func(context.Context, map[string]any) ([]byte, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return nil, errors.New("offline")
	}}
	if _, err := fetch.Fetch(t.Context(), "acme/repo", 7); err == nil {
		t.Fatal("incomplete human decisions cannot be treated as verified approval")
	}
}

func TestPRStatusEmptyBotReviewUsesActualCommit(t *testing.T) {
	for _, tt := range []struct{ head, state, body, want string }{{"head", "COMMENTED", "", "complete"}, {"old", "COMMENTED", "", "stale"}, {"head", "DISMISSED", "No new findings", "unknown"}} {
		p := statusFixture(t)
		p.Reviews.Nodes = []statusReview{statusReviewFor("robot", "Bot", tt.state, tt.body, tt.head, 1)}
		out := reducePRStatus(p, statusComplete())
		if out.Bots.State != tt.want {
			t.Fatal(tt, out.Bots)
		}
		if tt.head == "head" && out.Bots.Reviewers[0].State == "stale" {
			t.Fatal("current-head review labeled outdated", out.Bots)
		}
	}
}

func TestPRStatusStartupFailureIsFailedJob(t *testing.T) {
	p := statusFixture(t)
	p.Commits.Nodes[0].Commit.Rollup.State = "FAILURE"
	job := &p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes[0]
	job.Conclusion = "STARTUP_FAILURE"
	out := reducePRStatus(p, statusComplete())
	if out.CI.State != "fail" || len(out.CI.Jobs) != 1 || out.CI.Jobs[0].State != "fail" || out.CI.Jobs[0].Detail != "startup_failure" {
		t.Fatal(out.CI)
	}
}

func TestPRStatusCanceledQueuedReadDoesNotStartFetch(t *testing.T) {
	calls := 0
	svc := NewPRStatusService(func(context.Context, string, int64) (PRStatus, error) {
		calls++
		return PRStatus{}, errors.New("should not run")
	}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out := svc.Get(ctx, "acme/repo", 7)
	if calls != 0 || out.Error == "" {
		t.Fatal(calls, out)
	}
}

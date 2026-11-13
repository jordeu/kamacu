package github

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed pr_status.graphql
var statusQuery string

type StatusReviewer struct {
	Login    string `json:"login"`
	State    string `json:"state"`
	Detail   string `json:"detail,omitempty"`
	Findings int    `json:"findings,omitempty"`
}
type StatusGroup struct {
	State     string           `json:"state"`
	Reviewers []StatusReviewer `json:"reviewers"`
}
type StatusCheck struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}
type StatusChecks struct {
	State string        `json:"state"`
	Jobs  []StatusCheck `json:"jobs"`
}

// PRStatus is a read-only snapshot. Bots never participate in the primary
// human/CI result, and comment-only outcomes are never human approvals.
type PRStatus struct {
	Repository string       `json:"repository"`
	Number     int64        `json:"number"`
	Lifecycle  string       `json:"lifecycle"`
	Head       string       `json:"head"`
	Humans     StatusGroup  `json:"humans"`
	Bots       StatusGroup  `json:"bots"`
	CI         StatusChecks `json:"ci"`
	FetchedAt  *time.Time   `json:"fetched_at"`
	Stale      bool         `json:"stale"`
	Error      string       `json:"error,omitempty"`
}

func UnknownPRStatus(repo string, number int64, reason string) PRStatus {
	return PRStatus{Repository: repo, Number: number, Lifecycle: "unknown", Humans: StatusGroup{"unknown", []StatusReviewer{}}, Bots: StatusGroup{"unknown", []StatusReviewer{}}, CI: StatusChecks{"unknown", []StatusCheck{}}, Error: reason}
}

type statusActor struct {
	Login string `json:"login"`
	Kind  string `json:"__typename"`
	Slug  string `json:"slug"`
}
type statusPage struct {
	HasNext bool   `json:"hasNextPage"`
	Cursor  string `json:"endCursor"`
}
type statusConnection[T any] struct {
	Nodes []T        `json:"nodes"`
	Page  statusPage `json:"pageInfo"`
}
type statusReview struct {
	Author                              statusActor
	State, Body, SubmittedAt, CreatedAt string
	Commit                              struct{ OID string }
}
type statusComment struct {
	Author                     statusActor
	Body, CreatedAt, UpdatedAt string
}

func (c statusComment) observedAt() string {
	if c.UpdatedAt != "" {
		return c.UpdatedAt
	}
	return c.CreatedAt
}

type statusThread struct {
	IsResolved bool
	Comments   struct {
		Nodes []struct {
			Author statusActor
		}
	}
}
type statusRequest struct {
	Reviewer statusActor `json:"requestedReviewer"`
}
type statusCheckRaw struct {
	checkEntry
	DatabaseID int64 `json:"databaseId"`
	CheckSuite struct{ App struct{ Slug string } }
}
type statusRollup struct {
	State    string
	Contexts statusConnection[statusCheckRaw]
}
type statusCommit struct {
	OID    string
	Rollup *statusRollup `json:"statusCheckRollup"`
}
type statusCommitNode struct{ Commit statusCommit }
type statusPRRaw struct {
	State    string
	IsDraft  bool
	Head     string `json:"headRefOid"`
	Author   statusActor
	Reviews  statusConnection[statusReview]
	Requests statusConnection[statusRequest] `json:"reviewRequests"`
	Threads  statusConnection[statusThread]  `json:"reviewThreads"`
	Comments statusConnection[statusComment]
	Commits  statusConnection[statusCommitNode]
}

func statusErrorSection(path []any) string {
	for _, part := range path {
		switch part {
		case "reviews":
			return "Reviews"
		case "reviewRequests":
			return "Requests"
		case "reviewThreads":
			return "Threads"
		case "comments":
			return "Comments"
		case "commits":
			return "Checks"
		}
	}
	return ""
}

// StatusFetcher exposes only a JSON transport seam; pagination and all evidence
// reduction remain shared between production and tests.
type StatusFetcher struct {
	Query func(context.Context, map[string]any) ([]byte, error)
	Now   func() time.Time
}

func queryStatus(ctx context.Context, variables map[string]any) ([]byte, error) {
	body, err := json.Marshal(map[string]any{"query": statusQuery, "variables": variables})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil && !json.Valid(out) {
		return nil, fmt.Errorf("GitHub PR status lookup failed: %w", err)
	}
	return out, nil
}
func (f StatusFetcher) Fetch(ctx context.Context, repo string, number int64) (PRStatus, error) {
	canonical, err := ParseRepoRef(repo)
	if err != nil || number <= 0 {
		return PRStatus{}, errors.New("invalid PR identity")
	}
	parts := strings.Split(canonical, "/")
	vars := map[string]any{"owner": parts[0], "name": parts[1], "number": number, "readReviews": true, "readThreads": true, "readRequests": true, "readComments": true, "readChecks": true}
	query := f.Query
	if query == nil {
		query = queryStatus
	}
	now := f.Now
	if now == nil {
		now = time.Now
	}
	var all statusPRRaw
	done := map[string]bool{}
	failed := map[string]bool{}
	var firstObserved time.Time
	partial := false
	primaryComplete := func() bool {
		return done["Reviews"] && done["Requests"] && done["Checks"] && !failed["Reviews"] && !failed["Requests"] && !failed["Checks"]
	}
	for page := 0; page < 10; page++ {
		raw, err := query(ctx, vars)
		if err != nil {
			if primaryComplete() {
				partial = true
				break
			}
			return PRStatus{}, err
		}
		var envelope struct {
			Data struct {
				Repository *struct {
					PR *statusPRRaw `json:"pullRequest"`
				}
			}
			Errors []struct{ Path []any }
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Data.Repository == nil || envelope.Data.Repository.PR == nil {
			if primaryComplete() {
				partial = true
				break
			}
			return PRStatus{}, errors.New("GitHub PR status data unavailable")
		}
		p := envelope.Data.Repository.PR
		for _, failure := range envelope.Errors {
			section := statusErrorSection(failure.Path)
			if section == "" {
				return PRStatus{}, errors.New("GitHub PR status data unavailable")
			}
			failed[section] = true
			done[section] = true
			vars["read"+section] = false
		}
		if p.Head == "" || p.State == "" {
			return PRStatus{}, errors.New("invalid GitHub PR status data")
		}
		if page == 0 {
			all.State = p.State
			all.IsDraft = p.IsDraft
			all.Head = p.Head
			all.Author = p.Author
			firstObserved = now().UTC()
		} else if all.Head != p.Head {
			return PRStatus{}, errors.New("PR head changed during status lookup")
		}
		all.Reviews.Nodes = append(all.Reviews.Nodes, p.Reviews.Nodes...)
		all.Threads.Nodes = append(all.Threads.Nodes, p.Threads.Nodes...)
		all.Requests.Nodes = append(all.Requests.Nodes, p.Requests.Nodes...)
		all.Comments.Nodes = append(all.Comments.Nodes, p.Comments.Nodes...)
		pages := map[string]statusPage{"Reviews": p.Reviews.Page, "Threads": p.Threads.Page, "Requests": p.Requests.Page, "Comments": p.Comments.Page}
		if len(p.Commits.Nodes) == 0 && failed["Checks"] {
			p.Commits.Nodes = []statusCommitNode{{Commit: statusCommit{OID: all.Head}}}
		}
		if len(p.Commits.Nodes) != 1 || p.Commits.Nodes[0].Commit.OID != all.Head {
			return PRStatus{}, errors.New("PR checks do not match current head")
		}
		if page == 0 {
			all.Commits = p.Commits
		} else if p.Commits.Nodes[0].Commit.Rollup != nil && all.Commits.Nodes[0].Commit.Rollup != nil {
			all.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes = append(all.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes, p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes...)
		}
		rollup := p.Commits.Nodes[0].Commit.Rollup
		if rollup != nil {
			pages["Checks"] = rollup.Contexts.Page
		} else {
			pages["Checks"] = statusPage{}
		}
		complete := true
		for name, pi := range pages {
			if done[name] {
				continue
			}
			if !pi.HasNext {
				done[name] = true
				vars["read"+name] = false
			} else {
				if pi.Cursor == "" || vars[strings.ToLower(name[:1])+name[1:]+"Cursor"] == pi.Cursor {
					return PRStatus{}, errors.New("invalid PR status pagination")
				}
				vars[strings.ToLower(name[:1])+name[1:]+"Cursor"] = pi.Cursor
				complete = false
			}
		}
		if complete {
			break
		}
	}
	for section := range failed {
		done[section] = false
	}
	result := reducePRStatus(all, done)
	result.Repository = strings.ToLower(canonical)
	result.Number = number
	t := now().UTC()
	if partial {
		t = firstObserved
		result.Error = "Bot review details could not be refreshed"
	}
	result.FetchedAt = &t
	return result, nil
}

type reviewerEvidence struct {
	actor            statusActor
	latest, decision *statusReview
	requested        bool
	findings         int
	comment          *statusComment
}

var noFindings = regexp.MustCompile(`(?im)^\s*(?:#{1,6}\s*)?(?:\*\*)?no new findings(?:\*\*)?[.!]?\s*$`)

func reducePRStatus(p statusPRRaw, complete map[string]bool) PRStatus {
	out := UnknownPRStatus("", 0, "")
	out.Head = p.Head
	out.Lifecycle = strings.ToLower(p.State)
	if p.IsDraft && out.Lifecycle == "open" {
		out.Lifecycle = "draft"
	}
	people := map[string]*reviewerEvidence{}
	evidence := func(a statusActor) *reviewerEvidence {
		key := strings.ToLower(a.Login)
		e := people[key]
		if e == nil {
			e = &reviewerEvidence{actor: a}
			people[key] = e
		}
		return e
	}
	sort.SliceStable(p.Reviews.Nodes, func(i, j int) bool {
		a, b := p.Reviews.Nodes[i], p.Reviews.Nodes[j]
		at, bt := a.SubmittedAt, b.SubmittedAt
		if at == "" {
			at = a.CreatedAt
		}
		if bt == "" {
			bt = b.CreatedAt
		}
		return at < bt
	})
	for i := range p.Reviews.Nodes {
		r := &p.Reviews.Nodes[i]
		if strings.EqualFold(r.Author.Login, p.Author.Login) {
			continue
		}
		if r.State == "PENDING" {
			continue
		}
		e := evidence(r.Author)
		e.latest = r
		if r.State == "APPROVED" || r.State == "CHANGES_REQUESTED" || r.State == "DISMISSED" {
			e.decision = r
		}
	}
	for _, r := range p.Requests.Nodes {
		a := r.Reviewer
		if a.Kind == "Team" {
			a.Login = "team:" + a.Slug
		}
		evidence(a).requested = true
	}
	threadActorsKnown := true
	for _, t := range p.Threads.Nodes {
		if t.IsResolved {
			continue
		}
		if len(t.Comments.Nodes) == 0 {
			threadActorsKnown = false
			continue
		}
		a := t.Comments.Nodes[0].Author
		if a.Kind != "Bot" && a.Kind != "User" {
			threadActorsKnown = false
		}
		if a.Kind == "Bot" {
			evidence(a).findings++
		}
	}
	for i := range p.Comments.Nodes {
		c := &p.Comments.Nodes[i]
		if c.Author.Kind == "Bot" {
			e := evidence(c.Author)
			if e.comment == nil || c.observedAt() > e.comment.observedAt() {
				e.comment = c
			}
		}
	}
	humansKnown := complete["Reviews"] && complete["Requests"]
	botsKnown := humansKnown && complete["Threads"] && complete["Comments"] && threadActorsKnown
	var rawChecks []statusCheckRaw
	if p.Commits.Nodes[0].Commit.Rollup != nil {
		rawChecks = p.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes
	}
	latest := map[string]statusCheckRaw{}
	for _, c := range rawChecks {
		key := c.identity() + ":" + c.CheckSuite.App.Slug
		if prev, ok := latest[key]; !ok || newerPRCheck(c, prev) {
			latest[key] = c
		}
	}
	rawChecks = make([]statusCheckRaw, 0, len(latest))
	for _, c := range latest {
		rawChecks = append(rawChecks, c)
	}
	for _, e := range people {
		r := StatusReviewer{Login: e.actor.Login, State: "none"}
		if e.actor.Kind == "Bot" {
			switch {
			case e.decision != nil && e.decision.State == "CHANGES_REQUESTED":
				r.State = "changes"
				r.Detail = "Formal GitHub change request"
			case e.findings > 0:
				r.State = "comments"
				r.Findings = e.findings
				r.Detail = "Unresolved bot review threads"
			case e.requested:
				r.State = "waiting"
				r.Detail = "Bot review requested"
			default:
				r.State = "unknown"
				r.Detail = "No verified outcome for the current commit"
				if e.latest != nil {
					v := e.latest
					if v.Commit.OID != p.Head {
						r.State = "stale"
						r.Detail = "Review belongs to an older commit"
					} else {
						if v.State == "APPROVED" {
							r.State = "approved"
							r.Detail = "Bot approved the current commit"
						} else if v.State == "COMMENTED" && noFindings.MatchString(v.Body) {
							r.State = "clean"
							r.Detail = "Completed on the current commit · no new findings"
						} else if v.State == "COMMENTED" && strings.TrimSpace(v.Body) != "" {
							r.State = "comments"
							r.Detail = "Commented review · inspect feedback on GitHub"
						} else if v.State == "COMMENTED" {
							r.State = "complete"
							r.Detail = "Completed on the current commit · no unresolved bot threads"
						}
					}
				}
				if e.comment != nil && (e.latest == nil || e.comment.observedAt() > e.latest.SubmittedAt) {
					c := e.comment
					r.State = "unknown"
					r.Detail = "Bot comment is not tied to the current commit"
					if strings.Contains(strings.ToLower(c.Body), strings.ToLower(p.Head)) {
						if noFindings.MatchString(c.Body) {
							r.State = "clean"
							r.Detail = "No new findings · current commit referenced"
						} else {
							r.State = "comments"
							r.Detail = "Bot feedback references the current commit"
						}
					}
				}
				for _, c := range rawChecks {
					slug := c.CheckSuite.App.Slug
					if slug != "" && strings.EqualFold(slug, strings.TrimSuffix(e.actor.Login, "[bot]")) && c.Status != "COMPLETED" && c.Typename == "CheckRun" {
						r.State = "running"
						r.Detail = "Bot check is running for the current commit"
					}
				}
			}
			if !botsKnown && r.State != "changes" && r.State != "comments" {
				r.State = "unknown"
				r.Detail = "Bot review history is incomplete"
			}
			out.Bots.Reviewers = append(out.Bots.Reviewers, r)
		} else {
			switch {
			case e.actor.Kind != "User" && e.actor.Kind != "Team":
				humansKnown = false
				r.State = "unknown"
				r.Detail = "Reviewer identity unavailable"
			case e.requested:
				r.State = "waiting"
				r.Detail = "Review requested"
				if e.decision != nil && e.decision.State == "CHANGES_REQUESTED" {
					r.Detail = "Re-review requested · previously requested changes"
				}
			case e.decision != nil && e.decision.State == "CHANGES_REQUESTED":
				r.State = "changes"
			case e.decision != nil && e.decision.State == "APPROVED" && e.decision.Commit.OID == p.Head:
				r.State = "approved"
			case e.decision != nil && e.decision.State == "APPROVED":
				r.State = "waiting"
				r.Detail = "Approval is for an older commit"
			case e.decision != nil && e.decision.State == "DISMISSED":
				r.State = "waiting"
				r.Detail = "Prior decision was dismissed"
			case e.latest != nil:
				r.State = "commented"
				r.Detail = "Commented without a formal decision"
			}
			out.Humans.Reviewers = append(out.Humans.Reviewers, r)
		}
	}
	sort.Slice(out.Humans.Reviewers, func(i, j int) bool { return out.Humans.Reviewers[i].Login < out.Humans.Reviewers[j].Login })
	sort.Slice(out.Bots.Reviewers, func(i, j int) bool { return out.Bots.Reviewers[i].Login < out.Bots.Reviewers[j].Login })
	out.Humans.State = reviewGroupState(out.Humans.Reviewers, humansKnown, false)
	out.Bots.State = reviewGroupState(out.Bots.Reviewers, botsKnown, true)
	if complete["Checks"] {
		out.CI.State = "none"
		if p.Commits.Nodes[0].Commit.Rollup != nil {
			switch p.Commits.Nodes[0].Commit.Rollup.State {
			case "SUCCESS":
				out.CI.State = "pass"
			case "FAILURE", "ERROR":
				out.CI.State = "fail"
			case "PENDING", "EXPECTED":
				out.CI.State = "pending"
			default:
				out.CI.State = "unknown"
			}
		}
	}
	if !complete["Checks"] && p.Commits.Nodes[0].Commit.Rollup != nil {
		state := p.Commits.Nodes[0].Commit.Rollup.State
		if state == "FAILURE" || state == "ERROR" {
			out.CI.State = "fail"
		}
	}
	for _, c := range latest {
		name := c.Name
		if c.Typename == "StatusContext" {
			name = c.Context
		}
		state := prCheckState(c)
		detail := c.Conclusion
		if c.Typename == "StatusContext" {
			detail = c.State
		}
		out.CI.Jobs = append(out.CI.Jobs, StatusCheck{name, state, strings.ToLower(detail)})
	}
	sort.Slice(out.CI.Jobs, func(i, j int) bool { return out.CI.Jobs[i].Name < out.CI.Jobs[j].Name })
	if !humansKnown || !botsKnown || !complete["Checks"] {
		out.Error = "Some PR status details are incomplete"
	}
	return out
}

// CheckRun has no creation timestamp. Its run ID distinguishes a new queued
// rerun (which has no startedAt yet) from the completed run it supersedes.
func newerPRCheck(current, previous statusCheckRaw) bool {
	if current.Typename == "CheckRun" && current.DatabaseID > 0 && previous.DatabaseID > 0 && current.DatabaseID != previous.DatabaseID {
		return current.DatabaseID > previous.DatabaseID
	}
	return current.ranAt() > previous.ranAt()
}

func prCheckState(c statusCheckRaw) string {
	if c.Typename == "StatusContext" {
		switch c.State {
		case "SUCCESS":
			return "pass"
		case "FAILURE", "ERROR":
			return "fail"
		case "PENDING", "EXPECTED":
			return "pending"
		default:
			return "unknown"
		}
	}
	if c.Typename != "CheckRun" {
		return "unknown"
	}
	if c.Status != "COMPLETED" {
		switch c.Status {
		case "QUEUED", "IN_PROGRESS", "WAITING", "PENDING", "REQUESTED":
			return "pending"
		default:
			return "unknown"
		}
	}
	switch c.Conclusion {
	case "SUCCESS", "SKIPPED", "NEUTRAL":
		return "pass"
	case "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return "fail"
	default:
		return "unknown"
	}
}

func reviewGroupState(rows []StatusReviewer, complete, bots bool) string {
	if bots {
		for _, state := range []string{"changes", "comments", "running", "waiting", "unknown", "stale"} {
			for _, r := range rows {
				if r.State == state {
					return state
				}
			}
		}
		if !complete {
			return "unknown"
		}
		if len(rows) > 0 {
			for _, r := range rows {
				if r.State == "approved" || r.State == "complete" {
					return "complete"
				}
			}
			return "clean"
		}
		return "none"
	}
	for _, r := range rows {
		if r.State == "changes" {
			return "changes"
		}
	}
	if !complete {
		return "unknown"
	}
	for _, r := range rows {
		if r.State == "waiting" {
			return "waiting"
		}
	}
	for _, r := range rows {
		if r.State == "unknown" {
			return "unknown"
		}
	}
	for _, r := range rows {
		if r.State == "approved" {
			return "approved"
		}
	}
	return "none"
}

// PRStatusService deduplicates concurrent consumers and caches each PR for 60s.
// A failed refresh keeps the previous timestamp and marks the result stale.
type PRStatusService struct {
	mu      sync.Mutex
	entries map[string]*prStatusEntry
	fetch   func(context.Context, string, int64) (PRStatus, error)
	now     func() time.Time
	queues  map[string]int
}
type prStatusEntry struct {
	snapshot  PRStatus
	attempted time.Time
	done      chan struct{}
}

func NewPRStatusService(fetch func(context.Context, string, int64) (PRStatus, error), now func() time.Time) *PRStatusService {
	if fetch == nil {
		fetch = (StatusFetcher{}).Fetch
	}
	if now == nil {
		now = time.Now
	}
	return &PRStatusService{entries: map[string]*prStatusEntry{}, fetch: fetch, now: now, queues: map[string]int{}}
}

// Rotate the first worker-sized batch on every poll, even if all prior reads
// failed. Cache age alone cannot provide fairness when every entry is unknown.
func (s *PRStatusService) NextBatchStart(scope string, count, workers int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count == 0 {
		return 0
	}
	start := s.queues[scope] % count
	s.queues[scope] = (start + workers) % count
	return start
}

func (s *PRStatusService) Get(ctx context.Context, repo string, number int64) PRStatus {
	key := fmt.Sprintf("%s/%d", strings.ToLower(repo), number)
	for {
		s.mu.Lock()
		e := s.entries[key]
		now := s.now()
		if e != nil && e.done != nil {
			done := e.done
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return UnknownPRStatus(repo, number, "PR lookup timed out")
			case <-done:
				continue
			}
		}
		if e != nil {
			ttl := time.Minute
			if e.snapshot.Lifecycle == "merged" || e.snapshot.Lifecycle == "closed" {
				ttl = 5 * time.Minute
			}
			if e.snapshot.Stale || e.snapshot.FetchedAt == nil {
				ttl = 10 * time.Second
			}
			if now.Sub(e.attempted) < ttl {
				result := e.snapshot
				s.mu.Unlock()
				return result
			}
		}
		if ctx.Err() != nil {
			result := UnknownPRStatus(repo, number, "PR lookup timed out")
			if e != nil {
				result = e.snapshot
				result.Stale = true
				result.Error = "PR lookup timed out"
			}
			s.mu.Unlock()
			return result
		}
		if e == nil {
			e = &prStatusEntry{snapshot: UnknownPRStatus(repo, number, "")}
			s.entries[key] = e
		}
		e.done = make(chan struct{})
		e.attempted = now
		s.mu.Unlock()
		lookup, cancel := context.WithTimeout(ctx, 25*time.Second)
		snapshot, err := s.fetch(lookup, repo, number)
		cancel()
		s.mu.Lock()
		if err != nil {
			snapshot = e.snapshot
			snapshot.Error = err.Error()
			snapshot.Stale = true
		} else {
			snapshot.Repository = strings.ToLower(repo)
			snapshot.Number = number
		}
		e.snapshot = snapshot
		close(e.done)
		e.done = nil
		s.mu.Unlock()
		return snapshot
	}
}

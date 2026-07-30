package github

import (
	"context"
	"sync"
	"time"
)

// Result is the full /api/projects/{id}/pull-requests payload: a state enum
// plus the last-good PR list. The JSON tags ARE the endpoint contract — the
// frontend renders these fields verbatim and branches on State/Stale, never on
// the HTTP status (every response is 200, the quota precedent). This is a
// structural clone of quota.Result with PRs instead of Windows.
//
// State enum: "ok" | "no_gh" | "auth_required" | "disabled" | "error".
// "disabled" is produced ONLY by the endpoint gate (Plan 02 — toggle off or
// project not linked); the Service itself never returns it. It is listed here
// to document the complete wire contract.
type Result struct {
	State     string      `json:"state"`
	Stale     bool        `json:"stale"`
	FetchedAt *time.Time  `json:"fetchedAt"` // last SUCCESSFUL fetch; null until first success
	PRs       []PRSummary `json:"prs"`       // awaiting-review queue; null when no data to show
	Reviewed  []PRSummary `json:"reviewed"`  // reviewed-by:@me, deduped against PRs (D-13/D-14); null when no data
}

// repoEntry is the per-repo cache state. Unlike quota (one global account /
// one entry), each linked project is a distinct repo, so the Service holds one
// of these per canonical owner/name. Two timestamps are deliberate (Pitfall 3):
// fetchedAt drives the 60s success-TTL and the wire FetchedAt; lastAttempt
// drives the 10s hard floor that binds EVERY attempt, even force. Both review
// queues (awaiting + reviewed) cache under this ONE entry — one fetchedAt, one
// TTL/floor — so the two sections always reflect the same gh snapshot (D-12).
type repoEntry struct {
	cachedAwaiting []PRSummary // last-good user-review-requested:@me list
	cachedReviewed []PRSummary // last-good reviewed-by:@me list (deduped)
	hasCache       bool        // true once a success has cached; gates the TTL serve
	fetchedAt      time.Time   // zero until first success
	lastAttempt    time.Time   // set on EVERY attempt (binds even force)
	failures       int         // consecutive; >= maxFailures drops cached
	lastErr        string      // "" | "no_gh" | "auth_required" | "error"
	inflight       bool        // in-flight dedup
}

// Config is the public construction seam. Zero values select production
// defaults. Tests inject a fixed clock (Now) and a fake runner (Runner) that
// counts calls — the analogue of quota's CredentialsPath/BaseURL test seam.
// Runner now returns BOTH review queues in one call (PRLists) so a single
// cached cycle drives both sections (D-12). CompletedRunner is the separate
// seam for the activity endpoint's merged/closed list (D-03: decoupled from
// the review-column Runner).
type Config struct {
	Now             func() time.Time                                                                       // default: time.Now
	Runner          func(ctx context.Context, repo, repoDir string) (PRLists, string, error)               // default: fetchLists
	CompletedRunner func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error) // default: listCompletedReviews
}

// Service is the demand-driven, per-repo PR-list cache. Like quota.Service it
// has NO ticker goroutine: the browser's poll (scoped to the visible project,
// D-11) is the only trigger, so an idle server makes zero gh spawns. The state
// machine — 60s TTL, 10s attempt floor, in-flight dedup, drop-cache-after-N —
// is the quota gate ladder retargeted from token-fingerprint to owner/name
// keying.
//
// D-03: the merged/closed reviews cache (completed) is a SEPARATE map with its
// OWN TTL (mergedClosedTTL, 5min) — the activity page's occasional views must
// not 3x the gh load on the review column's 5s-poll hot path. The two caches
// are independently keyed and never cross-populate; Get never touches
// `completed` and GetMergedClosed never touches `entries`.
type Service struct {
	mu              sync.Mutex
	now             func() time.Time
	runner          func(ctx context.Context, repo, repoDir string) (PRLists, string, error)
	entries         map[string]*repoEntry            // keyed by canonical owner/name — the review-column cache (60s TTL)
	completedRunner func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error)
	completed       map[string]*mergedClosedEntry // keyed by canonical owner/name — the merged/closed reviews cache (5min TTL, D-03)
}

// mergedClosedEntry is the per-repo cache state for the activity endpoint's
// reviews-done list (D-03). It mirrors repoEntry's state machine — success-TTL,
// attempt floor, in-flight dedup, drop-cache-after-N — but is typed for the
// merged/closed ReviewDoneSummary list and carries its OWN TTL
// (mergedClosedTTL = 5min, not cacheTTL = 60s). Decoupled from repoEntry so
// the activity page's occasional views never multiply gh load on the
// review-column hot path.
type mergedClosedEntry struct {
	cached       []ReviewDoneSummary // last-good merged/closed list (sorted desc by CompletedAt)
	hasCache     bool                // true once a success has cached; gates the TTL serve
	fetchedAt    time.Time           // zero until first success
	lastAttempt  time.Time           // set on EVERY attempt (binds even force)
	failures     int                 // consecutive; >= maxFailures drops cached
	lastErr      string              // "" | "no_gh" | "auth_required" | "error"
	inflight     bool                // in-flight dedup
}

// MergedClosedResult is the per-repo merged/closed reviews result envelope
// returned by GetMergedClosed. It mirrors Result's shape (State/Stale/
// FetchedAt/PRs) so the activity endpoint's reviews sub-object (D-02) reads
// the same wire contract as the review column — just under a `reviews` key
// with ReviewDoneSummary elements instead of PRSummary. JSON tags ARE the
// endpoint contract. State is one of "ok"|"no_gh"|"auth_required"|"error";
// GetMergedClosed never returns "disabled" (that state is endpoint-only,
// produced by the Plan 02 GATE 1 toggle check, never by the Service — mirrors
// the service.go:15-18 contract on Result).
type MergedClosedResult struct {
	State     string             `json:"state"`
	Stale     bool               `json:"stale"`
	FetchedAt *time.Time         `json:"fetchedAt"` // last SUCCESSFUL fetch; null until first success
	PRs       []ReviewDoneSummary `json:"prs"`       // the merged/closed list; nil when no data to show
}

// New builds a Service from cfg, applying production defaults (time.Now,
// fetchLists, listCompletedReviews) for zero values.
func New(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	runner := cfg.Runner
	if runner == nil {
		runner = fetchLists
	}
	completedRunner := cfg.CompletedRunner
	if completedRunner == nil {
		completedRunner = listCompletedReviews
	}
	return &Service{
		now:             now,
		runner:          runner,
		entries:         map[string]*repoEntry{},
		completedRunner: completedRunner,
		completed:       map[string]*mergedClosedEntry{},
	}
}

const (
	cacheTTL        = 60 * time.Second  // gates re-fetch after a SUCCESS (matches the 60s poll)
	attemptFloor    = 10 * time.Second  // gates every attempt, even force (anti-spam on ?refresh=1)
	maxFailures     = 3                 // consecutive failures before cached PRs drop
	mergedClosedTTL = 5 * time.Minute   // D-04: success-TTL for the merged/closed reviews cache (activity page is occasional, not live)
)

// entry returns the per-repo cache entry, creating it on first use. Caller
// holds mu.
func (s *Service) entry(repo string) *repoEntry {
	e := s.entries[repo]
	if e == nil {
		e = &repoEntry{}
		s.entries[repo] = e
	}
	return e
}

// Get returns the current PR Result for repo, fetching via the runner only
// when the demand-driven cache rules allow it. force corresponds to ?refresh=1.
// The gate ladder is quota.Get's (lines 259-303) retargeted to a per-repo
// entry: the 10s floor binds lastAttempt (every attempt), the 60s TTL binds
// fetchedAt (successes) — separate fields, so a failing gh is not hammered by
// the 60s poll nor by refresh-button mashing (Pitfall 3).
func (s *Service) Get(ctx context.Context, repo, repoDir string, force bool) Result {
	s.mu.Lock()
	e := s.entry(repo)
	now := s.now()

	// Serve the cache without fetching when any gate holds.
	if (!force && e.hasCache && now.Sub(e.fetchedAt) < cacheTTL) ||
		now.Sub(e.lastAttempt) < attemptFloor ||
		e.inflight {
		res := e.resultLocked()
		s.mu.Unlock()
		return res
	}

	// Fetch. Release mu during the gh spawn; inflight dedups concurrent Gets
	// onto the cache meanwhile.
	e.inflight = true
	e.lastAttempt = now
	s.mu.Unlock()

	lists, state, _ := s.runner(ctx, repo, repoDir)

	s.mu.Lock()
	defer s.mu.Unlock()
	e.inflight = false
	switch state {
	case "ok":
		e.cachedAwaiting = lists.Awaiting
		e.cachedReviewed = lists.Reviewed
		e.hasCache = true
		e.fetchedAt = s.now()
		e.failures = 0
		e.lastErr = ""
	default: // no_gh / auth_required / error
		e.recordFailureLocked(state)
	}
	return e.resultLocked()
}

// recordFailureLocked increments the consecutive-failure counter, remembers
// which degraded state to report, and drops BOTH cached lists once maxFailures
// is reached (one gh failure degrades both sections together, D-12). Caller
// holds the Service mu.
func (e *repoEntry) recordFailureLocked(state string) {
	e.failures++
	e.lastErr = state
	if e.failures >= maxFailures {
		e.cachedAwaiting = nil
		e.cachedReviewed = nil
		e.hasCache = false
	}
}

// resultLocked materializes the Result for the current entry state. Both
// review queues ride the same fetchedAt and the same Stale flag — they are one
// cached snapshot. Caller holds the Service mu.
func (e *repoEntry) resultLocked() Result {
	state := "ok"
	if e.lastErr != "" {
		state = e.lastErr
	}
	res := Result{State: state, Stale: e.lastErr != "" && e.hasCache}
	if e.hasCache {
		res.PRs = e.cachedAwaiting
		res.Reviewed = e.cachedReviewed
		ft := e.fetchedAt
		res.FetchedAt = &ft
	}
	return res
}

// entryMergedClosed returns the per-repo merged/closed cache entry, creating
// it on first use. Caller holds mu. Mirrors entry(repo) for the separate
// merged/closed cache (D-03).
func (s *Service) entryMergedClosed(repo string) *mergedClosedEntry {
	e := s.completed[repo]
	if e == nil {
		e = &mergedClosedEntry{}
		s.completed[repo] = e
	}
	return e
}

// recordFailureLocked increments the consecutive-failure counter, remembers
// the degraded state, and drops the cached merged/closed list once maxFailures
// is reached — mirroring repoEntry.recordFailureLocked. Caller holds the
// Service mu.
func (e *mergedClosedEntry) recordFailureLocked(state string) {
	e.failures++
	e.lastErr = state
	if e.failures >= maxFailures {
		e.cached = nil
		e.hasCache = false
	}
}

// resultMergedClosedLocked materializes the MergedClosedResult for the current
// entry state. Caller holds the Service mu.
func (e *mergedClosedEntry) resultMergedClosedLocked() MergedClosedResult {
	state := "ok"
	if e.lastErr != "" {
		state = e.lastErr
	}
	res := MergedClosedResult{State: state, Stale: e.lastErr != "" && e.hasCache}
	if e.hasCache {
		res.PRs = e.cached
		ft := e.fetchedAt
		res.FetchedAt = &ft
	}
	return res
}

// GetMergedClosed returns the current merged/closed reviews result for repo,
// fetching via completedRunner only when the demand-driven cache rules allow
// it. force corresponds to ?refresh=1. The gate ladder is Get's retargeted to
// the merged/closed entry: the 10s floor binds lastAttempt (every attempt),
// the 5min mergedClosedTTL binds fetchedAt (successes) — separate from the
// review-column cacheTTL so the activity page's occasional views never
// multiply gh load on the 5s-poll hot path (D-03/D-04). NEVER touches
// s.entries; never returns state "disabled" (endpoint-only, Plan 02 GATE 1).
func (s *Service) GetMergedClosed(ctx context.Context, repo, repoDir string, force bool) MergedClosedResult {
	_ = ctx
	_ = repo
	_ = repoDir
	_ = force
	return MergedClosedResult{} // RED STUB
}

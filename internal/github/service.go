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
// cached cycle drives both sections (D-12).
type Config struct {
	Now    func() time.Time                                                       // default: time.Now
	Runner func(ctx context.Context, repo, repoDir string) (PRLists, string, error) // default: fetchLists
}

// Service is the demand-driven, per-repo PR-list cache. Like quota.Service it
// has NO ticker goroutine: the browser's poll (scoped to the visible project,
// D-11) is the only trigger, so an idle server makes zero gh spawns. The state
// machine — 60s TTL, 10s attempt floor, in-flight dedup, drop-cache-after-N —
// is the quota gate ladder retargeted from token-fingerprint to owner/name
// keying.
type Service struct {
	mu      sync.Mutex
	now     func() time.Time
	runner  func(ctx context.Context, repo, repoDir string) (PRLists, string, error)
	entries map[string]*repoEntry // keyed by canonical owner/name
}

// New builds a Service from cfg, applying production defaults (time.Now,
// fetchLists) for zero values.
func New(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	runner := cfg.Runner
	if runner == nil {
		runner = fetchLists
	}
	return &Service{
		now:     now,
		runner:  runner,
		entries: map[string]*repoEntry{},
	}
}

const (
	cacheTTL     = 60 * time.Second // gates re-fetch after a SUCCESS (matches the 60s poll)
	attemptFloor = 10 * time.Second // gates every attempt, even force (anti-spam on ?refresh=1)
	maxFailures  = 3                // consecutive failures before cached PRs drop
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

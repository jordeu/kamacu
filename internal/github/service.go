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
	PRs       []PRSummary `json:"prs"`       // null when no data to show
}

// repoEntry is the per-repo cache state. Unlike quota (one global account /
// one entry), each linked project is a distinct repo, so the Service holds one
// of these per canonical owner/name. Two timestamps are deliberate (Pitfall 3):
// fetchedAt drives the 60s success-TTL and the wire FetchedAt; lastAttempt
// drives the 10s hard floor that binds EVERY attempt, even force.
type repoEntry struct {
	cached      []PRSummary
	fetchedAt   time.Time // zero until first success
	lastAttempt time.Time // set on EVERY attempt (binds even force)
	failures    int       // consecutive; >= maxFailures drops cached
	lastErr     string    // "" | "no_gh" | "auth_required" | "error"
	inflight    bool      // in-flight dedup
}

// Config is the public construction seam. Zero values select production
// defaults. Tests inject a fixed clock (Now) and a fake runner (Runner) that
// counts calls — the analogue of quota's CredentialsPath/BaseURL test seam.
type Config struct {
	Now    func() time.Time                                                             // default: time.Now
	Runner func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error) // default: runGH
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
	runner  func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error)
	entries map[string]*repoEntry // keyed by canonical owner/name
}

// New builds a Service from cfg, applying production defaults (time.Now,
// runGH) for zero values.
func New(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	runner := cfg.Runner
	if runner == nil {
		runner = runGH
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
	if (!force && e.cached != nil && now.Sub(e.fetchedAt) < cacheTTL) ||
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

	prs, state, _ := s.runner(ctx, repo, repoDir)

	s.mu.Lock()
	defer s.mu.Unlock()
	e.inflight = false
	switch state {
	case "ok":
		e.cached = prs
		e.fetchedAt = s.now()
		e.failures = 0
		e.lastErr = ""
	default: // no_gh / auth_required / error
		e.recordFailureLocked(state)
	}
	return e.resultLocked()
}

// recordFailureLocked increments the consecutive-failure counter, remembers
// which degraded state to report, and drops the cached PRs once maxFailures is
// reached. Caller holds the Service mu.
func (e *repoEntry) recordFailureLocked(state string) {
	e.failures++
	e.lastErr = state
	if e.failures >= maxFailures {
		e.cached = nil
	}
}

// resultLocked materializes the Result for the current entry state. Caller
// holds the Service mu.
func (e *repoEntry) resultLocked() Result {
	state := "ok"
	if e.lastErr != "" {
		state = e.lastErr
	}
	res := Result{State: state, Stale: e.lastErr != "" && e.cached != nil}
	if e.cached != nil {
		res.PRs = e.cached
		ft := e.fetchedAt
		res.FetchedAt = &ft
	}
	return res
}

package github

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner builds a Service Runner that counts its invocations (per the
// quota_test countingHandler convention) and returns a canned (PRLists, state).
// The counter lets the gate-ladder tests assert exactly how many gh spawns
// happen; the PRLists shape carries BOTH the awaiting and reviewed queues that
// now ride one cache cycle (D-12).
func fakeRunner(hits *atomic.Int64, lists PRLists, state string) func(ctx context.Context, repo, repoDir string) (PRLists, string, error) {
	return func(ctx context.Context, repo, repoDir string) (PRLists, string, error) {
		hits.Add(1)
		return lists, state, nil
	}
}

// fakeCompletedRunner builds a Service CompletedRunner that counts its
// invocations and returns a canned ([]ReviewDoneSummary, state). Mirrors
// fakeRunner for the merged/closed cache (D-03): the counter lets the
// GetMergedClosed gate-ladder tests assert spawn counts; the canned slice is
// the merged/closed list.
func fakeCompletedRunner(hits *atomic.Int64, prs []ReviewDoneSummary, state string) func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error) {
	return func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error) {
		hits.Add(1)
		return prs, state, nil
	}
}

// newTestService wires a Service with a frozen, advanceable clock (the
// quota_test fakeClock pattern) and the given runner. It returns the Service
// and an advance func.
func newTestService(runner func(ctx context.Context, repo, repoDir string) (PRLists, string, error)) (*Service, func(time.Duration)) {
	cur := time.Now()
	s := New(Config{
		Now:    func() time.Time { return cur },
		Runner: runner,
	})
	return s, func(d time.Duration) { cur = cur.Add(d) }
}

// newTestServiceWithCompleted wires a Service with a frozen clock AND both
// runner seams (Runner for the review column, CompletedRunner for the
// merged/closed cache) so D-03 separation can be asserted — calling Get must
// not populate `completed`, calling GetMergedClosed must not populate
// `entries`. Returns the Service and an advance func.
func newTestServiceWithCompleted(
	runner func(ctx context.Context, repo, repoDir string) (PRLists, string, error),
	completedRunner func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error),
) (*Service, func(time.Duration)) {
	cur := time.Now()
	s := New(Config{
		Now:             func() time.Time { return cur },
		Runner:          runner,
		CompletedRunner: completedRunner,
	})
	return s, func(d time.Duration) { cur = cur.Add(d) }
}

// samplePRs is the canned awaiting-review list (the historical name kept so the
// existing gate-ladder assertions read unchanged).
func samplePRs() []PRSummary {
	return []PRSummary{
		{Number: 2, Title: "b", Checks: "pass"},
		{Number: 1, Title: "a", Checks: "fail"},
	}
}

// sampleReviewed is the canned reviewed-by:@me list (distinct PR numbers so no
// dedup collision with samplePRs — the runner is expected to have already
// deduped before returning).
func sampleReviewed() []PRSummary {
	return []PRSummary{
		{Number: 4, Title: "d", Checks: "pass"},
		{Number: 3, Title: "c", Checks: "pending"},
	}
}

// sampleLists bundles the two canned queues into the PRLists shape the runner
// now returns.
func sampleLists() PRLists {
	return PRLists{Awaiting: samplePRs(), Reviewed: sampleReviewed()}
}

func TestServiceGet(t *testing.T) {
	const repoA, repoB = "owner/a", "owner/b"
	const dir = "/tmp/a"
	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, sampleLists(), "ok"))
		res := s.Get(ctx, repoA, dir, false)
		if res.State != "ok" {
			t.Fatalf("State = %q, want ok", res.State)
		}
		if len(res.PRs) != 2 {
			t.Fatalf("len(PRs) = %d, want 2", len(res.PRs))
		}
		if len(res.Reviewed) != 2 {
			t.Fatalf("len(Reviewed) = %d, want 2 (reviewed list rides the same cycle)", len(res.Reviewed))
		}
		if res.FetchedAt == nil {
			t.Error("FetchedAt = nil, want non-nil after a successful fetch")
		}
		if hits.Load() != 1 {
			t.Errorf("runner hits = %d, want 1", hits.Load())
		}
	})

	t.Run("TTL caches within 60s", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, sampleLists(), "ok"))
		s.Get(ctx, repoA, dir, false)
		advance(30 * time.Second) // still inside the 60s TTL (and past the 10s floor)
		s.Get(ctx, repoA, dir, false)
		if hits.Load() != 1 {
			t.Errorf("runner hits = %d, want 1 (TTL should serve cache)", hits.Load())
		}
	})

	t.Run("force bypasses TTL past floor", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, sampleLists(), "ok"))
		s.Get(ctx, repoA, dir, false)
		advance(30 * time.Second) // past the 10s floor, still inside the 60s TTL
		s.Get(ctx, repoA, dir, true)
		if hits.Load() != 2 {
			t.Errorf("runner hits = %d, want 2 (force should bypass TTL)", hits.Load())
		}
	})

	t.Run("floor binds even force", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, sampleLists(), "ok"))
		s.Get(ctx, repoA, dir, true)
		advance(5 * time.Second) // inside the 10s floor
		s.Get(ctx, repoA, dir, true)
		if hits.Load() != 1 {
			t.Errorf("runner hits = %d, want 1 (10s floor binds even force)", hits.Load())
		}
	})

	t.Run("per-repo isolation", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, sampleLists(), "ok"))
		s.Get(ctx, repoA, dir, false)
		s.Get(ctx, repoB, "/tmp/b", false)
		if hits.Load() != 2 {
			t.Errorf("runner hits = %d, want 2 (each repo is its own cache entry)", hits.Load())
		}
	})

	t.Run("degrade no_gh never panics", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, PRLists{}, "no_gh"))
		res := s.Get(ctx, repoA, dir, false)
		if res.State != "no_gh" {
			t.Errorf("State = %q, want no_gh", res.State)
		}
		if res.PRs != nil {
			t.Errorf("PRs = %+v, want nil on no_gh", res.PRs)
		}
		if res.Reviewed != nil {
			t.Errorf("Reviewed = %+v, want nil on no_gh (degraded-no-cache read)", res.Reviewed)
		}
	})

	t.Run("drop cached after maxFailures", func(t *testing.T) {
		var state atomic.Value
		state.Store("ok")
		var hits atomic.Int64
		runner := func(ctx context.Context, repo, repoDir string) (PRLists, string, error) {
			hits.Add(1)
			return sampleLists(), state.Load().(string), nil
		}
		s, advance := newTestService(runner)

		// First fetch succeeds and caches.
		if res := s.Get(ctx, repoA, dir, false); res.State != "ok" || len(res.PRs) != 2 || len(res.Reviewed) != 2 {
			t.Fatalf("seed fetch State=%q prs=%d reviewed=%d, want ok/2/2", res.State, len(res.PRs), len(res.Reviewed))
		}

		// Now flip to error and fetch 3 consecutive times (advancing past the
		// floor each time so each Get actually spawns the runner).
		state.Store("error")
		var last Result
		for i := range 3 {
			advance(15 * time.Second) // clear the 10s floor; force bypasses TTL
			last = s.Get(ctx, repoA, dir, true)
			if i < 2 {
				// before the 3rd failure, both stale lists are still served
				if len(last.PRs) != 2 {
					t.Errorf("after %d failures len(PRs)=%d, want 2 (serve stale until maxFailures)", i+1, len(last.PRs))
				}
				if len(last.Reviewed) != 2 {
					t.Errorf("after %d failures len(Reviewed)=%d, want 2 (reviewed served stale too)", i+1, len(last.Reviewed))
				}
				if !last.Stale {
					t.Errorf("after %d failures Stale=false, want true", i+1)
				}
			}
		}
		if last.PRs != nil {
			t.Errorf("after 3 failures PRs=%+v, want nil (dropped at maxFailures)", last.PRs)
		}
		if last.Reviewed != nil {
			t.Errorf("after 3 failures Reviewed=%+v, want nil (both lists dropped at maxFailures)", last.Reviewed)
		}
		if last.State != "error" {
			t.Errorf("after 3 failures State=%q, want error", last.State)
		}
	})
}

// sampleCompletedPRs is the canned merged/closed list (the activity endpoint's
// reviews-done data — distinct from samplePRs's review-column shape).
func sampleCompletedPRs() []ReviewDoneSummary {
	return []ReviewDoneSummary{
		{Number: 42, Title: "ship it", CompletedAt: "2026-07-25T14:30:00Z", URL: "https://x/42"},
		{Number: 39, Title: "merge me", CompletedAt: "2026-07-20T09:00:00Z", URL: "https://x/39"},
	}
}

// TestGetMergedClosed is the gate-ladder guard for the merged/closed reviews
// cache (D-03/D-04): it mirrors TestServiceGet's shape but asserts the
// SEPARATE 5min TTL (mergedClosedTTL) and D-03 cache independence from the
// review-column entries map. The runner is the Config.CompletedRunner seam
// (defaults to listCompletedReviews in production); the fake counts spawns so
// each gate can be asserted precisely.
func TestGetMergedClosed(t *testing.T) {
	const repoA = "owner/a"
	const dir = "/tmp/a"
	ctx := context.Background()

	t.Run("happy path single spawn + cache", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, sampleCompletedPRs(), "ok"),
		)
		res := s.GetMergedClosed(ctx, repoA, dir, false)
		if res.State != "ok" {
			t.Fatalf("State = %q, want ok", res.State)
		}
		if len(res.PRs) != 2 {
			t.Fatalf("len(PRs) = %d, want 2", len(res.PRs))
		}
		if res.FetchedAt == nil {
			t.Error("FetchedAt = nil, want non-nil after a successful fetch")
		}
		if hits.Load() != 1 {
			t.Errorf("completedRunner hits = %d, want 1", hits.Load())
		}
	})

	t.Run("cache hit within mergedClosedTTL zero spawns", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, sampleCompletedPRs(), "ok"),
		)
		s.GetMergedClosed(ctx, repoA, dir, false) // seed
		// 4 minutes — still inside the 5min mergedClosedTTL (and past the 10s floor).
		advance(4 * time.Minute)
		s.GetMergedClosed(ctx, repoA, dir, false)
		if hits.Load() != 1 {
			t.Errorf("completedRunner hits = %d, want 1 (5min TTL should serve cache at 4min)", hits.Load())
		}
	})

	t.Run("TTL expires past mergedClosedTTL", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, sampleCompletedPRs(), "ok"),
		)
		s.GetMergedClosed(ctx, repoA, dir, false) // seed
		// 5min + change — past the 5min TTL, past the 10s floor → re-fetch.
		advance(5*time.Minute + time.Second)
		s.GetMergedClosed(ctx, repoA, dir, false)
		if hits.Load() != 2 {
			t.Errorf("completedRunner hits = %d, want 2 (TTL expired at 5min)", hits.Load())
		}
	})

	t.Run("force bypasses mergedClosedTTL past floor", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, sampleCompletedPRs(), "ok"),
		)
		s.GetMergedClosed(ctx, repoA, dir, false) // seed
		// 3min — inside the 5min TTL, past the 10s floor; force bypasses TTL.
		advance(3 * time.Minute)
		s.GetMergedClosed(ctx, repoA, dir, true)
		if hits.Load() != 2 {
			t.Errorf("completedRunner hits = %d, want 2 (force bypasses mergedClosedTTL)", hits.Load())
		}
	})

	t.Run("attemptFloor binds even force", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, sampleCompletedPRs(), "ok"),
		)
		s.GetMergedClosed(ctx, repoA, dir, true) // seed + force
		advance(5 * time.Second)                // inside the 10s floor
		s.GetMergedClosed(ctx, repoA, dir, true)
		if hits.Load() != 1 {
			t.Errorf("completedRunner hits = %d, want 1 (10s floor binds even force)", hits.Load())
		}
		// Past the floor, force should now spawn.
		advance(6 * time.Second) // 11s total — past the 10s floor
		s.GetMergedClosed(ctx, repoA, dir, true)
		if hits.Load() != 2 {
			t.Errorf("completedRunner hits = %d, want 2 (force spawns after floor clears)", hits.Load())
		}
	})

	t.Run("drop cached after maxFailures", func(t *testing.T) {
		var state atomic.Value
		state.Store("ok")
		var hits atomic.Int64
		completed := func(ctx context.Context, repo, repoDir string) ([]ReviewDoneSummary, string, error) {
			hits.Add(1)
			return sampleCompletedPRs(), state.Load().(string), nil
		}
		s, advance := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			completed,
		)
		// Seed a successful cache.
		if res := s.GetMergedClosed(ctx, repoA, dir, false); res.State != "ok" || len(res.PRs) != 2 {
			t.Fatalf("seed fetch State=%q prs=%d, want ok/2", res.State, len(res.PRs))
		}
		// Flip to error and fetch 3 consecutive times (advancing past the floor).
		state.Store("error")
		var last MergedClosedResult
		for i := range 3 {
			advance(15 * time.Second) // clear the 10s floor; force bypasses TTL
			last = s.GetMergedClosed(ctx, repoA, dir, true)
			if i < 2 {
				if len(last.PRs) != 2 {
					t.Errorf("after %d failures len(PRs)=%d, want 2 (serve stale until maxFailures)", i+1, len(last.PRs))
				}
				if !last.Stale {
					t.Errorf("after %d failures Stale=false, want true", i+1)
				}
			}
		}
		if last.PRs != nil {
			t.Errorf("after 3 failures PRs=%+v, want nil (dropped at maxFailures)", last.PRs)
		}
		if last.State != "error" {
			t.Errorf("after 3 failures State=%q, want error", last.State)
		}
	})

	t.Run("Get and GetMergedClosed caches are independent (D-03)", func(t *testing.T) {
		var runnerHits atomic.Int64
		var completedHits atomic.Int64
		s, _ := newTestServiceWithCompleted(
			fakeRunner(&runnerHits, sampleLists(), "ok"),
			fakeCompletedRunner(&completedHits, sampleCompletedPRs(), "ok"),
		)

		// Drive Get (review column) and GetMergedClosed (activity) on the same repo.
		getRes := s.Get(ctx, repoA, dir, false)
		gmcRes := s.GetMergedClosed(ctx, repoA, dir, false)

		// Each method hit its own runner exactly once.
		if runnerHits.Load() != 1 {
			t.Errorf("Runner hits = %d, want 1 (Get only)", runnerHits.Load())
		}
		if completedHits.Load() != 1 {
			t.Errorf("CompletedRunner hits = %d, want 1 (GetMergedClosed only)", completedHits.Load())
		}

		// The two caches are independently keyed: `entries` has repoA from Get,
		// `completed` has repoA from GetMergedClosed. Assert the maps are
		// populated independently and the result shapes don't cross-pollute.
		s.mu.Lock()
		_, hasEntry := s.entries[repoA]
		_, hasCompleted := s.completed[repoA]
		s.mu.Unlock()
		if !hasEntry {
			t.Error("s.entries[repoA] missing after Get (Get must populate entries)")
		}
		if !hasCompleted {
			t.Error("s.completed[repoA] missing after GetMergedClosed (must populate completed)")
		}

		// The wire shapes are distinct types — Get returns PRSummary lists,
		// GetMergedClosed returns ReviewDoneSummary. A cross-contamination bug
		// would surface as the wrong slice length or wrong element type.
		if len(getRes.PRs) != len(samplePRs()) {
			t.Errorf("Get PRs len = %d, want %d (review-column shape)", len(getRes.PRs), len(samplePRs()))
		}
		if len(gmcRes.PRs) != len(sampleCompletedPRs()) {
			t.Errorf("GetMergedClosed PRs len = %d, want %d (merged/closed shape)", len(gmcRes.PRs), len(sampleCompletedPRs()))
		}

		// Calling Get again must NOT trigger CompletedRunner, and calling
		// GetMergedClosed again must NOT trigger Runner (each method only
		// touches its own cache). Both within TTL → zero additional spawns.
		s.Get(ctx, repoA, dir, false)
		s.GetMergedClosed(ctx, repoA, dir, false)
		if runnerHits.Load() != 1 {
			t.Errorf("Runner hits = %d after GetMergedClosed call, want 1 (caches must not cross-populate)", runnerHits.Load())
		}
		if completedHits.Load() != 1 {
			t.Errorf("CompletedRunner hits = %d after Get call, want 1 (caches must not cross-populate)", completedHits.Load())
		}
	})

	t.Run("degrade no_gh never panics", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, nil, "no_gh"),
		)
		res := s.GetMergedClosed(ctx, repoA, dir, false)
		if res.State != "no_gh" {
			t.Errorf("State = %q, want no_gh", res.State)
		}
		if res.PRs != nil {
			t.Errorf("PRs = %+v, want nil on no_gh", res.PRs)
		}
		if res.FetchedAt != nil {
			t.Errorf("FetchedAt = %v, want nil on no_gh (no successful fetch)", res.FetchedAt)
		}
	})

	t.Run("never returns state disabled", func(t *testing.T) {
		// GetMergedClosed must never emit "disabled" — that state is endpoint-
		// only (Plan 02 GATE 1 toggle). The Service emits only ok/no_gh/
		// auth_required/error.
		var hits atomic.Int64
		s, _ := newTestServiceWithCompleted(
			fakeRunner(&atomic.Int64{}, sampleLists(), "ok"),
			fakeCompletedRunner(&hits, nil, "error"),
		)
		res := s.GetMergedClosed(ctx, repoA, dir, false)
		if res.State == "disabled" {
			t.Errorf("State = disabled; GetMergedClosed must never return it (endpoint-only, Plan 02 GATE 1)")
		}
	})
}

package github

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner builds a Service Runner that counts its invocations (per the
// quota_test countingHandler convention) and returns canned (prs, state). The
// counter lets the gate-ladder tests assert exactly how many gh spawns happen.
func fakeRunner(hits *atomic.Int64, prs []PRSummary, state string) func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error) {
	return func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error) {
		hits.Add(1)
		return prs, state, nil
	}
}

// newTestService wires a Service with a frozen, advanceable clock (the
// quota_test fakeClock pattern) and the given runner. It returns the Service
// and an advance func.
func newTestService(runner func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error)) (*Service, func(time.Duration)) {
	cur := time.Now()
	s := New(Config{
		Now:    func() time.Time { return cur },
		Runner: runner,
	})
	return s, func(d time.Duration) { cur = cur.Add(d) }
}

func samplePRs() []PRSummary {
	return []PRSummary{
		{Number: 2, Title: "b", Checks: "pass"},
		{Number: 1, Title: "a", Checks: "fail"},
	}
}

func TestServiceGet(t *testing.T) {
	const repoA, repoB = "owner/a", "owner/b"
	const dir = "/tmp/a"
	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, samplePRs(), "ok"))
		res := s.Get(ctx, repoA, dir, false)
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
			t.Errorf("runner hits = %d, want 1", hits.Load())
		}
	})

	t.Run("TTL caches within 60s", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, samplePRs(), "ok"))
		s.Get(ctx, repoA, dir, false)
		advance(30 * time.Second) // still inside the 60s TTL (and past the 10s floor)
		s.Get(ctx, repoA, dir, false)
		if hits.Load() != 1 {
			t.Errorf("runner hits = %d, want 1 (TTL should serve cache)", hits.Load())
		}
	})

	t.Run("force bypasses TTL past floor", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, samplePRs(), "ok"))
		s.Get(ctx, repoA, dir, false)
		advance(30 * time.Second) // past the 10s floor, still inside the 60s TTL
		s.Get(ctx, repoA, dir, true)
		if hits.Load() != 2 {
			t.Errorf("runner hits = %d, want 2 (force should bypass TTL)", hits.Load())
		}
	})

	t.Run("floor binds even force", func(t *testing.T) {
		var hits atomic.Int64
		s, advance := newTestService(fakeRunner(&hits, samplePRs(), "ok"))
		s.Get(ctx, repoA, dir, true)
		advance(5 * time.Second) // inside the 10s floor
		s.Get(ctx, repoA, dir, true)
		if hits.Load() != 1 {
			t.Errorf("runner hits = %d, want 1 (10s floor binds even force)", hits.Load())
		}
	})

	t.Run("per-repo isolation", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, samplePRs(), "ok"))
		s.Get(ctx, repoA, dir, false)
		s.Get(ctx, repoB, "/tmp/b", false)
		if hits.Load() != 2 {
			t.Errorf("runner hits = %d, want 2 (each repo is its own cache entry)", hits.Load())
		}
	})

	t.Run("degrade no_gh never panics", func(t *testing.T) {
		var hits atomic.Int64
		s, _ := newTestService(fakeRunner(&hits, nil, "no_gh"))
		res := s.Get(ctx, repoA, dir, false)
		if res.State != "no_gh" {
			t.Errorf("State = %q, want no_gh", res.State)
		}
		if res.PRs != nil {
			t.Errorf("PRs = %+v, want nil on no_gh", res.PRs)
		}
	})

	t.Run("drop cached after maxFailures", func(t *testing.T) {
		var state atomic.Value
		state.Store("ok")
		var hits atomic.Int64
		runner := func(ctx context.Context, repo, repoDir string) ([]PRSummary, string, error) {
			hits.Add(1)
			return samplePRs(), state.Load().(string), nil
		}
		s, advance := newTestService(runner)

		// First fetch succeeds and caches.
		if res := s.Get(ctx, repoA, dir, false); res.State != "ok" || len(res.PRs) != 2 {
			t.Fatalf("seed fetch State=%q len=%d, want ok/2", res.State, len(res.PRs))
		}

		// Now flip to error and fetch 3 consecutive times (advancing past the
		// floor each time so each Get actually spawns the runner).
		state.Store("error")
		var last Result
		for i := range 3 {
			advance(15 * time.Second) // clear the 10s floor; force bypasses TTL
			last = s.Get(ctx, repoA, dir, true)
			if i < 2 {
				// before the 3rd failure, stale cache is still served
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
}

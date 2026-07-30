package github

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"testing"
)

// TestReduceChecks is the table guard for the statusCheckRollup → pill
// reduction (D-00d). It exercises BOTH __typename variants (CheckRun &
// StatusContext) plus the aggregate precedence (fail > pending > pass), and —
// critically — pins SKIPPED/NEUTRAL/STALE as NON-failing (Pitfall 1: those
// conclusions dominate real PRs and must never paint the dot red).
func TestReduceChecks(t *testing.T) {
	tests := []struct {
		name   string
		rollup []checkEntry
		want   string
	}{
		// --- empty ---
		{"empty rollup", nil, "none"},
		{"empty slice", []checkEntry{}, "none"},

		// --- single CheckRun ---
		{"checkrun success", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}}, "pass"},
		{"checkrun in progress", []checkEntry{{Typename: "CheckRun", Status: "IN_PROGRESS"}}, "pending"},
		{"checkrun queued", []checkEntry{{Typename: "CheckRun", Status: "QUEUED"}}, "pending"},
		{"checkrun failure", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"}}, "fail"},
		{"checkrun timed out", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "TIMED_OUT"}}, "fail"},
		{"checkrun cancelled", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "CANCELLED"}}, "fail"},
		{"checkrun action required", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "ACTION_REQUIRED"}}, "fail"},
		{"checkrun skipped is pass", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"}}, "pass"},
		{"checkrun neutral is pass", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "NEUTRAL"}}, "pass"},
		{"checkrun stale is pass", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "STALE"}}, "pass"},
		{"checkrun completed no conclusion is pending", []checkEntry{{Typename: "CheckRun", Status: "COMPLETED", Conclusion: ""}}, "pending"},

		// --- single StatusContext ---
		{"statuscontext success", []checkEntry{{Typename: "StatusContext", State: "SUCCESS"}}, "pass"},
		{"statuscontext pending", []checkEntry{{Typename: "StatusContext", State: "PENDING"}}, "pending"},
		{"statuscontext expected", []checkEntry{{Typename: "StatusContext", State: "EXPECTED"}}, "pending"},
		{"statuscontext failure", []checkEntry{{Typename: "StatusContext", State: "FAILURE"}}, "fail"},
		{"statuscontext error", []checkEntry{{Typename: "StatusContext", State: "ERROR"}}, "fail"},

		// --- aggregate precedence ---
		{"any fail wins over pass+pending", []checkEntry{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "CheckRun", Status: "IN_PROGRESS"},
			{Typename: "StatusContext", State: "FAILURE"},
		}, "fail"},
		{"pending wins over pass (no fail)", []checkEntry{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "StatusContext", State: "PENDING"},
		}, "pending"},
		{"all pass", []checkEntry{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "StatusContext", State: "SUCCESS"},
		}, "pass"},
		{"mixed pass + skipped is still pass", []checkEntry{
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"},
			{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "NEUTRAL"},
		}, "pass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reduceChecks(tt.rollup); got != tt.want {
				t.Errorf("reduceChecks(%+v) = %q, want %q", tt.rollup, got, tt.want)
			}
		})
	}
}

// TestReduceChecksSupersededRuns is the regression guard for the real
// refresh-button bug: gh's statusCheckRollup returns EVERY historical run of a
// check (original, re-run, concurrency-cancelled), and a stale superseded
// FAILURE/CANCELLED entry must NOT paint the dot red. latestPerCheck keeps only
// the most recent run per check name; reduceChecks tallies that. These cases
// prove the dedup picks by run TIMESTAMP (not "any success wins"), matching
// GitHub's rollup state — verified live against seqeralabs/fusion #1461 (pass),
// #1459 (pass), #1352 (fail).
func TestReduceChecksSupersededRuns(t *testing.T) {
	const (
		tEarly = "2026-06-15T14:18:57Z"
		tLate  = "2026-06-16T03:35:03Z"
	)
	tests := []struct {
		name   string
		rollup []checkEntry
		want   string
	}{
		// fusion #1461: an old "Unit tests" FAILURE superseded by a re-run
		// SUCCESS → GitHub shows SUCCESS. The latest run wins.
		{"superseded failure ignored", []checkEntry{
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "FAILURE", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tLate},
		}, "pass"},

		// fusion #1459: every job has a concurrency-CANCELLED attempt plus a
		// later SUCCESS re-run → GitHub shows SUCCESS.
		{"all cancelled superseded by reruns", []checkEntry{
			{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "CANCELLED", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tLate},
			{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "CANCELLED", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tLate},
		}, "pass"},

		// fusion #1352: a genuinely failing check with only ONE run (not
		// superseded) → GitHub shows FAILURE. Must stay "fail".
		{"genuine single failure stays fail", []checkEntry{
			{Typename: "CheckRun", Name: "Analysis [golangci-lint]", Status: "COMPLETED", Conclusion: "FAILURE", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tEarly},
		}, "fail"},

		// Critical: proves we take the LATEST run, not "any success wins". A
		// SUCCESS superseded by a later FAILURE re-run → "fail".
		{"latest run is the failure", []checkEntry{
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "FAILURE", StartedAt: tLate},
		}, "fail"},

		// A completed SUCCESS superseded by a still-running re-run → "pending".
		{"pending rerun supersedes completed", []checkEntry{
			{Typename: "CheckRun", Name: "Unit tests", Status: "COMPLETED", Conclusion: "SUCCESS", StartedAt: tEarly},
			{Typename: "CheckRun", Name: "Unit tests", Status: "IN_PROGRESS", StartedAt: tLate},
		}, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reduceChecks(tt.rollup); got != tt.want {
				t.Errorf("reduceChecks(%+v) = %q, want %q", tt.rollup, got, tt.want)
			}
		})
	}
}

// TestReduceChecksSkippedHeavy is the explicit Pitfall-1 regression guard:
// a rollup dominated by SKIPPED entries (the cli/cli sample was ~30% SKIPPED)
// with the rest SUCCESS must reduce to "pass", never "fail".
func TestReduceChecksSkippedHeavy(t *testing.T) {
	var rollup []checkEntry
	for range 7 {
		rollup = append(rollup, checkEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SKIPPED"})
	}
	for range 3 {
		rollup = append(rollup, checkEntry{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"})
	}
	if got := reduceChecks(rollup); got != "pass" {
		t.Errorf("skipped-heavy rollup reduceChecks = %q, want pass (SKIPPED must never paint red)", got)
	}
}

// TestDedupeReviewed pins the server-side single-section rule (REVWD-03/D-14):
// any PR Number present in the awaiting-review list (`prs`, top precedence) is
// removed from the reviewed list before it ships, a non-overlapping reviewed PR
// is kept, the reviewed order is preserved, and an empty awaiting leaves the
// reviewed list intact. Pure function, no gh — so this is a fast table guard.
func TestDedupeReviewed(t *testing.T) {
	pr := func(n int) PRSummary { return PRSummary{Number: n} }
	numbers := func(in []PRSummary) []int {
		out := make([]int, 0, len(in))
		for _, p := range in {
			out = append(out, p.Number)
		}
		return out
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	tests := []struct {
		name     string
		awaiting []PRSummary
		reviewed []PRSummary
		want     []int
	}{
		{
			name:     "overlapping PR dropped from reviewed (top precedence)",
			awaiting: []PRSummary{pr(7)},
			reviewed: []PRSummary{pr(7), pr(3)},
			want:     []int{3},
		},
		{
			name:     "non-overlapping reviewed kept",
			awaiting: []PRSummary{pr(7)},
			reviewed: []PRSummary{pr(3), pr(5)},
			want:     []int{3, 5},
		},
		{
			name:     "reviewed order preserved after dedup",
			awaiting: []PRSummary{pr(2)},
			reviewed: []PRSummary{pr(9), pr(2), pr(4), pr(1)},
			want:     []int{9, 4, 1},
		},
		{
			name:     "empty awaiting leaves reviewed intact",
			awaiting: nil,
			reviewed: []PRSummary{pr(9), pr(4)},
			want:     []int{9, 4},
		},
		{
			name:     "empty reviewed stays empty",
			awaiting: []PRSummary{pr(1)},
			reviewed: nil,
			want:     []int{},
		},
		{
			name:     "all reviewed overlap → empty",
			awaiting: []PRSummary{pr(1), pr(2)},
			reviewed: []PRSummary{pr(1), pr(2)},
			want:     []int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numbers(dedupeReviewed(tt.awaiting, tt.reviewed))
			if !eq(got, tt.want) {
				t.Errorf("dedupeReviewed numbers = %v, want %v", got, tt.want)
			}
		})
	}
}

// exitErrOf spawns `sh -c "exit <code>"` to produce a REAL *exec.ExitError
// (os.ProcessState is not publicly constructible). The returned error's chain
// contains the *exec.ExitError that classifyGhListError unwraps via errors.As.
// Tests run on Linux/macOS where sh is present; if sh is absent the sub-test
// is skipped.
func exitErrOf(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
	if err == nil {
		t.Skipf("sh -c 'exit %d' exited 0 unexpectedly; cannot construct exit error", code)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("sh spawn did not yield *exec.ExitError: %T", err)
	}
	if exitErr.ExitCode() != code {
		t.Fatalf("sh exit code = %d, want %d", exitErr.ExitCode(), code)
	}
	return err
}

// TestClassifyGhListError is the table guard for the shared gh-failure
// classification (Pattern 3). It pins the THREE substrings + exit-4 path that
// map to "auth_required", plus the "everything else → error" fallback, and the
// no-exit-info case (a non-ExitError runErr still classifies via stderr
// alone). Identical sniffing for listPRs and listCompletedReviews — one path,
// no drift (Pitfall 4).
func TestClassifyGhListError(t *testing.T) {
	// Plain (non-ExitError) runErrs — classifyGhListError reads only stderr
	// for these (code stays -1).
	plainErr := errors.New("boom") //nolint:err113 // test-only

	tests := []struct {
		name   string
		runErr error
		stderr string
		want   string
	}{
		// auth_required paths
		{"exit code 4 alone", exitErrOf(t, 4), "", "auth_required"},
		{"exit code 4 with stderr", exitErrOf(t, 4), "could not authenticate: forbidden", "auth_required"},
		{"stderr 'gh auth login'", plainErr, "run `gh auth login` to retry", "auth_required"},
		{"stderr '401'", plainErr, "HTTP 401 Unauthorized", "auth_required"},
		{"stderr 'Bad credentials'", plainErr, "Bad credentials provided", "auth_required"},
		{"exit 1 + auth stderr", exitErrOf(t, 1), "Error: 401 expired token", "auth_required"},

		// error paths (non-auth failures)
		{"exit 1 generic", exitErrOf(t, 1), "HTTP 500 internal error", "error"},
		{"exit 2 generic", exitErrOf(t, 2), "connection refused", "error"},
		{"no exit info, no auth substring", plainErr, "panic: something else", "error"},
		{"empty stderr, exit 1", exitErrOf(t, 1), "", "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyGhListError(tt.runErr, tt.stderr); got != tt.want {
				t.Errorf("classifyGhListError(stderr=%q) = %q, want %q",
					tt.stderr, got, tt.want)
			}
		})
	}
}

// TestParseCompletedReviews pins the ok-path transformation of gh's
// `--json number,title,closedAt,url` output: closedAt→CompletedAt field map,
// most-recently-completed-first ordering (ISO 8601 lexical == chrono), and
// the malformed-JSON → error path. Pure-helper coverage so listCompletedReviews
// stays thin (the gh spawn itself is exercised via Service.GetMergedClosed in
// service_test.go).
func TestParseCompletedReviews(t *testing.T) {
	t.Run("maps closedAt to CompletedAt", func(t *testing.T) {
		stdout := []byte(`[{"number":42,"title":"ship","closedAt":"2026-07-25T14:30:00Z","url":"https://x/42"}]`)
		got, err := parseCompletedReviews(stdout)
		if err != nil {
			t.Fatalf("parseCompletedReviews: unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		p := got[0]
		if p.Number != 42 {
			t.Errorf("Number = %d, want 42", p.Number)
		}
		if p.Title != "ship" {
			t.Errorf("Title = %q, want ship", p.Title)
		}
		if p.CompletedAt != "2026-07-25T14:30:00Z" {
			t.Errorf("CompletedAt = %q, want 2026-07-25T14:30:00Z (mapped from closedAt)", p.CompletedAt)
		}
		if p.URL != "https://x/42" {
			t.Errorf("URL = %q, want https://x/42", p.URL)
		}
		// Provenance fields stay zero — GetMergedClosed leaves them for the
		// Plan 02 aggregation loop to annotate (D-06).
		if p.ProjectName != "" || p.ProjectID != 0 || p.Repo != "" {
			t.Errorf("provenance not zero: %+v (annotated by Plan 02)", p)
		}
	})

	t.Run("sorts by CompletedAt descending", func(t *testing.T) {
		// Feed out of order; expect newest-first.
		stdout := []byte(`[
			{"number":3,"title":"old","closedAt":"2026-06-01T00:00:00Z","url":""},
			{"number":1,"title":"new","closedAt":"2026-07-29T00:00:00Z","url":""},
			{"number":2,"title":"mid","closedAt":"2026-07-15T00:00:00Z","url":""}
		]`)
		got, err := parseCompletedReviews(stdout)
		if err != nil {
			t.Fatalf("parseCompletedReviews: unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		wantOrder := []int{1, 2, 3} // newest → mid → oldest
		for i, w := range wantOrder {
			if got[i].Number != w {
				t.Errorf("out[%d].Number = %d, want %d (desc CompletedAt)", i, got[i].Number, w)
			}
		}
	})

	t.Run("empty array yields empty slice", func(t *testing.T) {
		got, err := parseCompletedReviews([]byte(`[]`))
		if err != nil {
			t.Fatalf("parseCompletedReviews: unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0 for empty gh output", len(got))
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := parseCompletedReviews([]byte(`not json`))
		if err == nil {
			t.Fatal("expected error for malformed JSON, got nil")
		}
	})
}

// TestListCompletedReviews exercises the no_gh degrade path directly: when
// gh is absent (Available() false), listCompletedReviews returns state="no_gh"
// with an empty PR list and NEVER spawns a process. The ok/auth_required/error
// classification paths are covered by TestClassifyGhListError, and the ok-path
// transformation (closedAt→CompletedAt, sort) by TestParseCompletedReviews —
// together they cover the four listCompletedReviews states without coupling
// to live gh (the existing listPRs is tested the same way: through Service.Runner
// in service_test.go, not by shelling out).
func TestListCompletedReviews(t *testing.T) {
	t.Run("no_gh returns empty + no spawn", func(t *testing.T) {
		restore := SetAvailableForTest(false)
		defer restore()

		// Available() false means listCompletedReviews must short-circuit
		// BEFORE building or running any exec.Cmd. If it did spawn, gh would
		// either fail to start (not on PATH) or, worse, hit the network.
		prs, state, err := listCompletedReviews(context.Background(), "owner/name", "/tmp/anywhere")
		if err != nil {
			t.Fatalf("err = %v, want nil for classified degrade", err)
		}
		if state != "no_gh" {
			t.Fatalf("state = %q, want no_gh", state)
		}
		if prs != nil {
			t.Fatalf("prs = %v, want nil when gh is absent", prs)
		}
	})
}

package github

import "testing"

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

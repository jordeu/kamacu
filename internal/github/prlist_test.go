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

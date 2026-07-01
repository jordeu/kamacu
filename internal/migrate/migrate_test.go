package migrate

import "testing"

// TestGate locks the five-branch decision table (RESEARCH Pattern 1, D-13/D-04/
// D-11). These expectations are plan-locked (Task 1 <behavior>): if a case fails,
// the bug is in Gate (migrate.go), never these wants.
func TestGate(t *testing.T) {
	tests := []struct {
		name      string
		customDB  bool
		srcExists bool
		dstExists bool
		want      Decision
	}{
		{"custom db opts out", true, true, false, SkipCustom},
		{"custom db opts out even with both dirs present", true, true, true, SkipCustom},
		{"src only migrates", false, true, false, DoMigrate},
		{"dst only rolls forward", false, false, true, RollForward},
		{"neither dir is a fresh install", false, false, false, FreshInstall},
		{"both dirs present refuses boot (anomaly)", false, true, true, RefuseBoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Gate(tt.customDB, tt.srcExists, tt.dstExists); got != tt.want {
				t.Errorf("Gate(customDB=%v, src=%v, dst=%v) = %v, want %v",
					tt.customDB, tt.srcExists, tt.dstExists, got, tt.want)
			}
		})
	}
}

// TestDecisionString locks the stable human label for each Decision variant
// (used in slog lines, Task 1 <behavior>).
func TestDecisionString(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{SkipCustom, "SkipCustom"},
		{DoMigrate, "DoMigrate"},
		{RollForward, "RollForward"},
		{FreshInstall, "FreshInstall"},
		{RefuseBoot, "RefuseBoot"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String() = %q, want %q", int(tt.d), got, tt.want)
		}
	}
}

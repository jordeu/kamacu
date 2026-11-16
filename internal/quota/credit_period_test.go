package quota

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"kamacu/internal/store"
)

func persistentQuota(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return New(Config{DB: db, UserAgent: "test", CredentialsPath: "unused"})
}

func recordPeriod(t *testing.T, s *Service, account, date string, total float64, five, seven float64, reset string) *CreditPeriod {
	t.Helper()
	now, err := time.Parse(time.RFC3339, date)
	if err != nil {
		t.Fatal(err)
	}
	weekly := "2026-12-07T18:00:00Z"
	windows := []Window{{Key: "five_hour", Utilization: five, ResetsAt: &reset}, {Key: "seven_day", Utilization: seven, ResetsAt: &weekly}}
	extra := &ExtraUsage{Enabled: true, UsedCredits: &total, Currency: "USD"}
	if _, err := s.recordExtraUsage(context.Background(), account, extra, windows, now); err != nil {
		t.Fatal(err)
	}
	return extra.CreditPeriod
}

func expectPeriod(t *testing.T, got *CreditPeriod, spent float64, known bool) {
	t.Helper()
	if got == nil || got.SpentCredits != spent || got.BaselineKnown != known {
		t.Fatalf("period = %+v, want spent=%v known=%v", got, spent, known)
	}
}

func TestCreditPeriodLifecyclePersistsAndHandlesOverlappingLimits(t *testing.T) {
	s := persistentQuota(t)
	reset := "2026-11-16T18:00:00Z"
	record := func(date string, total, five, seven float64) *CreditPeriod {
		return recordPeriod(t, s, "a", "2026-11-16T"+date+"Z", total, five, seven, reset)
	}
	if got := record("10:00:00", 1000, 99, 90); got != nil {
		t.Fatal("available quota created paid period")
	}
	start := record("10:01:00", 1050, 100, 90)
	expectPeriod(t, start, 50, true)
	expectPeriod(t, record("10:02:00", 1125, 100, 90), 125, true)
	// A restart and an access-token rotation do not change the account-keyed ledger.
	s = New(Config{DB: s.db, UserAgent: "test", CredentialsPath: "unused"})
	expectPeriod(t, record("10:03:00", 1200, 100, 90), 200, true)
	expectPeriod(t, record("10:03:00", 1200, 100, 90), 200, true)
	// A second exhausted window does not reset or double-count the current period.
	expectPeriod(t, record("10:04:00", 1300, 100, 100), 300, true)
	ongoing := record("10:05:00", 1350, 0, 100)
	expectPeriod(t, ongoing, 350, true)
	if !ongoing.StartedAt.Equal(start.StartedAt) {
		t.Fatal("overlapping limit restarted period")
	}
	if got := record("10:06:00", 1400, 0, 80); got != nil {
		t.Fatal("replenished quota did not end period")
	}
	// A later limit after a polling gap has no trustworthy crossing baseline.
	expectPeriod(t, record("10:10:00", 1600, 100, 90), 0, false)
	expectPeriod(t, record("10:11:00", 1650, 100, 90), 50, false)
	// A different reset boundary is a different quota episode, even if both are 100%.
	reset = "2026-11-16T19:00:00Z"
	expectPeriod(t, record("10:12:00", 1700, 100, 90), 0, false)
	expectPeriod(t, record("10:13:00", 1720, 100, 90), 20, false)
	expectPeriod(t, record("10:14:00", 1710, 100, 90), 20, false)
	expectPeriod(t, record("10:15:00", 1720, 100, 90), 20, false)
	expectPeriod(t, recordPeriod(t, s, "b", "2026-11-16T10:16:00Z", 9000, 100, 90, reset), 0, false)
	expectPeriod(t, record("10:16:00", 1750, 100, 90), 50, false)
}

func TestCreditPeriodMonthRolloverIsPartial(t *testing.T) {
	s := persistentQuota(t)
	reset := "2026-12-01T03:00:00Z"
	recordPeriod(t, s, "a", "2026-11-30T23:58:00Z", 1000, 99, 90, reset)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-30T23:59:00Z", 1050, 100, 90, reset), 50, true)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-12-01T00:00:00Z", 25, 100, 90, reset), 75, false)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-12-01T00:01:00Z", 30, 100, 90, reset), 80, false)
}

func TestCreditPeriodUnknownBaselineAndDisabledCredits(t *testing.T) {
	s := persistentQuota(t)
	reset := "2026-11-16T18:00:00Z"
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-16T10:00:00Z", 1500, 100, 90, reset), 0, false)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-16T10:01:00Z", 1550, 100, 90, reset), 50, false)
	// Disabled usage with null spending clears the persisted episode.
	now, _ := time.Parse(time.RFC3339, "2026-11-16T10:02:00Z")
	if _, err := s.recordExtraUsage(context.Background(), "a", &ExtraUsage{Currency: "USD"}, nil, now); err != nil {
		t.Fatal(err)
	}
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-16T10:03:00Z", 1700, 100, 90, reset), 0, false)
}

func TestCreditPeriodExpiredWindowAndUnknownResetGap(t *testing.T) {
	s := persistentQuota(t)
	// A 100% reading whose reset boundary has passed is not current exhaustion.
	if got := recordPeriod(t, s, "a", "2026-11-16T10:00:00Z", 1000, 100, 90, "2026-11-16T09:00:00Z"); got != nil {
		t.Fatal("expired quota started a period")
	}
	total := 1000.0
	sample := func(date string, enabled bool) *CreditPeriod {
		t.Helper()
		now, _ := time.Parse(time.RFC3339, date)
		extra := &ExtraUsage{Enabled: enabled, UsedCredits: &total, Currency: "USD"}
		if _, err := s.recordExtraUsage(context.Background(), "b", extra, []Window{{Key: "five_hour", Utilization: 100}}, now); err != nil {
			t.Fatal(err)
		}
		return extra.CreditPeriod
	}
	expectPeriod(t, sample("2026-11-16T10:00:00Z", true), 0, false)
	total = 1100
	expectPeriod(t, sample("2026-11-16T10:01:00Z", true), 100, false)
	total = 1200
	expectPeriod(t, sample("2026-11-16T11:00:00Z", true), 0, false)
	if got := sample("2026-11-16T11:01:00Z", false); got != nil {
		t.Fatal("disabled usage retained paid period")
	}
	total = 1300
	expectPeriod(t, sample("2026-11-16T11:02:00Z", true), 0, false)
}

func TestCreditPeriodCounterCorrectionAtCrossing(t *testing.T) {
	s := persistentQuota(t)
	reset := "2026-11-16T18:00:00Z"
	recordPeriod(t, s, "a", "2026-11-16T10:00:00Z", 1000, 99, 90, reset)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-16T10:01:00Z", 500, 100, 90, reset), 0, false)
	expectPeriod(t, recordPeriod(t, s, "a", "2026-11-16T10:02:00Z", 520, 100, 90, reset), 20, false)
}

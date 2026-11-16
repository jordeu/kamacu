package quota

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kamacu/internal/store"
)

func TestNormalizeExtraUsage(t *testing.T) {
	for _, body := range []string{`{}`, `{"extra_usage":null}`, `{"extra_usage":"bad"}`} {
		if got := normalizeExtraUsage([]byte(body)); got != nil {
			t.Fatalf("%s: unexpected %+v", body, got)
		}
	}
	got := normalizeExtraUsage([]byte(`{"extra_usage":{"is_enabled":true,"used_credits":1234.5,"monthly_limit":5000,"utilization":24.69}}`))
	if got == nil || !got.Enabled || *got.UsedCredits != 1234.5 || *got.MonthlyLimit != 5000 || got.Currency != "USD" {
		t.Fatalf("incorrect normalization: %+v", got)
	}
	disabled := normalizeExtraUsage([]byte(`{"extra_usage":{"is_enabled":false,"used_credits":null,"monthly_limit":null}}`))
	if disabled == nil || disabled.Enabled || disabled.UsedCredits != nil || disabled.MonthlyLimit != nil {
		t.Fatalf("null fields fabricated: %+v", disabled)
	}
}

func TestDailyHistoryPersistenceAndBoundaries(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := New(Config{DB: db, UserAgent: "test", CredentialsPath: "unused"})
	record := func(account, date string, total float64) []DailySpend {
		t.Helper()
		now, err := time.Parse(time.RFC3339, date)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.recordExtraUsage(context.Background(), account, &ExtraUsage{UsedCredits: &total, Currency: "USD"}, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := record("a", "2026-10-29T12:00:00Z", 1000)
	if len(got) != 1 || got[0].Spent != 0 || !got[0].Partial {
		t.Fatalf("first sample should establish baseline: %+v", got)
	}
	got = record("a", "2026-10-29T13:00:00Z", 1125.5)
	if got[0].Spent != 125.5 {
		t.Fatalf("same-day increase: %+v", got)
	}
	got = record("a", "2026-10-29T14:00:00Z", 1125.5)
	if got[0].Spent != 125.5 {
		t.Fatalf("duplicate sample counted twice: %+v", got)
	}
	// A new service has no in-memory baseline; all accounting must survive restart.
	s = New(Config{DB: db, UserAgent: "test", CredentialsPath: "unused"})
	got = record("a", "2026-10-30T12:00:00Z", 1200)
	if len(got) != 2 || got[0].Spent != 74.5 || got[0].Partial {
		t.Fatalf("next-day persisted baseline: %+v", got)
	}
	got = record("a", "2026-10-30T13:00:00Z", 1100)
	got = record("a", "2026-10-30T14:00:00Z", 1200)
	if got[0].Spent != 74.5 || !got[0].Partial {
		t.Fatalf("correction double counted: %+v", got)
	}
	got = record("a", "2026-11-01T12:00:00Z", 250)
	if got[0].Spent != 250 || !got[0].Partial {
		t.Fatalf("month reset and tracking gap: %+v", got)
	}
	got = record("b", "2026-11-01T12:00:00Z", 9999)
	if len(got) != 1 || got[0].Spent != 0 {
		t.Fatalf("account histories mixed: %+v", got)
	}
	got = record("a", "2026-11-04T12:00:00Z", 400)
	if got[0].Spent != 150 || !got[0].Partial {
		t.Fatalf("gap should be explicit: %+v", got)
	}
}

func TestServiceExtraUsageHistoryAndTokenRefresh(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	total := "1000"
	fail := false
	s := newTestService(t, validCreds(fakeToken()), func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"five_hour":{"utilization":100},"extra_usage":{"is_enabled":true,"used_credits":` + total + `,"monthly_limit":5000}}`))
	})
	s.db = db
	s.accountPath = filepath.Join(t.TempDir(), "claude.json")
	if err := os.WriteFile(s.accountPath, []byte(`{"oauthAccount":{"accountUuid":"account-a","organizationUuid":"org"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	advance := fakeClock(s)
	res := s.Get(context.Background(), false)
	if res.ExtraUsage == nil || !res.ExtraUsage.HistoryAvailable || *res.ExtraUsage.UsedCredits != 1000 {
		t.Fatalf("extra usage missing: %+v", res)
	}
	total = "1500"
	if err := os.WriteFile(s.credsPath, []byte(validCreds("rotated-token")), 0600); err != nil {
		t.Fatal(err)
	}
	advance(time.Minute)
	res = s.Get(context.Background(), false)
	if res.ExtraUsage.Daily[0].Spent != 500 {
		t.Fatalf("token refresh lost history: %+v", res.ExtraUsage)
	}
	fail = true
	advance(time.Minute)
	res = s.Get(context.Background(), false)
	if !res.Stale || res.ExtraUsage == nil {
		t.Fatal("extra usage should remain stale after a failed fetch")
	}
	advance(time.Minute)
	s.Get(context.Background(), false)
	advance(time.Minute)
	res = s.Get(context.Background(), false)
	if res.ExtraUsage != nil {
		t.Fatal("extra usage must drop with expired cache")
	}
}

func TestExtraOnlyResponseCacheAndStale(t *testing.T) {
	hits := 0
	fail := false
	s := newTestService(t, validCreds(fakeToken()), func(w http.ResponseWriter, r *http.Request) {
		hits++
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"extra_usage":{"is_enabled":true,"used_credits":250,"monthly_limit":null}}`))
	})
	advance := fakeClock(s)
	res := s.Get(context.Background(), false)
	if res.ExtraUsage == nil || res.FetchedAt == nil || res.ExtraUsage.HistoryAvailable {
		t.Fatalf("extra-only response: %+v", res)
	}
	advance(30 * time.Second)
	s.Get(context.Background(), false)
	if hits != 1 {
		t.Fatalf("extra-only response bypassed cache: %d requests", hits)
	}
	fail = true
	advance(time.Minute)
	res = s.Get(context.Background(), false)
	if !res.Stale || res.ExtraUsage == nil {
		t.Fatalf("extra usage must be marked stale on failure: %+v", res)
	}
	for i := 0; i < 2; i++ {
		advance(time.Minute)
		res = s.Get(context.Background(), false)
	}
	if res.ExtraUsage != nil || res.FetchedAt != nil {
		t.Fatalf("extra usage should drop after repeated failures: %+v", res)
	}
}

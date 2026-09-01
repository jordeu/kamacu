package api

import (
	"reflect"
	"testing"
	"time"
)

func TestParseActivityTime(t *testing.T) {
	t.Run("millisecond precision", func(t *testing.T) {
		got, ok := parseActivityTime("2026-07-25T14:30:00.123Z")
		if !ok {
			t.Fatalf("expected ok=true for ms-precision, got false")
		}
		want := time.Date(2026, time.July, 25, 14, 30, 0, 123000000, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ms-precision instant: got %v, want %v", got.UTC(), want)
		}
	})
	t.Run("second precision", func(t *testing.T) {
		got, ok := parseActivityTime("2026-07-25T14:30:00Z")
		if !ok {
			t.Fatalf("expected ok=true for second-precision, got false")
		}
		want := time.Date(2026, time.July, 25, 14, 30, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("second-precision instant: got %v, want %v", got.UTC(), want)
		}
	})
	t.Run("ms zero-fraction equals second precision (same instant)", func(t *testing.T) {
		ms, ok1 := parseActivityTime("2026-07-25T14:30:00.000Z")
		if !ok1 {
			t.Fatalf("ms .000 parse failed")
		}
		sec, ok2 := parseActivityTime("2026-07-25T14:30:00Z")
		if !ok2 {
			t.Fatalf("second-precision parse failed")
		}
		if !ms.Equal(sec) {
			t.Errorf("ms(.000) and second-precision should be Equal: ms=%v sec=%v", ms, sec)
		}
	})
	t.Run("empty returns false and zero time", func(t *testing.T) {
		got, ok := parseActivityTime("")
		if ok {
			t.Errorf("expected ok=false for empty, got true (%v)", got)
		}
		if !got.IsZero() {
			t.Errorf("expected zero time for empty, got %v", got)
		}
	})
	t.Run("junk returns false and never panics", func(t *testing.T) {
		got, ok := parseActivityTime("not-a-date")
		if ok {
			t.Errorf("expected ok=false for junk, got true (%v)", got)
		}
		if !got.IsZero() {
			t.Errorf("expected zero time for junk, got %v", got)
		}
	})
}

func TestParseScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantKind string
		wantID   int64
		wantErr  bool
	}{
		{"global literal", "global", "global", 0, false},
		{"empty defaults to global", "", "global", 0, false},
		{"workspace N", "workspace:42", "workspace", 42, false},
		{"project N", "project:7", "project", 7, false},
		{"workspace missing N", "workspace:", "", 0, true},
		{"workspace non-numeric N", "workspace:abc", "", 0, true},
		{"unknown prefix", "bogus", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, id, err := parseScope(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected non-nil error for %q, got nil (kind=%q id=%d)", tc.raw, kind, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for %q, got %v", tc.raw, err)
			}
			if kind != tc.wantKind {
				t.Errorf("kind: got %q, want %q", kind, tc.wantKind)
			}
			if id != tc.wantID {
				t.Errorf("id: got %d, want %d", id, tc.wantID)
			}
		})
	}
}

func TestParseWindow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{"week literal", "week", 7 * 24 * time.Hour, false},
		{"empty defaults to week", "", 7 * 24 * time.Hour, false},
		{"month", "month", 30 * 24 * time.Hour, false},
		{"junk", "decade", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseWindow(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected non-nil error for %q, got nil (%v)", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for %q, got %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComputeTimeStat(t *testing.T) {
	t.Run("empty returns zero timeStat", func(t *testing.T) {
		got := computeTimeStat(nil)
		if !reflect.DeepEqual(got, timeStat{}) {
			t.Errorf("empty: got %+v, want zero timeStat", got)
		}
	})
	t.Run("odd N median is the middle element", func(t *testing.T) {
		got := computeTimeStat([]time.Duration{3 * time.Hour, 1 * time.Hour, 2 * time.Hour})
		want := timeStat{N: 3, Min: 3600, Max: 10800, Median: 7200}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("odd N: got %+v, want %+v", got, want)
		}
	})
	t.Run("even N median averages the two middle (Pitfall 6)", func(t *testing.T) {
		got := computeTimeStat([]time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour})
		want := timeStat{N: 4, Min: 3600, Max: 14400, Median: 9000}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("even N: got %+v, want %+v", got, want)
		}
	})
	t.Run("truncation to int64 seconds", func(t *testing.T) {
		got := computeTimeStat([]time.Duration{1500 * time.Millisecond, 2500 * time.Millisecond})
		want := timeStat{N: 2, Min: 1, Max: 2, Median: 2}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("truncation: got %+v, want %+v", got, want)
		}
	})
}

func TestBuildDurationSlices(t *testing.T) {
	const (
		atProg   = "2026-07-25T10:00:00.000Z"
		atReview = "2026-07-25T11:00:00.000Z"
		atDone   = "2026-07-25T12:00:00.000Z"
	)
	twoHour := 2 * time.Hour
	oneHour := time.Hour

	t.Run("full In Progress -> In Review -> Done", func(t *testing.T) {
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: atProg,
			InReviewAt:   atReview,
			DoneAt:       atDone,
		}})
		if !reflect.DeepEqual(cycle, []time.Duration{twoHour}) {
			t.Errorf("cycle: got %v, want [2h]", cycle)
		}
		if !reflect.DeepEqual(dir, []time.Duration{oneHour}) {
			t.Errorf("dwellInReview: got %v, want [1h]", dir)
		}
		if !reflect.DeepEqual(dip, []time.Duration{oneHour}) {
			t.Errorf("dwellInProgress (inReview-inProgress): got %v, want [1h]", dip)
		}
	})

	t.Run("skipped In Review falls back to done-inProgress", func(t *testing.T) {
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: atProg,
			DoneAt:       atDone,
		}})
		if !reflect.DeepEqual(cycle, []time.Duration{twoHour}) {
			t.Errorf("cycle: got %v, want [2h]", cycle)
		}
		if len(dir) != 0 {
			t.Errorf("dwellInReview: got %v, want empty", dir)
		}
		if !reflect.DeepEqual(dip, []time.Duration{twoHour}) {
			t.Errorf("dwellInProgress fallback (done-inProgress): got %v, want [2h]", dip)
		}
	})

	t.Run("NULL in_review_at excludes dwellInReview only", func(t *testing.T) {
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: atProg,
			InReviewAt:   "",
			DoneAt:       atDone,
		}})
		if !reflect.DeepEqual(cycle, []time.Duration{twoHour}) {
			t.Errorf("cycle: got %v, want [2h]", cycle)
		}
		if len(dir) != 0 {
			t.Errorf("dwellInReview: got %v, want empty", dir)
		}
		if !reflect.DeepEqual(dip, []time.Duration{twoHour}) {
			t.Errorf("dwellInProgress fallback: got %v, want [2h]", dip)
		}
	})

	t.Run("NULL in_progress_at excludes cycle and dwellInProgress", func(t *testing.T) {
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: "",
			InReviewAt:   atReview,
			DoneAt:       atDone,
		}})
		if len(cycle) != 0 {
			t.Errorf("cycle: got %v, want empty", cycle)
		}
		if !reflect.DeepEqual(dir, []time.Duration{oneHour}) {
			t.Errorf("dwellInReview: got %v, want [1h]", dir)
		}
		if len(dip) != 0 {
			t.Errorf("dwellInProgress: got %v, want empty (no inProgress)", dip)
		}
	})

	t.Run("NULL done_at excludes all slices", func(t *testing.T) {
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: atProg,
			InReviewAt:   atReview,
			DoneAt:       "",
		}})
		if len(cycle) != 0 || len(dip) != 0 || len(dir) != 0 {
			t.Errorf("no done_at: expected all empty, got cycle=%v dip=%v dir=%v", cycle, dip, dir)
		}
	})

	t.Run("unparseable *_at skipped not panicked (Pitfall 7)", func(t *testing.T) {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on unparseable done: %v", r)
				}
			}()
			cycle, dip, dir := buildDurationSlices([]activityTask{{
				InProgressAt: atProg,
				InReviewAt:   atReview,
				DoneAt:       "garbage",
			}})
			if len(cycle) != 0 || len(dip) != 0 || len(dir) != 0 {
				t.Errorf("unparseable done: expected all empty, got cycle=%v dip=%v dir=%v", cycle, dip, dir)
			}
		}()
		cycle, dip, dir := buildDurationSlices([]activityTask{{
			InProgressAt: "garbage",
			InReviewAt:   atReview,
			DoneAt:       atDone,
		}})
		if len(cycle) != 0 {
			t.Errorf("unparseable inProgress: cycle should be empty, got %v", cycle)
		}
		if !reflect.DeepEqual(dir, []time.Duration{oneHour}) {
			t.Errorf("unparseable inProgress: dwellInReview should be [1h], got %v", dir)
		}
		if len(dip) != 0 {
			t.Errorf("unparseable inProgress: dwellInProgress should be empty, got %v", dip)
		}
	})
}

func TestRollupReviewState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []string
		want   string
	}{
		{"empty -> ok", nil, "ok"},
		{"all ok -> ok", []string{"ok", "ok"}, "ok"},
		{"mixed ok + degraded -> partial", []string{"ok", "no_gh"}, "partial"},
		{"ok + error -> partial", []string{"ok", "error"}, "partial"},
		{"none ok, priority picks error", []string{"error", "auth_required", "no_gh"}, "error"},
		{"none ok, auth_required beats no_gh", []string{"no_gh", "auth_required"}, "auth_required"},
		{"none ok, only no_gh", []string{"no_gh", "no_gh"}, "no_gh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rollupReviewState(tc.states); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

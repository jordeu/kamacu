package reaper

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// spyStopper records StopAllForTask calls and reports a configurable set of
// task IDs as having a running session. It satisfies SessionStopper without
// importing the real *session.Manager (no PTYs in unit tests).
type spyStopper struct {
	running map[int64]bool // task IDs with at least one running session
	reaped  []int64        // task IDs StopAllForTask was called for
}

func (s *spyStopper) StopAllForTask(taskID int64) {
	s.reaped = append(s.reaped, taskID)
}

func (s *spyStopper) ListByTask(taskID int64) []session.Info {
	if s.running[taskID] {
		// One running session is enough for the reaper's "anyRunning" check.
		return []session.Info{{TaskID: taskID, Status: session.StatusRunning}}
	}
	// An exited session must NOT count as worth reaping.
	return []session.Info{{TaskID: taskID, Status: session.StatusExited}}
}

// newReaperDB opens a migrated throwaway DB and seeds one project so tasks have
// a valid project_id.
func newReaperDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "reaper.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name, repo_path) VALUES (1, 'p', '/tmp/p')`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// isoAt formats a time the same way tasks.done_at is stored (strftime
// '%Y-%m-%dT%H:%M:%fZ' = millisecond ISO-8601, UTC).
func isoAt(ts time.Time) string {
	return ts.UTC().Format("2006-01-02T15:04:05.000Z")
}

// seedTask inserts a task with an explicit status and (optional) done_at.
// A nil doneAt leaves done_at NULL.
func seedTask(t *testing.T, db *sql.DB, id int64, status string, doneAt *time.Time) {
	t.Helper()
	var da any
	if doneAt != nil {
		da = isoAt(*doneAt)
	}
	if _, err := db.Exec(
		`INSERT INTO tasks (id, project_id, title, status, position, done_at)
		 VALUES (?, 1, ?, ?, ?, ?)`,
		id, "task", status, float64(id), da); err != nil {
		t.Fatalf("seed task %d: %v", id, err)
	}
}

// runReapOnce builds a reaper pinned to `now` with the given TTL setting and
// running-session set, runs one tick, and returns the sorted reaped IDs.
func runReapOnce(t *testing.T, db *sql.DB, ttl string, now time.Time, running map[int64]bool) []int64 {
	t.Helper()
	if err := settings.Set(db, settings.KeyDoneSessionTTL, ttl); err != nil {
		t.Fatalf("set done_session_ttl=%q: %v", ttl, err)
	}
	spy := &spyStopper{running: running}
	r := New(db, spy)
	r.now = func() time.Time { return now }
	r.reapOnce(context.Background())
	sort.Slice(spy.reaped, func(i, j int) bool { return spy.reaped[i] < spy.reaped[j] })
	return spy.reaped
}

func contains(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestReapExpiredRunningDoneTask: a Done task whose done_at is older than the
// TTL and which has a running session gets StopAllForTask called.
func TestReapExpiredRunningDoneTask(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour) // 48h ago, TTL 24h -> expired
	seedTask(t, db, 1, "done", &old)

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: true})
	if !contains(reaped, 1) {
		t.Fatalf("expired+running Done task 1 was not reaped: %v", reaped)
	}
}

// TestWithinTTLNotReaped: a Done task within the TTL is left alone.
func TestWithinTTLNotReaped(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour) // 1h ago, TTL 24h -> not expired
	seedTask(t, db, 1, "done", &recent)

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: true})
	if contains(reaped, 1) {
		t.Fatalf("within-TTL Done task 1 was reaped: %v", reaped)
	}
}

// TestLeftDoneNotReaped: a task that LEFT Done (status != done) is never reaped,
// even with an ancient done_at — the status gate cancels reaping (D-90).
func TestLeftDoneNotReaped(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	ancient := now.Add(-1000 * time.Hour)
	seedTask(t, db, 1, "in_review", &ancient)

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: true})
	if contains(reaped, 1) {
		t.Fatalf("task that left Done (in_review) was reaped: %v", reaped)
	}
}

// TestDisabledNeverReaps: never/0/empty disables reaping (REAP-01/D-91).
func TestDisabledNeverReaps(t *testing.T) {
	for _, ttl := range []string{"never", "0", ""} {
		db := newReaperDB(t)
		now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
		ancient := now.Add(-1000 * time.Hour)
		seedTask(t, db, 1, "done", &ancient)

		reaped := runReapOnce(t, db, ttl, now, map[int64]bool{1: true})
		if len(reaped) != 0 {
			t.Fatalf("ttl=%q disabled but reaped %v", ttl, reaped)
		}
	}
}

// TestNullDoneAtNotReaped: a Done task with NULL done_at has no clock and is
// never reaped (defensive — migration backfills existing Done rows).
func TestNullDoneAtNotReaped(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	seedTask(t, db, 1, "done", nil) // NULL done_at

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: true})
	if contains(reaped, 1) {
		t.Fatalf("Done task with NULL done_at was reaped: %v", reaped)
	}
}

// TestNoRunningSessionsSkipped: an expired Done task with NO running sessions is
// skipped — no pointless StopAllForTask call / log noise.
func TestNoRunningSessionsSkipped(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	seedTask(t, db, 1, "done", &old)

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: false})
	if contains(reaped, 1) {
		t.Fatalf("expired Done task with no running sessions was reaped: %v", reaped)
	}
}

// TestReapsOnlyExpiredAmongMany: with a mix, only the expired+running Done tasks
// are reaped — the query gate and the running check compose correctly.
func TestReapsOnlyExpiredAmongMany(t *testing.T) {
	db := newReaperDB(t)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	seedTask(t, db, 1, "done", &old)        // expired + running -> reaped
	seedTask(t, db, 2, "done", &recent)     // within TTL -> not reaped
	seedTask(t, db, 3, "in_review", &old)   // left Done -> not reaped
	seedTask(t, db, 4, "done", nil)         // NULL done_at -> not reaped
	seedTask(t, db, 5, "done", &old)        // expired but no running session -> skipped

	reaped := runReapOnce(t, db, "24h", now, map[int64]bool{1: true, 5: false})
	if len(reaped) != 1 || reaped[0] != 1 {
		t.Fatalf("expected only task 1 reaped, got %v", reaped)
	}
}

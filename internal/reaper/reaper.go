// Package reaper kills the sessions of tasks left in Done past a configurable
// TTL (REAP-01). It is the codebase's first background goroutine: a ticker that,
// each tick, finds tasks with status='done' whose done_at is older than the
// done_session_ttl window and stops EVERY running session of the task (bash,
// tmux, AND the agent) via the session Manager.
//
// Design decisions it honors:
//   - D-90: the reaper keys on tasks.done_at (set on the move-to-Done) and is
//     GATED on status='done'. A task that left Done (status != done) is never
//     reaped regardless of done_at — leaving Done cancels reaping.
//   - D-91: the TTL comes from settings.done_session_ttl, read at use, and the
//     disable semantics (never / 0 / empty / non-positive) live entirely in
//     settings.ParseDoneSessionTTL — the single source of truth shared with the
//     save-time validator, so reaper and validator can never disagree.
//   - D-95: reaping is silent. There is no notification/badge; the server logs
//     each reap via log/slog.
//   - D-96: StopAllForTask kills the PTYs but NEVER deletes tasks.claude_session_id
//     or the transcript (the session package holds no DB and Stop() touches only
//     the process), so the agent keeps its normal resumable ghost — fully
//     reversible. This is automatic; the reaper adds no extra code for it.
//   - D-87: the reaper NEVER touches worktrees — it only calls StopAllForTask.
//
// There is no graceful-shutdown machinery in the app (process death is the
// stop), so the goroutine needs no shutdown channel beyond the context it is
// given; a plain ticker living for the process lifetime is correct.
package reaper

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"kangent/internal/session"
	"kangent/internal/settings"
)

// defaultTick is how often the reaper scans for expired Done tasks. The window
// is coarse on purpose (research discretion 5-15 min): a Done-TTL measured in
// hours does not need sub-minute precision, and a sparse tick keeps the idle
// cost near zero.
const defaultTick = 10 * time.Minute

// SessionStopper is the slice of the session Manager the reaper needs. Defined
// locally (rather than importing *session.Manager directly into the signature)
// so unit tests can substitute a spy without spawning real PTYs. *session.Manager
// satisfies it.
type SessionStopper interface {
	// StopAllForTask kills every running session of the task (bash + tmux +
	// agent) and blocks until they exit. It never touches claude_session_id.
	StopAllForTask(taskID int64)
	// ListByTask reports the task's sessions so the reaper can skip tasks with
	// nothing running (avoid pointless kills and log noise).
	ListByTask(taskID int64) []session.Info
}

// Reaper scans for expired Done tasks and stops their sessions.
type Reaper struct {
	db   *sql.DB
	mgr  SessionStopper
	tick time.Duration
	// now is the clock seam (mirrors quota.Config.Now): defaults to time.Now,
	// overridden in tests to make TTL expiry deterministic without sleeps.
	now func() time.Time
}

// New returns a Reaper with the default 10-minute tick and the wall clock.
func New(db *sql.DB, mgr SessionStopper) *Reaper {
	return &Reaper{db: db, mgr: mgr, tick: defaultTick, now: time.Now}
}

// Run drives reapOnce on the ticker until ctx is cancelled (or, in practice,
// until the process dies — there is no graceful shutdown). It reaps once
// immediately at start so a restart promptly reaps tasks that were already
// long-Done, rather than waiting a full tick.
func (r *Reaper) Run(ctx context.Context) {
	r.reapOnce(ctx)
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reapOnce(ctx)
		}
	}
}

// reapOnce is one scan: read the TTL, find expired Done tasks with running
// sessions, and stop them. It never returns an error and never panics on a bad
// setting — a degraded reaper is always preferable to a crashed goroutine.
func (r *Reaper) reapOnce(ctx context.Context) {
	raw, err := settings.Get(r.db, settings.KeyDoneSessionTTL)
	if err != nil {
		slog.Warn("reaper: reading done_session_ttl", "error", err)
		return
	}
	ttl, disabled, err := settings.ParseDoneSessionTTL(raw)
	if err != nil {
		// A saved value is validated, so this should be unreachable; degrade to
		// disabled rather than crash the goroutine (Pitfall: never let a bad
		// hand-edited row kill reaping for the whole process).
		slog.Warn("reaper: invalid done_session_ttl, reaping disabled this tick", "value", raw, "error", err)
		return
	}
	if disabled {
		return // never / 0 / empty / non-positive -> reaping off (D-91)
	}

	// done_at is stored as millisecond ISO-8601 (strftime '%Y-%m-%dT%H:%M:%fZ'),
	// which sorts chronologically as a string, so a lexical `<` against a cutoff
	// formatted the same way is a correct time comparison — no per-row parsing.
	cutoff := r.now().UTC().Add(-ttl).Format("2006-01-02T15:04:05.000Z")

	// The status='done' gate is D-90's cancellation (a task that left Done is
	// excluded); done_at IS NOT NULL guards the no-clock case.
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM tasks WHERE status = 'done' AND done_at IS NOT NULL AND done_at < ?`,
		cutoff)
	if err != nil {
		slog.Warn("reaper: querying expired Done tasks", "error", err)
		return
	}
	defer rows.Close()

	var expired []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			slog.Warn("reaper: scanning expired task id", "error", err)
			continue
		}
		expired = append(expired, id)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("reaper: iterating expired tasks", "error", err)
		return
	}

	for _, id := range expired {
		// Skip tasks with nothing running — no PTYs to reclaim, no log noise.
		anyRunning := false
		for _, info := range r.mgr.ListByTask(id) {
			if info.Status == session.StatusRunning {
				anyRunning = true
				break
			}
		}
		if !anyRunning {
			continue
		}
		// Kills bash + tmux + agent PTYs; KEEPS claude_session_id + transcript
		// (D-96 resumable ghost — automatic). NEVER touches worktrees (D-87).
		r.mgr.StopAllForTask(id)
		slog.Info("reaped Done-TTL sessions", "task", id, "ttl", ttl)
	}
}

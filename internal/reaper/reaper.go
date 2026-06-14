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
	"fmt"
	"log/slog"
	"time"

	"kangent/internal/api"
	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
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

// PRStateGetter is the slice of the github service the reaper's PR pass needs
// (D-02). Defined LOCALLY (exactly like SessionStopper above) so a test injects
// a spy and the real *github.Service satisfies it. Returns "OPEN"|"CLOSED"|
// "MERGED" or an error — a gh failure is warn-and-skip, never a crash (D-03).
type PRStateGetter interface {
	PRState(ctx context.Context, repo string, n int) (string, error)
}

// Reaper scans for expired Done tasks and stops their sessions, and (when the
// PR pass is wired via NewWithPR) reconciles merged/closed PR worktrees.
type Reaper struct {
	db  *sql.DB
	mgr SessionStopper
	// sessionMgr is the concrete manager the shared CleanupWorktreeGated helper
	// requires (it takes *session.Manager, not the SessionStopper interface).
	// New (Done-TTL only) leaves it nil; NewWithPR sets it to the same manager
	// as mgr. Kept SEPARATE from mgr so the existing Done-TTL reaper_test.go can
	// still call New(db, spy) with a spy SessionStopper unchanged.
	sessionMgr *session.Manager
	// wt, tmuxClient, and pr are the PR-pass deps (nil/zero for Done-TTL-only
	// construction). pr == nil is the switch that disables reconcilePRsOnce.
	wt         *worktree.Service
	tmuxClient tmux.Client
	pr         PRStateGetter
	tick       time.Duration
	// now is the clock seam (mirrors quota.Config.Now): defaults to time.Now,
	// overridden in tests to make TTL expiry deterministic without sleeps.
	now func() time.Time
}

// New returns a Done-TTL-only Reaper with the default 10-minute tick and the
// wall clock. The PR reconcile pass is NOT wired (pr == nil), so Run skips it —
// this keeps the existing Done-TTL tests' New(db, spy) calls compiling and
// behaving unchanged. Use NewWithPR for the full production reaper.
func New(db *sql.DB, mgr SessionStopper) *Reaper {
	return &Reaper{db: db, mgr: mgr, tick: defaultTick, now: time.Now}
}

// NewWithPR returns the full production Reaper: both the Done-TTL pass and the
// merged/closed PR reconcile pass (13-02). mgr is stored as BOTH the
// SessionStopper interface (Done-TTL pass) and the concrete *session.Manager
// (the shared CleanupWorktreeGated helper). wt/tmuxClient/pr drive the PR pass.
func NewWithPR(db *sql.DB, mgr *session.Manager, wt *worktree.Service, tmuxClient tmux.Client, pr PRStateGetter) *Reaper {
	return &Reaper{
		db:         db,
		mgr:        mgr,
		sessionMgr: mgr,
		wt:         wt,
		tmuxClient: tmuxClient,
		pr:         pr,
		tick:       defaultTick,
		now:        time.Now,
	}
}

// Run drives reapOnce (and, when wired, reconcilePRsOnce) on the ticker until
// ctx is cancelled (or, in practice, until the process dies — there is no
// graceful shutdown). It runs once immediately at start so a restart promptly
// reaps tasks that were already long-Done and reconciles already-merged PRs,
// rather than waiting a full tick.
func (r *Reaper) Run(ctx context.Context) {
	r.reapOnce(ctx)
	r.reconcilePRsOnce(ctx)
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reapOnce(ctx)
			r.reconcilePRsOnce(ctx)
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

// reconcilePRsOnce is the reaper's SECOND pass (D-01): for every
// source='github_pr' task that still owns a worktree, read the PR's state and,
// if it is MERGED or CLOSED, attempt a conservative gated auto-removal (D-05),
// deleting the task row on success (D-07). It mirrors reapOnce's defensive
// structure: it never returns an error and degrades-don't-breaks on every
// failure (a gh/git hiccup warns and skips that task, never crashing the tick).
//
// The source='github_pr' WHERE clause is D-03's analogue of reapOnce's
// status='done' gate: manual tasks (source='manual') are NEVER selected here,
// so their worktrees are never auto-removed (D-87 preserved).
//
// The reaper passes force=false, stopSessions=false to the shared helper, so a
// dirty worktree OR a running/detached session BLOCKS removal (it is never
// forced past, never silently killed). The two extra conservative gates
// (rev-list unpushed against a re-fetched PR head, repo-global stash) live HERE,
// not in the helper (RESEARCH Open Q1), and are evaluated from FRESH git state.
func (r *Reaper) reconcilePRsOnce(ctx context.Context) {
	if r.pr == nil {
		return // PR pass not wired (Done-TTL-only construction / tests)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.pr_number, t.worktree_path, p.github_repo, p.repo_path
		   FROM tasks t JOIN projects p ON p.id = t.project_id
		  WHERE t.source = 'github_pr'
		    AND t.worktree_path IS NOT NULL
		    AND p.github_repo IS NOT NULL`)
	if err != nil {
		slog.Warn("reaper: querying PR review tasks", "error", err)
		return
	}
	type prTask struct {
		id      int64
		n       int64
		wtPath  string
		repo    string
		repoDir string
	}
	var tasks []prTask
	for rows.Next() {
		var t prTask
		var n sql.NullInt64
		if err := rows.Scan(&t.id, &n, &t.wtPath, &t.repo, &t.repoDir); err != nil {
			slog.Warn("reaper: scanning PR task", "error", err)
			continue
		}
		t.n = n.Int64
		tasks = append(tasks, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		slog.Warn("reaper: iterating PR tasks", "error", err)
		return
	}

	for _, t := range tasks {
		state, err := r.pr.PRState(ctx, t.repo, int(t.n))
		if err != nil {
			slog.Warn("reaper: reading PR state", "task", t.id, "pr", t.n, "error", err)
			continue // degrade-don't-break (D-03)
		}
		if state != "MERGED" && state != "CLOSED" {
			continue // OPEN or unknown -> leave it (D-03)
		}

		// CONSERVATIVE GATE (D-05). Skip on ANY of dirty / unpushed / stash /
		// running session. Computed from FRESH git state.
		// Gate a: dirty.
		if dirty, derr := r.wt.DirtyCount(ctx, t.wtPath); derr != nil {
			slog.Warn("reaper: dirty check", "task", t.id, "error", derr)
			continue
		} else if dirty > 0 {
			slog.Info("reaper: skip PR cleanup — dirty worktree", "task", t.id, "pr", t.n)
			continue
		}
		// Gate b: unpushed/local-only commits. headRefOid is NOT stored, so
		// re-fetch refs/pull/<n>/head (the same fetch CheckoutPR uses; fork-safe)
		// then rev-list FETCH_HEAD..HEAD (Pitfall 1 — porcelain alone misses a
		// committed-but-unpushed fixup).
		if ferr := r.wt.FetchRef(ctx, t.repoDir, fmt.Sprintf("refs/pull/%d/head", t.n)); ferr != nil {
			slog.Warn("reaper: fetch PR head for unpushed gate", "task", t.id, "pr", t.n, "error", ferr)
			continue // can't verify -> conservative skip
		}
		if unpushed, uerr := r.wt.UnpushedCount(ctx, t.wtPath, "FETCH_HEAD"); uerr != nil {
			slog.Warn("reaper: unpushed check", "task", t.id, "error", uerr)
			continue
		} else if unpushed > 0 {
			slog.Info("reaper: skip PR cleanup — unpushed commits", "task", t.id, "pr", t.n)
			continue
		}
		// Gate c: a stash (repo-global — Pitfall 2; conservative skip is OK).
		if stashes, serr := r.wt.StashCount(ctx, t.wtPath); serr != nil {
			slog.Warn("reaper: stash check", "task", t.id, "error", serr)
			continue
		} else if stashes > 0 {
			slog.Info("reaper: skip PR cleanup — stash present", "task", t.id, "pr", t.n)
			continue
		}
		// Gate d (sessions) is enforced INSIDE CleanupWorktreeGated via the
		// session count + stopSessions=false. Compute the count + live tmux the
		// same way worktreeHandlers does.
		sessionCount := r.runningSessions(t.id) + r.extraLiveTmux(ctx, t.id)
		live := r.liveTmuxNames(ctx, t.id)

		removed, reason, cerr := api.CleanupWorktreeGated(
			ctx, r.db, r.wt, r.sessionMgr, r.tmuxClient, live,
			t.id, t.repoDir, t.wtPath, sessionCount,
			false /*stopSessions*/, false /*force*/)
		if cerr != nil {
			slog.Warn("reaper: PR worktree cleanup", "task", t.id, "error", cerr)
			continue
		}
		if !removed {
			slog.Info("reaper: skip PR cleanup — gate", "task", t.id, "pr", t.n, "reason", reason)
			continue
		}

		// D-07: auto-cleanup DELETES the row. FK order (Pitfall 3): the helper
		// already killed sessions; clear tmux_sessions rows BEFORE tasks (no
		// CASCADE, FKs ON). Mirror taskHandlers.delete.
		if _, derr := r.db.ExecContext(ctx, `DELETE FROM tmux_sessions WHERE task_id = ?`, t.id); derr != nil {
			slog.Warn("reaper: deleting tmux_sessions rows", "task", t.id, "error", derr)
		}
		if _, derr := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, t.id); derr != nil {
			slog.Warn("reaper: deleting PR task row", "task", t.id, "error", derr)
			continue
		}
		slog.Info("reaped merged/closed PR worktree", "task", t.id, "pr", t.n, "state", state)
	}
}

// runningSessions counts the task's in-memory running sessions (mirrors
// worktreeHandlers.runningSessions). It reads the SessionStopper interface so it
// works under both New (spy) and NewWithPR (real *session.Manager).
func (r *Reaper) runningSessions(taskID int64) int {
	n := 0
	for _, info := range r.mgr.ListByTask(taskID) {
		if info.Status == session.StatusRunning {
			n++
		}
	}
	return n
}

// liveTmuxNames returns the task's tmux session names ALIVE on the dedicated
// socket (mirrors worktreeHandlers.liveTmuxNames): probe every tmux_sessions row
// with has-session, collecting a name only when CONCLUSIVELY alive (alive &&
// err == nil) — an inconclusive probe (broken/hung tmux) is read as neither
// alive nor dead (Pitfall 6). A name is collected so the helper's pre-remove
// kill loop reaps detached survivors. Guards the zero-tmuxClient case (the
// Done-TTL-only construction never wires one) by returning nil.
func (r *Reaper) liveTmuxNames(ctx context.Context, taskID int64) []string {
	if r.tmuxClient.Socket == "" {
		return nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		slog.Warn("reaper: listing tmux_sessions for task", "task", taskID, "error", err)
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Warn("reaper: scanning tmux_sessions row", "task", taskID, "error", err)
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("reaper: iterating tmux_sessions rows", "task", taskID, "error", err)
	}
	live := make([]string, 0, len(names))
	for _, name := range names {
		alive, err := r.tmuxClient.HasSession(ctx, name)
		if alive && err == nil {
			live = append(live, name)
		}
	}
	return live
}

// extraLiveTmux counts the task's live detached tmux survivors that the Manager
// has NO live in-memory session for (mirrors worktreeHandlers.cleanupSessionCount's
// fold), so an in-memory tmux session — already counted by runningSessions — is
// never double-counted (D-92). Guards the nil-sessionMgr case (Done-TTL-only
// construction) by returning 0.
func (r *Reaper) extraLiveTmux(ctx context.Context, taskID int64) int {
	if r.sessionMgr == nil {
		return 0
	}
	count := 0
	for _, name := range r.liveTmuxNames(ctx, taskID) {
		if !r.sessionMgr.HasLiveTmux(name) {
			count++
		}
	}
	return count
}

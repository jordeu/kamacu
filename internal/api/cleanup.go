package api

import (
	"context"
	"database/sql"
	"os"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// CleanupWorktreeGated runs the gated worktree-removal core shared by the HTTP
// handler (worktreeHandlers.remove) and the reaper (reconcilePRsOnce, 13-02).
// It is a BYTE-EQUIVALENT extraction of the inline sequence in remove(): the
// two gates (sessions, dirty), stop-before-remove, kill-tmux-before-remove,
// wt.Remove, and the null-columns UPDATE (manual semantics — the ROW survives;
// the reaper performs its own extra row-delete on top, D-07).
//
// It is EXPORTED so the reaper (a different package, internal/reaper) can call
// the one shared cleanup path — one path, two callers (D-04). reaper -> api is
// acyclic (api does NOT import reaper).
//
// It NEVER writes HTTP. A tripped gate returns removed=false + the SAME reason
// string the handler maps to a 409, and mutates NOTHING. A remove failure
// returns err. force/stopSessions come from the user-confirmed HTTP request;
// the reaper passes both false (D-05 conservative). sessionCount is the
// caller's already-computed cleanupSessionCount; liveTmux is the caller's
// already-probed liveTmuxNames (both handler and reaper compute these).
//
// GATE SCOPE (RESEARCH Open Q1): the two NEW conservative gates (rev-list
// unpushed, stash) live entirely in 13-02's reconcilePRsOnce, NOT here, so this
// helper stays a pure mechanical extraction verifiable against the unchanged
// worktree DELETE tests (D-04 regression guard).
func CleanupWorktreeGated(
	ctx context.Context, db *sql.DB, wt *worktree.Service,
	mgr *session.Manager, tmuxClient tmux.Client, liveTmux []string,
	taskID int64, repo, path string, sessionCount int,
	stopSessions, force bool,
) (removed bool, reason string, err error) {
	// Gate 1 (D-32 sessions): never silently kill sessions. The count folds in
	// live detached tmux survivors (D-92), so the gate trips for a detached
	// tmux session too — the caller then passes stopSessions=true and the kill
	// loop below reaps it.
	if sessionCount > 0 && !stopSessions {
		return false, "sessions running", nil
	}
	// Gate 2 (D-33 dirty): never silently destroy uncommitted work. A missing
	// dir counts clean (Remove self-heals; never block cleanup on it).
	dirty := 0
	if _, statErr := os.Stat(path); statErr == nil {
		dirty, err = wt.DirtyCount(ctx, path)
		if err != nil {
			return false, "", err
		}
	}
	if dirty > 0 && !force {
		return false, "worktree has uncommitted changes", nil
	}

	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Stop FIRST and block through the SIGTERM grace BEFORE remove — this
	// ordering IS the refuse-while-running enforcement (git removes trees under
	// live cwds without error, Pitfall 4).
	mgr.StopAllForTask(taskID)
	// Kill every live tmux session of the task DIRECTLY before wt.Remove
	// (D-92/D-93). StopAllForTask only reaches in-memory sessions; a detached
	// tmux survivor has no in-memory session, so without this kill git would
	// remove the tree while the tmux server stays daemonized cwd'd inside it
	// (the orphan-shell hole). KillSession is idempotent; a failure is best-
	// effort and NEVER blocks cleanup (Pitfall 5). The caller passes the live
	// names it already probed.
	for _, name := range liveTmux {
		_ = tmuxClient.KillSession(cctx, name)
	}
	// Plain remove when clean; --force only when the user passed the dirty
	// gate. The clean-but-submodules --force fallback lives inside Remove.
	if rerr := wt.Remove(cctx, repo, path, dirty > 0); rerr != nil {
		return false, "", rerr
	}
	// Manual semantics: the ROW survives, columns nulled (D-26 absent state; the
	// branch ALWAYS survives, D-34). The reaper's extra DELETE (13-02) is on
	// top of this for the auto-cleanup case (D-07).
	if _, uerr := db.Exec(
		`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`, taskID); uerr != nil {
		return false, "", uerr
	}
	return true, "", nil
}

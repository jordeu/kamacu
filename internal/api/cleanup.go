package api

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	"kamacu/internal/session"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
)

// BlockedError is the D-01 permission-blocked signal: a worktree containing a
// foreign-uid, mode-700 subtree (the container `./.db` case) cannot be deleted
// by the unprivileged app. It carries the offending Path so the panel can render
// a copyable "sudo rm -rf <Path>" hint — the app NEVER runs sudo/pkexec itself
// (D-01, Security Domain V5). CleanupWorktreeGated returns it with
// reason=="blocked" instead of a raw error, and the handler maps it to a 200
// `{outcome:"blocked", path}` (NOT a 500) so the row renders inline (RESEARCH A4).
type BlockedError struct{ Path string }

func (e *BlockedError) Error() string { return "blocked: " + e.Path }

// classifyRemoveBlocked reports whether a wt.Remove failure is a permission
// block (D-01), and if so returns a *BlockedError carrying the offending path.
//
// wt.Remove shells out to git, so a real permission failure surfaces as git's
// stderr STRING (not a wrapped *fs.PathError); the string branch is the one that
// fires in practice. The *fs.PathError / fs.ErrPermission branch is the precise
// Go-native path (RESEARCH Option B, verified: os.RemoveAll on a mode-000 subtree
// yields fs.ErrPermission AND exposes the exact blocked dir via *fs.PathError.Path)
// — kept defensively for any caller that surfaces a native error.
//
// Path resolution: prefer *fs.PathError.Path (exact, no parsing); else parse
// git's `could not open directory '<dir>'` message; else fall back to the passed
// worktree path. NEVER retry --force on a blocked case (Pitfall 1) — that
// produces the misleading "is not a working tree" and leaves an undeletable shell.
func classifyRemoveBlocked(err error, fallbackPath string) (*BlockedError, bool) {
	if err == nil {
		return nil, false
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(err, fs.ErrPermission) {
		return &BlockedError{Path: pe.Path}, true
	}
	// Stderr-string fallback for the git-shell-out path (verified messages:
	// "warning: could not open directory '<dir>': Permission denied" +
	// "error: failed to delete '<path>': Directory not empty").
	msg := err.Error()
	permBlocked := strings.Contains(msg, "Permission denied") ||
		(strings.Contains(msg, "Directory not empty") && strings.Contains(msg, "failed to delete"))
	if !permBlocked {
		return nil, false
	}
	path := fallbackPath
	if dir := parseCouldNotOpenDir(msg); dir != "" {
		path = dir
	}
	return &BlockedError{Path: path}, true
}

// parseCouldNotOpenDir extracts <dir> from git's `could not open directory
// '<dir>'` line, or "" if absent.
func parseCouldNotOpenDir(msg string) string {
	const marker = "could not open directory '"
	i := strings.Index(msg, marker)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(marker):]
	if j := strings.IndexByte(rest, '\''); j >= 0 {
		return rest[:j]
	}
	return ""
}

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
		// Extension A (D-01): a permission block is a DISTINCT outcome, not a
		// generic error. Detect it BEFORE surfacing the raw error and NEVER retry
		// --force (Pitfall 1). Returns reason=="blocked" + a *BlockedError the
		// handler maps to a 200 {outcome:"blocked", path}.
		if be, blocked := classifyRemoveBlocked(rerr, path); blocked {
			return false, "blocked", be
		}
		return false, "", rerr
	}
	// Extension B (orphan mode): an orphan worktree (taskID==0) has NO DB task
	// row, so skip the null-columns UPDATE entirely. For a referenced task
	// (taskID>0) the manual semantics are unchanged — the ROW survives, columns
	// nulled (D-26 absent state; the branch ALWAYS survives, D-34). The reaper's
	// extra DELETE (13-02) is on top of this for the auto-cleanup case (D-07).
	if taskID > 0 {
		if _, uerr := db.Exec(
			`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`, taskID); uerr != nil {
			return false, "", uerr
		}
	}
	return true, "", nil
}

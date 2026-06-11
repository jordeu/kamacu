// Package worktree shells out to the system git CLI to create, inspect, and
// remove per-task worktrees (GIT-01, GIT-03 mechanics). Every behavior it
// relies on was empirically verified against git 2.43.0 in 03-RESEARCH.md.
//
// Invariants:
//   - Always exec.CommandContext with arg arrays — never a shell (task titles
//     flow near these commands; injection surface).
//   - Success is exit 0 ONLY. Exit codes are inconsistent across failure
//     modes (255 vs 128 observed) and stderr is chatty on success
//     ("Preparing worktree…"), so neither is a signal.
//   - This package NEVER deletes branches (D-34) and never fetches (D-24).
package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// gitRun executes `git -C repo args...` and returns stdout. Failure is any
// nonzero exit; the returned error message is git's stderr, trimmed and with
// the "fatal: " prefix stripped — it surfaces verbatim in the UI as
// "Couldn't create a worktree: {git error}" per UI-SPEC.
func gitRun(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" {
			msg = err.Error() // e.g. git binary missing, context canceled
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// Slug converts a task title to a ref-safe slug: lowercase, runs of
// characters outside [a-z0-9] collapse to a single '-', leading/trailing '-'
// trimmed, capped at 40 chars (re-trimming any trailing '-'), and an empty
// result becomes "task". The constructive charset [a-z0-9-] sidesteps all of
// git's ref-format edge cases ("..", "@{", ".lock", trailing "/", …) — no
// regex over git's rules needed.
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-") // charset is ASCII; byte slice is safe
	}
	if s == "" {
		return "task"
	}
	return s
}

// Service serializes worktree mutations (coarse mutex — single-user app, so
// Retry spam can't race create/remove) and owns the central worktree root
// (D-23, e.g. ~/.kangent/worktrees).
type Service struct {
	mu   sync.Mutex
	Root string
}

// NewService returns a Service rooted at root (absolute, already ~-expanded).
// Tests pass t.TempDir().
func NewService(root string) *Service {
	return &Service{Root: root}
}

// PathFor returns Root/<filepath.Base(repoPath)>/<slug>-<taskID>. Task IDs
// are globally unique, so leaf dirs never collide even across projects.
// Production placement moved to PathUnder in Phase 6 (settings-driven base);
// PathFor remains as the pre-Phase-6 shape for tests.
func (s *Service) PathFor(repoPath, slug string, taskID int64) string {
	return filepath.Join(s.Root, filepath.Base(repoPath), slug+"-"+strconv.FormatInt(taskID, 10))
}

// PathUnder returns base/<filepath.Base(repoPath)>/<slug>-<taskID>.
// Production placement is settings-driven (Phase 6, WT-01): callers pass the
// ~-expanded worktree_base. Service.Root/PathFor remain for tests and as the
// pre-Phase-6 shape; the creation path no longer consults Root.
func PathUnder(base, repoPath, slug string, taskID int64) string {
	return filepath.Join(base, filepath.Base(repoPath), slug+"-"+strconv.FormatInt(taskID, 10))
}

// ResolveBase returns a commit-ish to branch from, per D-24 (default branch
// tip, local reads only — no network, ever; git's remote head
// auto-detection commands hit the network and are forbidden here).
// Chain (RESEARCH §1, all verified):
//
//  1. origin/HEAD symbolic ref → branch name N; prefer local refs/heads/N
//     (the local tip per D-24), else the remote-tracking ref origin/N.
//  2. Local main, then master.
//  3. Current branch (symbolic-ref --short HEAD).
//  4. Detached HEAD → raw sha via rev-parse.
//
// All legs failing means an unborn HEAD (fresh `git init`, no commit) — the
// error lands the task in the D-25 failed state.
func (s *Service) ResolveBase(ctx context.Context, repo string) (string, error) {
	if out, err := gitRun(ctx, repo, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/origin/")
		if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name, nil // local tip of the default branch
		}
		return "origin/" + name, nil // remote-tracking ref as commit-ish
	}
	for _, b := range []string{"main", "master"} {
		if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+b); err == nil {
			return b, nil
		}
	}
	if out, err := gitRun(ctx, repo, "symbolic-ref", "--short", "HEAD"); err == nil {
		name := strings.TrimSpace(out)
		// On an unborn HEAD (fresh init, no commit) symbolic-ref still
		// succeeds, but the branch ref has no commit and is not a valid
		// base — verify it resolves; otherwise fall through to rev-parse,
		// which fails with git's own error (the D-25 failed state).
		if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name, nil
		}
	}
	out, err := gitRun(ctx, repo, "rev-parse", "HEAD") // detached HEAD still resolves
	if err != nil {
		return "", err // unborn HEAD → D-25 failed state
	}
	return strings.TrimSpace(out), nil
}

// Create makes a worktree at path on branch, branching from base.
//
// The leaf path is pre-checked with os.Stat BEFORE git runs: `worktree add
// -b` creates the branch before validating the path, so a path collision
// would leak the branch (Pitfall 1, verified). The app owns Root, so a
// collision is a bug — fail fast with the path in the message.
//
// If the branch already exists and no worktree uses it — either leaked by a
// previous failure or deliberately kept after cleanup (D-34) — it is reused
// via `worktree add` without -b instead of failing on "branch already
// exists".
func (s *Service) Create(ctx context.Context, repo, branch, path, base string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err := gitRun(ctx, repo, "worktree", "add", path, branch)
		return err
	}
	_, err := gitRun(ctx, repo, "worktree", "add", "-b", branch, path, base)
	return err
}

// DirtyCount returns the number of porcelain=v2 records for the worktree at
// wt, counting untracked files individually (--untracked-files=all). The
// per-file untracked count is load-bearing: plain `worktree remove` refuses
// on untracked-only dirt (Pitfall 2, verified), so the dirty check must see
// exactly what remove checks. The count feeds the D-33 changed-file warning.
func (s *Service) DirtyCount(ctx context.Context, wt string) (int, error) {
	out, err := gitRun(ctx, wt, "status", "--porcelain=v2", "--untracked-files=all", "-z")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range strings.Split(out, "\x00") {
		if strings.TrimSpace(rec) != "" {
			n++
		}
	}
	return n, nil
}

// Remove removes the worktree at wt from repo, then prunes bookkeeping
// (D-34). It NEVER deletes branches.
//
// Fallbacks (all verified on git 2.43):
//   - Clean worktrees with INITIALIZED submodules refuse plain remove
//     ("working trees containing submodules cannot be ... removed",
//     Pitfall 3). When !force fails with that message, retry once with
//     --force — safe because the caller verified cleanliness via DirtyCount.
//   - A path never registered as a worktree whose directory is also absent
//     is treated as already-removed success (idempotency).
//   - "--force --force" is never used: only manual `git worktree lock`
//     requires it, and a user's manual lock must surface as an error, not
//     be bulldozed.
//
// Pitfall 4 (verified): git happily removes a worktree while live processes
// are cwd'd inside it — no EBUSY; the processes survive with an ENOENT cwd.
// The running-sessions gate (D-32) is therefore enforced by the API layer
// BEFORE calling Remove; there is no git-level safety net and none here.
func (s *Service) Remove(ctx context.Context, repo, wt string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wt)
	if _, err := gitRun(ctx, repo, args...); err != nil {
		if !force && strings.Contains(err.Error(), "submodules") {
			_, err = gitRun(ctx, repo, "worktree", "remove", "--force", wt)
		}
		if err != nil && strings.Contains(err.Error(), "is not a working tree") {
			if _, statErr := os.Stat(wt); os.IsNotExist(statErr) {
				err = nil // never registered and already gone — idempotent
			}
		}
		if err != nil {
			return err
		}
	}
	// D-34 bookkeeping; verified harmless no-op when nothing to prune.
	_, _ = gitRun(ctx, repo, "worktree", "prune")
	return nil
}

// EnsureSubmodules best-effort initializes submodules if .gitmodules exists
// at the worktree root (`worktree add` does not populate them — verified).
// Errors are returned for logging only: callers must NOT fail worktree
// creation on them (D-25 spirit) — the worktree is usable regardless, and a
// bash tab lets the user run the init manually (e.g. when it needs network).
func (s *Service) EnsureSubmodules(ctx context.Context, wt string) error {
	if _, err := os.Stat(filepath.Join(wt, ".gitmodules")); err != nil {
		return nil // no submodules — nothing to do, no git call needed
	}
	_, err := gitRun(ctx, wt, "submodule", "update", "--init", "--recursive")
	return err
}

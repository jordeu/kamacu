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
func (s *Service) PathFor(repoPath, slug string, taskID int64) string {
	return filepath.Join(s.Root, filepath.Base(repoPath), slug+"-"+strconv.FormatInt(taskID, 10))
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

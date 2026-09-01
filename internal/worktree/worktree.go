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
//   - This package NEVER deletes branches (D-34) and never fetches (D-24) —
//     EXCEPT CheckoutPR/FetchRef, ResolveBaseFresh, and FastForwardDefault,
//     deliberate scoped exceptions to the never-fetch invariant: (1)
//     PR-head/PR-base retrieval (Phase 12, ARCHITECTURE §4) — a PR review's
//     whole point is fetching someone else's branch; (2) ResolveBaseFresh
//     (Phase 14, CKOUT-02/D-04) fetches origin's default branch and returns
//     origin/<default> as the base for a MANAGED checkout, and
//     FastForwardDefault follows it with a local `fetch .` to fast-forward
//     the local default branch to that same tip — callers gate both on the
//     project's managed marker so folder projects keep D-24 byte-for-byte.
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
// (D-23, e.g. ~/.kamacu/worktrees).
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

// DefaultBranch returns the repo's default branch name by reading the same
// origin/HEAD symbolic ref ResolveBase's first leg uses, stripping the leading
// "origin/" (e.g. refs/remotes/origin/HEAD → "main"). A managed clone always
// has origin/HEAD set (verified: gh repo clone + file:// clone both set it), so
// this is the precise, cheap name to feed FetchRef for the CKOUT-02 pre-task
// fetch. Returns the bare branch name (no "origin/" prefix). An error means
// origin/HEAD is absent — the caller skips the fetch and lets ResolveBase work
// on the local base (this is read-only; it does NOT hit the network).
func (s *Service) DefaultBranch(ctx context.Context, repo string) (string, error) {
	out, err := gitRun(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(out), "origin/"), nil
}

// ResolveBaseFresh returns a commit-ish to branch from on a MANAGED checkout:
// it first runs the CKOUT-02 best-effort default-branch fetch, then returns
// the remote-tracking ref origin/<default> when it resolves. The LOCAL
// default branch is deliberately never preferred here: nothing in Kamacu
// ever updates it, so in a managed clone it is frozen at clone time — leg 1
// of ResolveBase would silently base every new task on that outdated tip
// even after a successful fetch (the exact bug this method exists to fix).
// Basing work on origin/<default> also keeps the diff tab honest: it diffs
// against the same ref, so it shows exactly what a PR targeting the default
// branch will show.
//
// The fetch is the same deliberate, scoped D-24 exception CheckoutPR and
// FetchRef carry — callers gate on the project's managed marker; folder
// projects never reach this method. Best-effort (D-05): a failed fetch is
// discarded and the still-resolvable (clone-time) origin/<default> is
// returned. Only when origin/HEAD is absent or origin/<default> does not
// resolve at all does the full ResolveBase local chain answer — a merely
// stale base is never an error.
func (s *Service) ResolveBaseFresh(ctx context.Context, repo string) (string, error) {
	if name, err := s.DefaultBranch(ctx, repo); err == nil {
		_ = s.FetchRef(ctx, repo, name) // best-effort (D-05): failure falls through
		remote := "origin/" + name
		if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote); err == nil {
			return remote, nil
		}
	}
	return s.ResolveBase(ctx, repo)
}

// FastForwardDefault advances the local refs/heads/<defaultBranch> to the
// fetched refs/remotes/origin/<defaultBranch> tip. ResolveBaseFresh bases new
// task branches on origin/<default> and deliberately never prefers the local
// branch — which therefore stays frozen at clone time unless something moves
// it. This method is that something, and the point is the LOCAL readers: the
// cleanup panel's unpushed counts resolve their base via ResolveBase
// (local-first), so against a frozen tip they overcount every upstream
// commit since the clone. Managed-only (gated by provisionWorktree, same
// D-24 exception family as FetchRef) and STRICTLY non-destructive:
//
//   - No local <defaultBranch> → no-op nil (ResolveBaseFresh's origin/ leg
//     covers that shape already).
//   - origin/<defaultBranch> is not a DESCENDANT of the local tip
//     (merge-base --is-ancestor fails: diverged history, an origin rewrite,
//     or a git error — indistinguishable at exit-code granularity) → no-op
//     nil. NEVER a force move; the local tip is left exactly where it is.
//   - <defaultBranch> checked out at the repo root (the normal managed
//     clone): `merge --ff-only origin/<defaultBranch>`. git itself is the
//     safety gate — it advances branch+index+worktree atomically on a clean
//     fast-forward and refuses outright ("local changes would be
//     overwritten") rather than clobbering any local modification, so no
//     pre-clean check is needed.
//   - <defaultBranch> not the root's current branch (detached root or root
//     on another branch): `fetch . <src>:refs/heads/<defaultBranch>` — the
//     plain (non-'+') refspec makes git enforce ff-only natively, and the
//     ref-only update touches no worktree files.
//
// Callers run it right AFTER ResolveBaseFresh, so the remote-tracking ref it
// reads is the one that fetch just advanced (including a still-resolvable
// tip from an earlier fetch when this provisioning's fetch failed). Real git
// failures are returned for the caller to LOG only — this is branch hygiene,
// never a provisioning gate (D-05 spirit).
func (s *Service) FastForwardDefault(ctx context.Context, repo, defaultBranch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+defaultBranch); err != nil {
		return nil // no local default branch — nothing to advance
	}
	if _, err := gitRun(ctx, repo, "merge-base", "--is-ancestor",
		"refs/heads/"+defaultBranch, "refs/remotes/origin/"+defaultBranch); err != nil {
		return nil // not a fast-forward (or git error) — never force
	}
	if out, err := gitRun(ctx, repo, "symbolic-ref", "--short", "HEAD"); err == nil &&
		strings.TrimSpace(out) == defaultBranch {
		_, err := gitRun(ctx, repo, "merge", "--ff-only", "origin/"+defaultBranch)
		return err
	}
	_, err := gitRun(ctx, repo, "fetch", ".",
		"refs/remotes/origin/"+defaultBranch+":refs/heads/"+defaultBranch)
	return err
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

// branchExistsLocally reports whether refs/heads/<name> exists in repo (the
// same show-ref --verify --quiet pattern Create uses for its branch-reuse
// check). It is the guard that keeps CheckoutPR from ever reusing or moving an
// existing local branch ref (GHREV-05): we only ever create a name that does
// NOT already exist.
func (s *Service) branchExistsLocally(ctx context.Context, repo, name string) bool {
	_, err := gitRun(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// CheckoutPR provisions a worktree at `path` on the PR head, checked out on a
// NAMED branch (GHREV-01: the PR's REAL head branch, headRefName), with a safe
// pr/<n> collision fallback (GHREV-05). headOID comes from gh pr view (NOT
// FETCH_HEAD — that is clobbered by a later base fetch, RESEARCH Pitfall 1).
// The fetch is a DELIBERATE, SCOPED EXCEPTION to this package's "never fetch"
// invariant (D-24): a PR review's whole point is fetching someone else's
// branch. refs/pull/<n>/head resolves FORK heads from the BASE repo, so forks
// need no fork remote and no special-casing. Remote is hard-coded "origin"
// for v1.3 (RESEARCH OQ3 — every linked repo is a GitHub clone).
//
// Branch-name selection NEVER reuses or moves an existing local ref — that is
// the GHREV-05 safety guarantee. The collision fallback protects the fork
// same-name case (e.g. a fork PR whose head is "master"): we never touch the
// project's existing local "master", so the primary checkout's HEAD is
// unchanged. `worktree add -b <chosen>` creates the branch AND checks it out
// in the NEW worktree in one step, pinned to headOID, so the project's primary
// checkout HEAD/branch never move.
func (s *Service) CheckoutPR(ctx context.Context, repo, path, headOID, headRefName string, prNumber int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := gitRun(ctx, repo, "fetch", "origin", fmt.Sprintf("refs/pull/%d/head", prNumber)); err != nil {
		return err
	}

	// Pick a branch name that does NOT already exist locally (never reuse/move
	// an existing ref — GHREV-05). Rule order:
	//   1. headRefName, if set and free.
	//   2. pr/<n>, if free (the fork same-name collision fallback).
	//   3. pr/<n>-<short headOID>, if free (double-collision resilience).
	//   4. otherwise a clear error.
	var chosen string
	switch {
	case headRefName != "" && !s.branchExistsLocally(ctx, repo, headRefName):
		chosen = headRefName
	default:
		candidate := fmt.Sprintf("pr/%d", prNumber)
		if !s.branchExistsLocally(ctx, repo, candidate) {
			chosen = candidate
		} else {
			shortOID := headOID
			if len(shortOID) > 7 {
				shortOID = shortOID[:7]
			}
			candidate = fmt.Sprintf("pr/%d-%s", prNumber, shortOID)
			if s.branchExistsLocally(ctx, repo, candidate) {
				return fmt.Errorf("couldn't pick a branch name for PR #%d", prNumber)
			}
			chosen = candidate
		}
	}

	// `worktree add -b <chosen>` creates `chosen` pinned to headOID AND checks
	// it out in the NEW worktree only — the primary checkout never moves.
	_, err := gitRun(ctx, repo, "worktree", "add", "-b", chosen, path, headOID)
	return err
}

// FetchRef best-effort fetches a named ref from origin (e.g. a PR base
// branch) so the remote-tracking ref origin/<ref> exists for a later
// merge-base. Same scoped-fetch exception as CheckoutPR. Callers may ignore
// the error and let the subsequent merge-base surface a clear message.
func (s *Service) FetchRef(ctx context.Context, repo, ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := gitRun(ctx, repo, "fetch", "origin", ref)
	return err
}

// Entry is one record from `git worktree list --porcelain -z`. It is a faithful
// mirror of git's porcelain attributes (verified against git 2.43.0, RESEARCH
// Pattern 1): the format is documented stable regardless of config
// (https://git-scm.com/docs/git-worktree). The FIRST entry of every repo's list
// is the MAIN worktree (the repo checkout itself); List does NOT filter it —
// the API layer excludes the main worktree by comparing
// filepath.Clean(Path) == filepath.Clean(project.repo_path), keeping List an
// independently-testable parser (RESEARCH Pitfall 2).
type Entry struct {
	Path           string
	HEAD           string
	Branch         string // "" when detached or bare
	Bare           bool
	Detached       bool
	Locked         bool
	LockedReason   string
	Prunable       bool
	PrunableReason string
}

// List enumerates every worktree registered in repo, robustly, via
// `git worktree list --porcelain -z`. The -z variant NUL-separates attributes
// (\x00) and double-NUL-separates records (\x00\x00), which makes paths
// containing spaces/newlines safe to parse (RESEARCH Pattern 1). A worktree
// whose directory was manually deleted appears with a `prunable gitdir file
// points to non-existent location` attribute (losing its `branch`, showing
// `detached`) — git's own reverse-orphan signal the cleanup panel surfaces.
//
// The main worktree is returned like any other entry (see Entry doc). Follows
// the gitRun arg-array invariant — never a shell (Security Domain V5).
func (s *Service) List(ctx context.Context, repo string) ([]Entry, error) {
	out, err := gitRun(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out), nil
}

// parseWorktreeList parses the raw `worktree list --porcelain -z` buffer into
// Entry records. Records are "\x00\x00"-separated; attributes within a record
// are "\x00"-separated. A trailing "\x00" (or "\x00\x00") is trimmed before the
// record split so it never yields a spurious empty Entry. Split out from List
// so the crafted-buffer edge cases (locked reasons, bare, trailing-NUL) are
// unit-testable without live git. Mirrors the switch in RESEARCH Pattern 1.
func parseWorktreeList(out string) []Entry {
	var entries []Entry
	for _, rec := range strings.Split(strings.TrimRight(out, "\x00"), "\x00\x00") {
		if rec == "" {
			continue
		}
		var e Entry
		for _, line := range strings.Split(rec, "\x00") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				e.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "HEAD "):
				e.HEAD = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				e.Branch = strings.TrimPrefix(line, "branch ")
			case line == "bare":
				e.Bare = true
			case line == "detached":
				e.Detached = true
			case line == "locked" || strings.HasPrefix(line, "locked "):
				e.Locked = true
				e.LockedReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
			case line == "prunable" || strings.HasPrefix(line, "prunable "):
				e.Prunable = true
				e.PrunableReason = strings.TrimSpace(strings.TrimPrefix(line, "prunable"))
			}
		}
		entries = append(entries, e)
	}
	return entries
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

// UnpushedCount returns the number of commits in base..HEAD for the worktree
// at wt — commits present locally but NOT in base (e.g. a committed-but-unpushed
// fixup, which is INVISIBLE to `git status --porcelain` — verified, Pitfall 1).
// base is typically FETCH_HEAD after a fresh `fetch origin refs/pull/<n>/head`.
// A non-zero count means the reaper MUST skip auto-removal (D-05 gate b).
func (s *Service) UnpushedCount(ctx context.Context, wt, base string) (int, error) {
	out, err := gitRun(ctx, wt, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if perr != nil {
		return 0, perr
	}
	return n, nil
}

// StashCount returns the number of stash entries visible from the worktree at
// wt. NOTE (Pitfall 2, verified): stashes are stored as refs/stash in the
// SHARED common git dir, so this is REPO-GLOBAL — a stash created in the main
// checkout is visible here. The reaper uses it as a conservative gate (D-05
// gate c): any stash anywhere in the repo skips auto-removal. This can cause a
// too-conservative skip (never a destructive removal) — the correct failure
// direction; the user's escape hatch is the manual D-08 cleanup.
func (s *Service) StashCount(ctx context.Context, wt string) (int, error) {
	out, err := gitRun(ctx, wt, "stash", "list")
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	return len(strings.Split(out, "\n")), nil
}

// Prune deregisters worktrees whose directories no longer exist by running
// `git worktree prune` at repo (D-07 stale-pointer cleanup). It touches no
// branches and deletes no files — a manually-removed worktree dir leaves a
// dangling `.git/worktrees/<name>` admin entry that prune clears so a later scan
// is clean. Serialized under the same mutex as Create/Remove. `worktree prune`
// is a verified harmless no-op when there is nothing to prune.
func (s *Service) Prune(ctx context.Context, repo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := gitRun(ctx, repo, "worktree", "prune")
	return err
}

// RefResolves reports whether ref resolves to a commit in the worktree at wt,
// using a NETWORK-FREE `rev-parse --verify --quiet <ref>^{commit}` (no fetch).
// The panel uses it to decide whether a github_pr row's origin/<pr_base_ref>
// exists locally before measuring unpushed commits against it (Pattern 3): if
// the remote-tracking ref was never fetched, the base is unresolvable and the
// row degrades to unpushed:null rather than surfacing a git error. The ^{commit}
// peel keeps a tag or annotated ref from passing as a non-commit.
func (s *Service) RefResolves(ctx context.Context, wt, ref string) bool {
	_, err := gitRun(ctx, wt, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
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

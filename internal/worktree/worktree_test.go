package worktree

// Tests run against REAL git in t.TempDir() repos — no mocks. Every behavior
// asserted here was empirically verified on git 2.43.0 in 03-RESEARCH.md.
// Hygiene: TestMain isolates host git config from every git invocation
// (helpers AND the package under test); helper commands additionally pass
// -c user.name/-c user.email so commits work without any config.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Never let host/global git config leak into any git call made by this
	// test binary — including the package's own exec.CommandContext calls.
	// The "global" config is a controlled temp file (not /dev/null) because
	// the submodule tests need file-protocol clones, and the clone subprocess
	// spawned by `submodule update --init` does not see the superproject's
	// repo-local config — only global/system config or the environment.
	dir, err := os.MkdirTemp("", "worktree-gitconfig")
	if err != nil {
		panic(err)
	}
	cfg := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[protocol \"file\"]\n\tallow = always\n"), 0o644); err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", cfg)
	os.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// gitCmd runs git in dir with a throwaway identity and isolated config,
// failing the test on any nonzero exit. Returns combined output.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// makeRepoOn creates a throwaway repo in t.TempDir() on the given initial
// branch with one commit (file.txt).
func makeRepoOn(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	gitCmd(t, dir, "add", "file.txt")
	gitCmd(t, dir, "commit", "-m", "initial")
	return dir
}

func makeRepo(t *testing.T) string { return makeRepoOn(t, "main") }

// cloneRepo clones src (file:// — sets origin/HEAD, per RESEARCH §1) into a
// fresh temp dir and returns the clone path.
func cloneRepo(t *testing.T, src string) string {
	t.Helper()
	parent := t.TempDir()
	dst := filepath.Join(parent, "clone")
	gitCmd(t, parent, "clone", "file://"+src, dst)
	return dst
}

// refFormatOK asserts name is a valid branch name per git's own oracle.
func refFormatOK(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("git", "check-ref-format", "--branch", name)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("git check-ref-format --branch %q: %v\n%s", name, err, out)
	}
}

func TestSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"punctuation collapses", "Fix Login!!", "fix-login"},
		{"runs and edges trimmed", "  --Weird___Title--  ", "weird-title"},
		{"no latin chars survive", "Тест", "task"},
		{"long title capped at 40", strings.Repeat("a", 60), strings.Repeat("a", 40)},
		{
			"cap re-trims trailing dash",
			"aaaaaaaaa bbbbbbbbb ccccccccc ddddddddd eeeeeeeee",
			"aaaaaaaaa-bbbbbbbbb-ccccccccc-ddddddddd",
		},
		{"empty title", "", "task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Slug(tc.in)
			if got != tc.want {
				t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) > 40 {
				t.Errorf("Slug(%q) = %q: longer than 40 chars", tc.in, got)
			}
			if strings.HasSuffix(got, "-") {
				t.Errorf("Slug(%q) = %q: trailing dash", tc.in, got)
			}
			// Oracle: git itself must accept the resulting branch name.
			refFormatOK(t, "task/"+got+"-1")
		})
	}
}

// TestPathUnder: the settings-driven placement function keeps the locked
// repo-basename subdirectory structure under whatever base it is given
// (WT-01; callers pass the ~-expanded worktree_base).
func TestPathUnder(t *testing.T) {
	cases := []struct {
		name string
		base string
		repo string
		slug string
		id   int64
		want string
	}{
		{"basic", "/base", "/repos/myapp", "fix-login", 42, "/base/myapp/fix-login-42"},
		{"trailing slash on base neutralized", "/base/", "/repos/myapp", "fix-login", 42, "/base/myapp/fix-login-42"},
		{"nested base", "/home/u/.kangent/worktrees", "/work/proj", "task", 7, "/home/u/.kangent/worktrees/proj/task-7"},
		{"repo basename only", "/b", "/deep/nested/repo-dir", "s", 1, "/b/repo-dir/s-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathUnder(tc.base, tc.repo, tc.slug, tc.id); got != tc.want {
				t.Errorf("PathUnder(%q, %q, %q, %d) = %q, want %q", tc.base, tc.repo, tc.slug, tc.id, got, tc.want)
			}
		})
	}

	// PathUnder(root, ...) must agree with the legacy Service.PathFor shape —
	// existing tests seed the harness base with the old Root, so path
	// assertions keep working unchanged.
	svc := NewService("/root")
	if got, want := PathUnder("/root", "/r/app", "x", 3), svc.PathFor("/r/app", "x", 3); got != want {
		t.Errorf("PathUnder = %q, PathFor = %q — must agree for the same base", got, want)
	}
}

func TestResolveBaseOriginHeadWithLocalBranch(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	src := makeRepo(t)
	clone := cloneRepo(t, src) // origin/HEAD set, local "main" checked out

	got, err := svc.ResolveBase(ctx, clone)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got != "main" {
		t.Errorf("ResolveBase = %q, want %q (local tip of default branch)", got, "main")
	}
}

func TestResolveBaseOriginHeadWithoutLocalBranch(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	src := makeRepo(t)
	clone := cloneRepo(t, src)
	// Remove the local default branch so only origin/main remains.
	gitCmd(t, clone, "checkout", "-b", "other")
	gitCmd(t, clone, "branch", "-D", "main")

	got, err := svc.ResolveBase(ctx, clone)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got != "origin/main" {
		t.Errorf("ResolveBase = %q, want %q (remote-tracking commit-ish)", got, "origin/main")
	}
}

func TestResolveBaseLocalMainOrMaster(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())

	for _, branch := range []string{"main", "master"} {
		t.Run(branch, func(t *testing.T) {
			repo := makeRepoOn(t, branch) // no remote → no origin/HEAD
			got, err := svc.ResolveBase(ctx, repo)
			if err != nil {
				t.Fatalf("ResolveBase: %v", err)
			}
			if got != branch {
				t.Errorf("ResolveBase = %q, want %q", got, branch)
			}
		})
	}
}

func TestResolveBaseCurrentBranchFallback(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	repo := makeRepoOn(t, "trunk") // no origin/HEAD, no main/master

	got, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got != "trunk" {
		t.Errorf("ResolveBase = %q, want %q (current branch)", got, "trunk")
	}
}

func TestResolveBaseDetachedHead(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	repo := makeRepoOn(t, "trunk")
	gitCmd(t, repo, "checkout", "--detach")

	got, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(got) {
		t.Errorf("ResolveBase = %q, want a 40-hex raw sha (detached HEAD)", got)
	}
}

func TestResolveBaseUnbornHead(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	repo := t.TempDir()
	gitCmd(t, repo, "init", "-b", "main") // no commit → unborn HEAD

	if _, err := svc.ResolveBase(ctx, repo); err == nil {
		t.Error("ResolveBase on unborn HEAD succeeded, want error (D-25 failed state)")
	}
}

func TestCreateHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path := svc.PathFor(repo, "fix-login", 1)
	base, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}

	if err := svc.Create(ctx, repo, "task/fix-login-1", path, base); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list := gitCmd(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(list, "worktree "+path) {
		t.Errorf("worktree list missing %q:\n%s", path, list)
	}
	if strings.TrimSpace(gitCmd(t, repo, "branch", "--list", "task/fix-login-1")) == "" {
		t.Error("branch task/fix-login-1 not created")
	}
}

func TestCreateReusesExistingBranch(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path := svc.PathFor(repo, "x", 1)
	base, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	// Simulate the Pitfall-1 leak (or a branch kept after cleanup, D-34).
	gitCmd(t, repo, "branch", "task/x-1")

	if err := svc.Create(ctx, repo, "task/x-1", path, base); err != nil {
		t.Fatalf("Create with pre-existing unused branch: %v", err)
	}
	head := strings.TrimSpace(gitCmd(t, path, "symbolic-ref", "--short", "HEAD"))
	if head != "task/x-1" {
		t.Errorf("worktree HEAD = %q, want %q (branch reuse, no -b)", head, "task/x-1")
	}
}

func TestCreatePathCollision(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path := svc.PathFor(repo, "collide", 1)
	base, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("pre-create leaf: %v", err)
	}

	err = svc.Create(ctx, repo, "task/collide-1", path, base)
	if err == nil {
		t.Fatal("Create on existing leaf path succeeded, want error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not mention the colliding path %q", err, path)
	}
	// The pre-check must fire BEFORE git runs: no branch may leak (Pitfall 1).
	if strings.TrimSpace(gitCmd(t, repo, "branch", "--list", "task/collide-1")) != "" {
		t.Error("branch task/collide-1 leaked despite the path pre-check")
	}
}

// makeWorktree creates repo's worktree on branch via the service and returns
// its path. Fails the test on any error.
func makeWorktree(t *testing.T, svc *Service, repo, slug string, id int64) (path, branch string) {
	t.Helper()
	ctx := context.Background()
	branch = "task/" + slug + "-" + "1"
	path = svc.PathFor(repo, slug, id)
	base, err := svc.ResolveBase(ctx, repo)
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if err := svc.Create(ctx, repo, branch, path, base); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return path, branch
}

// assertBranchKept asserts the D-34 invariant: the branch survives Remove.
func assertBranchKept(t *testing.T, repo, branch string) {
	t.Helper()
	if strings.TrimSpace(gitCmd(t, repo, "branch", "--list", branch)) == "" {
		t.Errorf("branch %q was deleted — D-34 violated (Remove must never touch branches)", branch)
	}
}

// assertNotListed asserts the worktree bookkeeping no longer mentions path
// (prune ran after removal).
func assertNotListed(t *testing.T, repo, path string) {
	t.Helper()
	if list := gitCmd(t, repo, "worktree", "list", "--porcelain"); strings.Contains(list, "worktree "+path) {
		t.Errorf("worktree list still mentions %q after Remove:\n%s", path, list)
	}
}

// addSubmodule wires sub into src as a file:// submodule and commits.
// protocol.file.allow is set in src's LOCAL config (shared by its worktrees)
// so the package's own `submodule update --init` is permitted too.
func addSubmodule(t *testing.T, src, sub string) {
	t.Helper()
	gitCmd(t, src, "config", "protocol.file.allow", "always")
	gitCmd(t, src, "-c", "protocol.file.allow=always", "submodule", "add", "file://"+sub, "sub")
	gitCmd(t, src, "commit", "-m", "add submodule")
}

func TestDirtyCount(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, _ := makeWorktree(t, svc, repo, "dirty", 1)

	n, err := svc.DirtyCount(ctx, path)
	if err != nil {
		t.Fatalf("DirtyCount clean: %v", err)
	}
	if n != 0 {
		t.Errorf("DirtyCount clean = %d, want 0", n)
	}

	// 1 modified tracked + 1 staged new + 2 untracked files in a dir = 4.
	// Untracked MUST be counted per-file (-uall): plain `worktree remove`
	// refuses on untracked-only dirt (Pitfall 2), so the count must see it.
	if err := os.WriteFile(filepath.Join(path, "file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, path, "add", "staged.txt")
	if err := os.MkdirAll(filepath.Join(path, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(path, "newdir", f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	n, err = svc.DirtyCount(ctx, path)
	if err != nil {
		t.Fatalf("DirtyCount dirty: %v", err)
	}
	if n != 4 {
		t.Errorf("DirtyCount = %d, want 4 (modified + staged + 2 untracked files)", n)
	}
}

func TestRemoveCleanKeepsBranch(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, branch := makeWorktree(t, svc, repo, "clean", 1)

	if err := svc.Remove(ctx, repo, path, false); err != nil {
		t.Fatalf("Remove clean force=false: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after Remove: %v", err)
	}
	assertBranchKept(t, repo, branch)
	assertNotListed(t, repo, path)
}

func TestRemoveUntrackedDirtRequiresForce(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, branch := makeWorktree(t, svc, repo, "untracked", 1)
	// Untracked-only dirt — verified to refuse plain remove (Pitfall 2).
	if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Remove(ctx, repo, path, false); err == nil {
		t.Fatal("Remove with untracked dirt and force=false succeeded, want refusal")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("worktree dir vanished after refused Remove: %v", err)
	}

	if err := svc.Remove(ctx, repo, path, true); err != nil {
		t.Fatalf("Remove force=true: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after forced Remove: %v", err)
	}
	assertBranchKept(t, repo, branch)
	assertNotListed(t, repo, path)
}

func TestRemoveMissingPathIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	never := filepath.Join(t.TempDir(), "never-a-worktree")

	if err := svc.Remove(ctx, repo, never, false); err != nil {
		t.Errorf("Remove on unregistered absent path = %v, want nil (idempotent)", err)
	}
}

func TestRemoveCleanWithInitializedSubmodules(t *testing.T) {
	ctx := context.Background()
	sub := makeRepo(t)
	src := makeRepo(t)
	addSubmodule(t, src, sub)
	svc := NewService(t.TempDir())
	path, branch := makeWorktree(t, svc, src, "withsub", 1)
	if err := svc.EnsureSubmodules(ctx, path); err != nil {
		t.Fatalf("EnsureSubmodules: %v", err)
	}
	// Sanity: clean per our own check — the fallback must therefore be safe.
	if n, err := svc.DirtyCount(ctx, path); err != nil || n != 0 {
		t.Fatalf("DirtyCount = %d, %v; want 0, nil", n, err)
	}

	// Verified: plain remove refuses worktrees with initialized submodules
	// even when clean (Pitfall 3) — Remove must fall back to --force.
	if err := svc.Remove(ctx, src, path, false); err != nil {
		t.Fatalf("Remove clean-with-submodules force=false: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after Remove: %v", err)
	}
	assertBranchKept(t, src, branch)
	assertNotListed(t, src, path)
}

func TestEnsureSubmodulesNoGitmodules(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, _ := makeWorktree(t, svc, repo, "plain", 1)

	if err := svc.EnsureSubmodules(ctx, path); err != nil {
		t.Errorf("EnsureSubmodules without .gitmodules = %v, want nil", err)
	}
}

func TestEnsureSubmodulesInitializes(t *testing.T) {
	ctx := context.Background()
	sub := makeRepo(t)
	src := makeRepo(t)
	addSubmodule(t, src, sub)
	svc := NewService(t.TempDir())
	path, _ := makeWorktree(t, svc, src, "sub", 1)

	// worktree add does not populate submodules (verified): '-' prefix.
	before := gitCmd(t, path, "submodule", "status")
	if !strings.Contains(before, "-") {
		t.Fatalf("expected uninitialized submodule before EnsureSubmodules:\n%s", before)
	}

	if err := svc.EnsureSubmodules(ctx, path); err != nil {
		t.Fatalf("EnsureSubmodules: %v", err)
	}
	for _, line := range strings.Split(gitCmd(t, path, "submodule", "status"), "\n") {
		if strings.HasPrefix(line, "-") {
			t.Errorf("submodule still uninitialized after EnsureSubmodules: %s", line)
		}
	}
}

func TestCreateRawShaBase(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path := svc.PathFor(repo, "sha", 1)
	sha := strings.TrimSpace(gitCmd(t, repo, "rev-parse", "HEAD"))

	if err := svc.Create(ctx, repo, "task/sha-1", path, sha); err != nil {
		t.Fatalf("Create with raw sha base: %v", err)
	}
	wtSha := strings.TrimSpace(gitCmd(t, path, "rev-parse", "HEAD"))
	if wtSha != sha {
		t.Errorf("worktree HEAD = %s, want %s", wtSha, sha)
	}
}

// makePRRemote builds a "remote" repo on trunk with an initial commit, then a
// SECOND commit on a side branch that it publishes as the server-side PR head
// ref refs/pull/<n>/head (exactly how GitHub exposes a PR head — the side
// branch itself is deleted so only the pull ref resolves the head, mirroring a
// fork PR whose branch lives in another repo). Returns the remote path and the
// PR head OID.
func makePRRemote(t *testing.T, prNumber int) (remote, headOID string) {
	t.Helper()
	remote = makeRepoOn(t, "trunk")
	// A divergent PR-head commit on a throwaway branch.
	gitCmd(t, remote, "checkout", "-b", "pr-source")
	if err := os.WriteFile(filepath.Join(remote, "pr.txt"), []byte("pr change\n"), 0o644); err != nil {
		t.Fatalf("write pr.txt: %v", err)
	}
	gitCmd(t, remote, "add", "pr.txt")
	gitCmd(t, remote, "commit", "-m", "pr head commit")
	headOID = strings.TrimSpace(gitCmd(t, remote, "rev-parse", "HEAD"))
	// Publish as the GitHub-style server-side pull ref, then drop the branch so
	// ONLY refs/pull/<n>/head resolves the head (fork-PR shape).
	gitCmd(t, remote, "update-ref", "refs/pull/"+strconv.Itoa(prNumber)+"/head", headOID)
	gitCmd(t, remote, "checkout", "trunk")
	gitCmd(t, remote, "branch", "-D", "pr-source")
	return remote, headOID
}

// TestCheckoutPRNamedBranch is the GHREV-01 proof: the PR review worktree is on
// the PR's REAL head branch (headRefName), NOT a detached HEAD, pinned to the PR
// head OID.
func TestCheckoutPRNamedBranch(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote, headOID := makePRRemote(t, 1)
	clone := cloneRepo(t, remote)

	path := svc.PathFor(clone, "pr-1", 1)
	if err := svc.CheckoutPR(ctx, clone, path, headOID, "feature-x", 1); err != nil {
		t.Fatalf("CheckoutPR: %v", err)
	}

	// The worktree HEAD must equal the PR head OID exactly.
	wtSha := strings.TrimSpace(gitCmd(t, path, "rev-parse", "HEAD"))
	if wtSha != headOID {
		t.Errorf("worktree HEAD = %q, want PR head OID %q", wtSha, headOID)
	}
	// And it must be ON the named head branch — NOT detached.
	branch := strings.TrimSpace(gitCmd(t, path, "symbolic-ref", "--short", "HEAD"))
	if branch != "feature-x" {
		t.Errorf("worktree branch = %q, want %q (named head branch, GHREV-01)", branch, "feature-x")
	}
}

// TestCheckoutPRLeavesSourceHeadUnchanged is the GHREV-05 safety proof: opening
// a PR review must NOT move the project's primary checkout HEAD (same property
// for same-repo and fork PRs since the head comes from refs/pull/<n>/head).
func TestCheckoutPRLeavesSourceHeadUnchanged(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote, headOID := makePRRemote(t, 7)
	clone := cloneRepo(t, remote)

	branchBefore := strings.TrimSpace(gitCmd(t, clone, "symbolic-ref", "--short", "HEAD"))
	shaBefore := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "HEAD"))

	path := svc.PathFor(clone, "pr-7", 7)
	if err := svc.CheckoutPR(ctx, clone, path, headOID, "pr-7-head", 7); err != nil {
		t.Fatalf("CheckoutPR: %v", err)
	}

	branchAfter := strings.TrimSpace(gitCmd(t, clone, "symbolic-ref", "--short", "HEAD"))
	shaAfter := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "HEAD"))
	if branchBefore != branchAfter {
		t.Errorf("source branch changed: before %q, after %q (GHREV-05)", branchBefore, branchAfter)
	}
	if shaBefore != shaAfter {
		t.Errorf("source HEAD sha changed: before %q, after %q (GHREV-05)", shaBefore, shaAfter)
	}
}

// TestCheckoutPRBranchInNewWorktreeOnly: the named head branch IS created now
// (GHREV-01), but it must live in the NEW worktree only — the SOURCE clone's
// CURRENT branch is unchanged and no PRE-EXISTING ref moved. (The new named
// branch legitimately appears in `git branch`; the invariant is the source HEAD
// is untouched, GHREV-05.)
func TestCheckoutPRBranchInNewWorktreeOnly(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote, headOID := makePRRemote(t, 3)
	clone := cloneRepo(t, remote)

	srcBranchBefore := strings.TrimSpace(gitCmd(t, clone, "symbolic-ref", "--short", "HEAD"))
	srcShaBefore := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "HEAD"))

	path := svc.PathFor(clone, "pr-3", 3)
	if err := svc.CheckoutPR(ctx, clone, path, headOID, "pr-3-head", 3); err != nil {
		t.Fatalf("CheckoutPR: %v", err)
	}

	// The new worktree is on the named branch.
	wtBranch := strings.TrimSpace(gitCmd(t, path, "symbolic-ref", "--short", "HEAD"))
	if wtBranch != "pr-3-head" {
		t.Errorf("worktree branch = %q, want %q", wtBranch, "pr-3-head")
	}
	// The SOURCE clone's current branch and HEAD are byte-for-byte unchanged.
	srcBranchAfter := strings.TrimSpace(gitCmd(t, clone, "symbolic-ref", "--short", "HEAD"))
	srcShaAfter := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "HEAD"))
	if srcBranchBefore != srcBranchAfter {
		t.Errorf("source branch changed: before %q, after %q", srcBranchBefore, srcBranchAfter)
	}
	if srcShaBefore != srcShaAfter {
		t.Errorf("source HEAD sha changed: before %q, after %q", srcShaBefore, srcShaAfter)
	}
}

// TestCheckoutPRCollisionFallsBackToPRBranch is the GHREV-05 collision proof: a
// fork PR whose head branch name (e.g. "master") already exists locally must
// fall back to pr/<n> and NEVER reuse or move the pre-existing local ref.
func TestCheckoutPRCollisionFallsBackToPRBranch(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote, headOID := makePRRemote(t, 8)
	clone := cloneRepo(t, remote)

	// Pre-create a local branch "master" pointing at the clone's current commit.
	someCommit := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "HEAD"))
	gitCmd(t, clone, "branch", "master", someCommit)

	path := svc.PathFor(clone, "pr-8", 8)
	if err := svc.CheckoutPR(ctx, clone, path, headOID, "master", 8); err != nil {
		t.Fatalf("CheckoutPR: %v", err)
	}

	// The new worktree is on pr/8 — NOT the colliding "master".
	wtBranch := strings.TrimSpace(gitCmd(t, path, "symbolic-ref", "--short", "HEAD"))
	if wtBranch != "pr/8" {
		t.Errorf("worktree branch = %q, want %q (collision fallback, GHREV-05)", wtBranch, "pr/8")
	}
	// The pre-existing local "master" ref is untouched — never reused, never moved.
	masterAfter := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "refs/heads/master"))
	if masterAfter != someCommit {
		t.Errorf("local master moved: was %q, now %q (must never be reused/moved, GHREV-05)", someCommit, masterAfter)
	}
}

func TestCheckoutPRPathCollision(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote, headOID := makePRRemote(t, 9)
	clone := cloneRepo(t, remote)

	path := svc.PathFor(clone, "pr-9", 9)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("pre-create leaf: %v", err)
	}
	err := svc.CheckoutPR(ctx, clone, path, headOID, "pr-9-head", 9)
	if err == nil {
		t.Fatal("CheckoutPR on existing leaf path succeeded, want error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error %q does not signal a path-already-exists collision", err)
	}
	// The pre-check must fire BEFORE git fetches — no worktree may register.
	if list := gitCmd(t, clone, "worktree", "list", "--porcelain"); strings.Contains(list, "worktree "+path) {
		t.Errorf("worktree %q registered despite the path pre-check:\n%s", path, list)
	}
}

func TestFetchRef(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	// A remote on trunk with a second branch "base-branch" to fetch.
	remote := makeRepoOn(t, "trunk")
	gitCmd(t, remote, "checkout", "-b", "base-branch")
	if err := os.WriteFile(filepath.Join(remote, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base.txt: %v", err)
	}
	gitCmd(t, remote, "add", "base.txt")
	gitCmd(t, remote, "commit", "-m", "base branch commit")
	baseOID := strings.TrimSpace(gitCmd(t, remote, "rev-parse", "HEAD"))
	gitCmd(t, remote, "checkout", "trunk")
	clone := cloneRepo(t, remote)

	if err := svc.FetchRef(ctx, clone, "base-branch"); err != nil {
		t.Fatalf("FetchRef: %v", err)
	}
	// After the fetch the remote-tracking ref origin/base-branch must resolve.
	got := strings.TrimSpace(gitCmd(t, clone, "rev-parse", "origin/base-branch"))
	if got != baseOID {
		t.Errorf("origin/base-branch = %q after FetchRef, want %q", got, baseOID)
	}
}

// TestFetchRefError: a fetch of a nonexistent ref returns an error but never
// panics — the caller (diff plan) decides to ignore it.
func TestFetchRefError(t *testing.T) {
	ctx := context.Background()
	svc := NewService(t.TempDir())
	remote := makeRepoOn(t, "trunk")
	clone := cloneRepo(t, remote)

	if err := svc.FetchRef(ctx, clone, "no-such-branch"); err == nil {
		t.Error("FetchRef of a nonexistent ref returned nil, want an error")
	}
}

// entryByPath returns the parsed Entry whose Path (filepath.Clean-matched)
// equals want, or fails the test.
func entryByPath(t *testing.T, entries []Entry, want string) Entry {
	t.Helper()
	for _, e := range entries {
		if filepath.Clean(e.Path) == filepath.Clean(want) {
			return e
		}
	}
	t.Fatalf("no Entry with Path %q in %+v", want, entries)
	return Entry{}
}

// TestListParsesRealGit exercises the porcelain parser against REAL git output
// (the Pattern-1 source of truth, verified on git 2.43.0): a main worktree plus
// a linked worktree, then a linked worktree whose dir was manually deleted
// (git's own reverse-orphan `prunable` + `detached` signal).
func TestListParsesRealGit(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t) // main worktree on "main"
	svc := NewService(t.TempDir())

	// A branch worktree, still on disk.
	branchPath, branchName := makeWorktree(t, svc, repo, "feature", 1)

	entries, err := svc.List(ctx, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// The main worktree is NOT filtered by List (faithful parser) — the API
	// layer excludes it. Both records must be present.
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2 (main + 1 linked): %+v", len(entries), entries)
	}

	main := entryByPath(t, entries, repo)
	if main.Branch != "refs/heads/main" {
		t.Errorf("main worktree Branch = %q, want %q", main.Branch, "refs/heads/main")
	}
	if main.HEAD == "" {
		t.Errorf("main worktree HEAD is empty, want a sha")
	}
	if main.Detached || main.Bare || main.Prunable {
		t.Errorf("main worktree flags wrong: %+v", main)
	}

	linked := entryByPath(t, entries, branchPath)
	if linked.Branch != "refs/heads/"+branchName {
		t.Errorf("linked worktree Branch = %q, want %q", linked.Branch, "refs/heads/"+branchName)
	}

	// Now manually delete the linked worktree's dir on disk — git reports it as
	// detached + prunable with a non-existent-location reason (RESEARCH Pattern 1).
	if err := os.RemoveAll(branchPath); err != nil {
		t.Fatalf("rm linked worktree dir: %v", err)
	}
	entries, err = svc.List(ctx, repo)
	if err != nil {
		t.Fatalf("List after dir removal: %v", err)
	}
	gone := entryByPath(t, entries, branchPath)
	if !gone.Prunable {
		t.Errorf("deleted-dir worktree Prunable = false, want true: %+v", gone)
	}
	if !gone.Detached {
		t.Errorf("deleted-dir worktree Detached = false, want true: %+v", gone)
	}
	if gone.Branch != "" {
		t.Errorf("deleted-dir worktree Branch = %q, want \"\" (git drops branch on prunable)", gone.Branch)
	}
	if !strings.Contains(gone.PrunableReason, "non-existent location") {
		t.Errorf("PrunableReason = %q, want it to mention the non-existent location", gone.PrunableReason)
	}
}

// TestListParseBuffer asserts the pure parse logic against crafted -z buffers,
// covering records that are awkward to reproduce deterministically with live
// git: a locked worktree with a reason, a bare main, and trailing-NUL hygiene
// (a trailing \x00 or \x00\x00 must NOT produce a spurious empty Entry).
func TestListParseBuffer(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Entry
	}{
		{
			name: "two records: main branch + detached prunable linked",
			in:   "worktree /repo\x00HEAD abc123\x00branch refs/heads/main\x00\x00worktree /wt\x00HEAD def456\x00detached\x00prunable gitdir file points to non-existent location\x00\x00",
			want: []Entry{
				{Path: "/repo", HEAD: "abc123", Branch: "refs/heads/main"},
				{Path: "/wt", HEAD: "def456", Detached: true, Prunable: true, PrunableReason: "gitdir file points to non-existent location"},
			},
		},
		{
			name: "feature branch ref preserved verbatim",
			in:   "worktree /wt\x00HEAD ff\x00branch refs/heads/feature/x\x00\x00",
			want: []Entry{{Path: "/wt", HEAD: "ff", Branch: "refs/heads/feature/x"}},
		},
		{
			name: "locked with reason",
			in:   "worktree /wt\x00HEAD ff\x00branch refs/heads/b\x00locked under maintenance\x00\x00",
			want: []Entry{{Path: "/wt", HEAD: "ff", Branch: "refs/heads/b", Locked: true, LockedReason: "under maintenance"}},
		},
		{
			name: "locked with no reason",
			in:   "worktree /wt\x00HEAD ff\x00branch refs/heads/b\x00locked\x00\x00",
			want: []Entry{{Path: "/wt", HEAD: "ff", Branch: "refs/heads/b", Locked: true, LockedReason: ""}},
		},
		{
			name: "bare main record has no branch",
			in:   "worktree /repo.git\x00bare\x00\x00",
			want: []Entry{{Path: "/repo.git", Bare: true}},
		},
		{
			name: "trailing double-NUL yields no spurious empty entry",
			in:   "worktree /repo\x00HEAD abc\x00branch refs/heads/main\x00\x00",
			want: []Entry{{Path: "/repo", HEAD: "abc", Branch: "refs/heads/main"}},
		},
		{
			name: "single trailing NUL is trimmed, no empty entry",
			in:   "worktree /repo\x00HEAD abc\x00branch refs/heads/main\x00",
			want: []Entry{{Path: "/repo", HEAD: "abc", Branch: "refs/heads/main"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWorktreeList(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("parseWorktreeList(%q) returned %d entries, want %d: %+v", tc.in, len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

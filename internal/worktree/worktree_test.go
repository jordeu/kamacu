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
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Never let host/global git config leak into any git call made by this
	// test binary — including the package's own exec.CommandContext calls.
	os.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	os.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	os.Exit(m.Run())
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

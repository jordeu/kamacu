package github

// Clone is tested against REAL git in t.TempDir() repos — no mocks for the
// success path (the fixture shape mirrors worktree_test.go). The failure path
// overrides the package-level cloneRunner seam so it never touches gh/network.
// TestMain isolates host git config from every git invocation and allows the
// file:// protocol the fixtures rely on.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Never let host/global git config leak into any git call made by this
	// test binary. The "global" config is a controlled temp file (not
	// /dev/null) because the clone fixtures need the file:// protocol allowed.
	dir, err := os.MkdirTemp("", "github-gitconfig")
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
// branch with one commit (file.txt). Mirrors worktree_test.go.
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

// gitClone runs `git clone` (the cloneRunner test seam swaps this in for the
// real-git success path, so Clone exercises its own success/cleanup logic
// without needing a live gh). Returns trimmed stderr + error, matching the
// realClone contract.
func gitClone(ctx context.Context, ref, dest string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", ref, dest)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.Getenv("GIT_CONFIG_GLOBAL"), "GIT_CONFIG_SYSTEM=/dev/null")
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(errb.String()), err
	}
	return "", nil
}

// TestCloneSuccess clones a real file:// source repo into a fresh dest and
// asserts the dest is a git repo with origin set. Uses the cloneRunner seam to
// run `git clone` (no gh dependency) while exercising Clone's own success path.
func TestCloneSuccess(t *testing.T) {
	src := makeRepoOn(t, "main")

	orig := cloneRunner
	cloneRunner = gitClone
	defer func() { cloneRunner = orig }()

	dest := filepath.Join(t.TempDir(), "sub", "clone")
	if err := Clone(context.Background(), "file://"+src, dest); err != nil {
		t.Fatalf("Clone success: unexpected error: %v", err)
	}

	// dest must be a git repo.
	cmd := exec.Command("git", "-C", dest, "rev-parse", "--git-dir")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dest is not a git repo: %v\n%s", err, out)
	}
	// origin must be set to the source.
	got := strings.TrimSpace(gitCmd(t, dest, "remote", "get-url", "origin"))
	if got != "file://"+src {
		t.Fatalf("origin = %q, want %q", got, "file://"+src)
	}
}

// TestCloneFailureRemovesDest overrides the runner with a fake that returns a
// canned error and asserts Clone returns an error carrying the trimmed stderr
// AND that dest does not exist afterward (os.RemoveAll cleaned it).
func TestCloneFailureRemovesDest(t *testing.T) {
	orig := cloneRunner
	cloneRunner = func(ctx context.Context, ref, dest string) (string, error) {
		// Simulate git creating a partial dir before failing.
		_ = os.MkdirAll(dest, 0o755)
		return "boom", errors.New("boom")
	}
	defer func() { cloneRunner = orig }()

	dest := filepath.Join(t.TempDir(), "managed", "owner", "name")
	err := Clone(context.Background(), "owner/name", dest)
	if err == nil {
		t.Fatal("Clone failure: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Clone error = %q, want it to contain %q", err.Error(), "boom")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("dest still exists after failure (stat err = %v); os.RemoveAll did not clean up", statErr)
	}
}

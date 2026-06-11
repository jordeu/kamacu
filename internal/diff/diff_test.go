package diff

// Compute is tested against REAL git in t.TempDir() repos — no mocks. Every
// command/exit-code/format relied on here was empirically verified on git
// 2.43.0 in 05-RESEARCH.md. TestMain isolates host git config from every git
// call this binary makes (the helpers AND Compute's own exec); helper commands
// additionally pass -c user.name/-c user.email so commits work config-free.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Never let host/global git config leak into any git call (Phase 3 lesson).
	dir, err := os.MkdirTemp("", "diff-gitconfig")
	if err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	os.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	// Touch the file so git sees an empty (not missing) global config.
	_ = os.WriteFile(filepath.Join(dir, "gitconfig"), nil, 0o644)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// git runs git in dir with a throwaway identity, failing the test on nonzero
// exit. Used only by the fixture builders, never by the package under test.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@test"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.Getenv("GIT_CONFIG_GLOBAL"), "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// findFile returns the *File for path, or nil.
func findFile(d *Diff, path string) *File {
	for i := range d.Files {
		if d.Files[i].Path == path {
			return &d.Files[i]
		}
	}
	return nil
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

// fullFixture builds the D-59 scenario and returns (worktreePath, base):
//   - repo on main with committed text + a binary file
//   - task-branch worktree off main
//   - in the worktree: a committed change, a staged change, an unstaged change,
//     a staged NEW file, an untracked file
//   - main then advances by one commit (its new lines must appear NOWHERE)
func fullFixture(t *testing.T) (wt, base string) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")

	write(t, repo, "tracked.txt", "line1\nline2\nline3\n")
	write(t, repo, "willedit.txt", "alpha\nbeta\ngamma\n")
	write(t, repo, "unstaged.txt", "uno\ndos\ntres\n")
	// A small binary blob (4 NUL bytes) committed on the base.
	if err := os.WriteFile(filepath.Join(repo, "logo.bin"), []byte{0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("write logo.bin: %v", err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base commit")

	// Worktree off main.
	parent := t.TempDir()
	wt = filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task-branch", wt, "main")

	// 1. Committed change on the task branch.
	write(t, wt, "tracked.txt", "line1\nCOMMITTED-CHANGE\nline3\n")
	git(t, wt, "add", "tracked.txt")
	git(t, wt, "commit", "-m", "task work")

	// 2. Staged change (modify + add, not committed).
	write(t, wt, "willedit.txt", "alpha\nSTAGED-CHANGE\ngamma\n")
	git(t, wt, "add", "willedit.txt")

	// 3. Unstaged change (modify, not added).
	write(t, wt, "unstaged.txt", "uno\nUNSTAGED-CHANGE\ntres\n")

	// 4. Staged NEW file.
	write(t, wt, "stagednew.txt", "fresh staged file\n")
	git(t, wt, "add", "stagednew.txt")

	// 5. Untracked file (never added).
	write(t, wt, "untracked.txt", "brand new untracked\n")

	// Advance main by one commit AFTER branching — three-dot semantics must
	// exclude this from the diff entirely.
	write(t, repo, "mainonly.txt", "MAIN-ADVANCED-LINE\n")
	git(t, repo, "add", "mainonly.txt")
	git(t, repo, "commit", "-m", "main advances")

	return wt, "main"
}

func TestComputeFullPicture(t *testing.T) {
	requireGit(t)
	wt, base := fullFixture(t)

	d, err := Compute(context.Background(), wt, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	// The five text files plus the absence of main's advance.
	wantTextPaths := []string{"tracked.txt", "willedit.txt", "unstaged.txt", "stagednew.txt", "untracked.txt"}
	for _, p := range wantTextPaths {
		if findFile(d, p) == nil {
			t.Errorf("missing changed file %q in diff (files=%v)", p, fileNames(d))
		}
	}

	// Three-dot semantics: main's post-branch line must appear NOWHERE.
	if findFile(d, "mainonly.txt") != nil {
		t.Errorf("mainonly.txt present — base movement leaked into the diff (not three-dot)")
	}
	for _, f := range d.Files {
		for _, h := range f.Hunks {
			for _, ln := range h.Lines {
				if strings.Contains(ln.Text, "MAIN-ADVANCED-LINE") {
					t.Errorf("main's advanced line leaked into %q hunk: %q", f.Path, ln.Text)
				}
			}
		}
	}

	// Untracked file → status new, full-addition hunk.
	if u := findFile(d, "untracked.txt"); u != nil {
		if u.Status != "new" {
			t.Errorf("untracked.txt status = %q, want new", u.Status)
		}
		if len(u.Hunks) == 0 {
			t.Errorf("untracked.txt has no hunks — full-addition patch missing (exit-1 not handled?)")
		}
	}

	// Files sorted by path ascending.
	for i := 1; i < len(d.Files); i++ {
		if d.Files[i-1].Path > d.Files[i].Path {
			t.Errorf("files not sorted by path: %q before %q", d.Files[i-1].Path, d.Files[i].Path)
		}
	}

	// Totals.Files counts all changed files.
	if d.Totals.Files != len(d.Files) {
		t.Errorf("Totals.Files = %d, want len(files) = %d", d.Totals.Files, len(d.Files))
	}
}

func TestComputeBinary(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "logo.bin"), []byte{0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("write logo.bin: %v", err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")

	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")
	// Change the committed binary on the task branch.
	if err := os.WriteFile(filepath.Join(wt, "logo.bin"), []byte{1, 2, 3, 4, 5, 6}, 0o644); err != nil {
		t.Fatalf("rewrite logo.bin: %v", err)
	}
	git(t, wt, "commit", "-am", "change binary")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	b := findFile(d, "logo.bin")
	if b == nil {
		t.Fatalf("logo.bin missing from diff (files=%v)", fileNames(d))
	}
	if !b.Binary {
		t.Errorf("logo.bin Binary = false, want true")
	}
	if b.Additions != nil || b.Deletions != nil {
		t.Errorf("binary additions/deletions = %v/%v, want nil/nil", b.Additions, b.Deletions)
	}
	if len(b.Hunks) != 0 {
		t.Errorf("binary hunks = %d, want 0", len(b.Hunks))
	}
}

func TestComputePristine(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if d.Files == nil {
		t.Errorf("Files is nil, want non-nil empty slice (D-63 empty state)")
	}
	if len(d.Files) != 0 {
		t.Errorf("Files = %v, want empty on a pristine worktree", fileNames(d))
	}
	if d.Totals.Files != 0 || d.Totals.Additions != 0 || d.Totals.Deletions != 0 {
		t.Errorf("Totals = %+v, want all zero", d.Totals)
	}
}

func TestComputeUntrackedOnly(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")
	write(t, wt, "new.txt", "only untracked\n")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute (untracked only): %v", err)
	}
	f := findFile(d, "new.txt")
	if f == nil {
		t.Fatalf("new.txt missing — no-index exit-1 path not handled (files=%v)", fileNames(d))
	}
	if f.Status != "new" {
		t.Errorf("new.txt status = %q, want new", f.Status)
	}
	if f.Additions == nil || *f.Additions != 1 {
		t.Errorf("new.txt additions = %v, want 1", f.Additions)
	}
}

func TestComputeBogusBase(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")

	_, err := Compute(context.Background(), wt, "nonexistent-branch-xyz")
	if err == nil {
		t.Fatalf("Compute with bogus base returned nil error, want a git stderr relay")
	}
}

func fileNames(d *Diff) []string {
	var names []string
	for _, f := range d.Files {
		names = append(names, f.Path)
	}
	return names
}

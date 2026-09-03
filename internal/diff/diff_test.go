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

// pushFixture builds the commit-state scenario and returns its worktree:
//   - repo on main with five text files, a bare clone wired up as origin
//   - task-branch worktree off main whose first commit is PUSHED
//   - after the push: one more commit (ahead of origin/task), a worktree edit
//     on a pushed file, a staged edit, and an untracked file
//
// Expected markers per file (asserted by the tests below):
//
//	committed.txt  committed + pushed        → no markers
//	dirty.txt      pushed, then edited       → uncommitted only
//	ahead.txt      committed after push      → unpushed only
//	both.txt       committed after push AND
//	               then edited               → BOTH markers
//	staged.txt     staged edit               → uncommitted only
//	untracked.txt  untracked                 → uncommitted only (never unpushed)
func pushFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "committed.txt", "base\n")
	write(t, repo, "dirty.txt", "base\n")
	write(t, repo, "ahead.txt", "base\n")
	write(t, repo, "both.txt", "base\n")
	write(t, repo, "staged.txt", "base\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")

	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, repo, "clone", "--bare", repo, origin)
	git(t, repo, "remote", "add", "origin", origin)

	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")

	// Committed changes that ARE pushed.
	write(t, wt, "committed.txt", "committed and pushed\n")
	write(t, wt, "dirty.txt", "pushed then dirtied\n")
	git(t, wt, "add", "-A")
	git(t, wt, "commit", "-m", "pushed work")
	git(t, wt, "push", "-u", "origin", "task")

	// Committed AFTER the push — ahead of origin/task.
	write(t, wt, "ahead.txt", "committed after push\n")
	write(t, wt, "both.txt", "committed after push, then dirtied\n")
	git(t, wt, "add", "-A")
	git(t, wt, "commit", "-m", "local work")

	// Worktree-only edits — the `git status` signal.
	write(t, wt, "dirty.txt", "pushed then dirtied\nWORKTREE-EDIT\n")
	write(t, wt, "both.txt", "committed after push, then dirtied\nWORKTREE-EDIT\n")
	write(t, wt, "staged.txt", "base\nSTAGED-EDIT\n")
	git(t, wt, "add", "staged.txt")
	write(t, wt, "untracked.txt", "untracked\n")

	return wt
}

// marker returns the (uncommitted, unpushed) flags of path in d, failing the
// test when the path is missing from the diff.
func marker(t *testing.T, d *Diff, path string) (uncommitted, unpushed bool) {
	t.Helper()
	f := findFile(d, path)
	if f == nil {
		t.Fatalf("%s missing from diff (files=%v)", path, fileNames(d))
	}
	return f.Uncommitted, f.Unpushed
}

func TestComputeUncommittedMarker(t *testing.T) {
	requireGit(t)
	wt := pushFixture(t)

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	cases := []struct {
		path     string
		want     bool
		wantPush bool
	}{
		{"committed.txt", false, false},
		{"dirty.txt", true, false},
		{"staged.txt", true, false},
		{"untracked.txt", true, false},
	}
	for _, c := range cases {
		uc, up := marker(t, d, c.path)
		if uc != c.want {
			t.Errorf("%s Uncommitted = %v, want %v", c.path, uc, c.want)
		}
		if up != c.wantPush {
			t.Errorf("%s Unpushed = %v, want %v", c.path, up, c.wantPush)
		}
	}

	// Totals count the marked files: dirty + staged + untracked + both.
	if d.Totals.Uncommitted != 4 {
		t.Errorf("Totals.Uncommitted = %d, want 4", d.Totals.Uncommitted)
	}
}

func TestComputeUnpushedMarkerPushedBranch(t *testing.T) {
	requireGit(t)
	wt := pushFixture(t)

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	cases := []struct {
		path       string
		wantCommit bool
		want       bool
	}{
		{"committed.txt", false, false},
		{"dirty.txt", true, false},
		{"ahead.txt", false, true},
		{"both.txt", true, true},
		{"untracked.txt", true, false},
	}
	for _, c := range cases {
		uc, up := marker(t, d, c.path)
		if up != c.want {
			t.Errorf("%s Unpushed = %v, want %v", c.path, up, c.want)
		}
		if uc != c.wantCommit {
			t.Errorf("%s Uncommitted = %v, want %v", c.path, uc, c.wantCommit)
		}
	}

	// Totals count the marked files: ahead + both.
	if d.Totals.Unpushed != 2 {
		t.Errorf("Totals.Unpushed = %d, want 2", d.Totals.Unpushed)
	}
}

func TestComputeUnpushedMarkerNeverPushedBranch(t *testing.T) {
	requireGit(t)
	// Origin exists, but the task branch was never pushed: every committed
	// change is local-only and must carry the unpushed marker.
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "base\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, repo, "clone", "--bare", repo, origin)
	git(t, repo, "remote", "add", "origin", origin)

	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")
	write(t, wt, "a.txt", "committed locally\n")
	git(t, wt, "commit", "-am", "never pushed")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if uc, up := marker(t, d, "a.txt"); uc || !up {
		t.Errorf("a.txt markers = (uncommitted=%v, unpushed=%v), want (false, true) on a never-pushed branch", uc, up)
	}
}

func TestComputeUnpushedMarkerNoRemote(t *testing.T) {
	requireGit(t)
	// Folder-style repo with no remote at all: "pushed" has no meaning, so no
	// unpushed marker may appear even though the commit exists only locally.
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "base\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")
	write(t, wt, "a.txt", "committed locally\n")
	git(t, wt, "commit", "-am", "local only")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if uc, up := marker(t, d, "a.txt"); uc || up {
		t.Errorf("a.txt markers = (uncommitted=%v, unpushed=%v), want (false, false) with no remote", uc, up)
	}
}

func TestComputeMarkersDetachedHead(t *testing.T) {
	requireGit(t)
	// PR-review worktrees check out a detached HEAD: no branch, no push
	// concept — the unpushed marker degrades away while uncommitted still
	// works.
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "base\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "--detach", wt, "main")
	write(t, wt, "a.txt", "committed locally\n")
	git(t, wt, "commit", "-am", "detached work")
	write(t, wt, "a.txt", "committed locally\nWORKTREE-EDIT\n")

	d, err := Compute(context.Background(), wt, "main")
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if uc, up := marker(t, d, "a.txt"); !uc || up {
		t.Errorf("a.txt markers = (uncommitted=%v, unpushed=%v), want (true, false) on detached HEAD", uc, up)
	}
}

func TestCommitStateExcludedFromHash(t *testing.T) {
	requireGit(t)
	// Committing and pushing never change the rendered diff, so they must not
	// change the Viewed-keying hash — otherwise every agent commit would reset
	// the user's review state.
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "a.txt", "base\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")
	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, repo, "clone", "--bare", repo, origin)
	git(t, repo, "remote", "add", "origin", origin)
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task", wt, "main")

	steps := []struct {
		name string
		run  func()
	}{
		{"uncommitted", func() { write(t, wt, "a.txt", "changed\n") }},
		{"committed", func() { git(t, wt, "commit", "-am", "commit the change") }},
		{"pushed", func() { git(t, wt, "push", "-u", "origin", "task") }},
	}

	var prev string
	for _, s := range steps {
		s.run()
		d, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute (%s): %v", s.name, err)
		}
		h := findFile(d, "a.txt").Hash
		if prev != "" && h != prev {
			t.Fatalf("hash changed at step %q (%q → %q) — commit state leaked into the Viewed hash", s.name, prev, h)
		}
		prev = h
	}
	if prev == "" {
		t.Fatalf("no hash computed for a.txt")
	}
}

// TestFileHash pins the rendered-per-file hash contract that keys the "Viewed"
// persistence (DIFF-03) and drives auto-reset (DIFF-04). Every assertion is on a
// RELATIONSHIP between hashes (equal / not-equal), never a hard-coded hex literal,
// so the test survives any future reshaping of the serialization that preserves
// its invariants. The real-git cases run through Compute (the production path);
// the rename-vs-modify case calls hashFile directly (in-package) to prove that
// Status/OldPath participate without fighting git's rename-detection heuristics.
func TestFileHash(t *testing.T) {
	requireGit(t)

	// oneCommitRepo returns a repo on main carrying the given text file, plus a
	// task-branch worktree off main (mirrors the other fixtures' shape).
	oneCommitRepo := func(t *testing.T, name, content string) (repo, wt string) {
		t.Helper()
		repo = t.TempDir()
		git(t, repo, "init", "-b", "main")
		write(t, repo, name, content)
		git(t, repo, "add", "-A")
		git(t, repo, "commit", "-m", "base")
		wt = filepath.Join(t.TempDir(), "wt")
		git(t, repo, "worktree", "add", "-b", "task", wt, "main")
		return repo, wt
	}

	t.Run("deterministic: same content hashes identically across two Computes", func(t *testing.T) {
		_, wt := oneCommitRepo(t, "a.txt", "one\ntwo\nthree\n")
		write(t, wt, "a.txt", "one\nCHANGED\nthree\n")

		d1, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #1: %v", err)
		}
		d2, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #2: %v", err)
		}

		// Every file carries a non-empty 64-char lowercase-hex hash.
		for _, f := range d1.Files {
			if len(f.Hash) != 64 {
				t.Errorf("%s: hash %q is not 64 chars (sha256 hex)", f.Path, f.Hash)
			}
			for _, r := range f.Hash {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
					t.Errorf("%s: hash %q is not lowercase hex", f.Path, f.Hash)
					break
				}
			}
		}

		f1, f2 := findFile(d1, "a.txt"), findFile(d2, "a.txt")
		if f1 == nil || f2 == nil {
			t.Fatalf("a.txt missing (files=%v / %v)", fileNames(d1), fileNames(d2))
		}
		if f1.Hash != f2.Hash {
			t.Errorf("non-deterministic hash for unchanged content: %q != %q", f1.Hash, f2.Hash)
		}
	})

	t.Run("content-sensitive: editing a file changes its hash", func(t *testing.T) {
		_, wt := oneCommitRepo(t, "a.txt", "one\ntwo\nthree\n")

		write(t, wt, "a.txt", "one\nCHANGED\nthree\n")
		d1, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #1: %v", err)
		}
		h1 := findFile(d1, "a.txt").Hash

		write(t, wt, "a.txt", "one\nCHANGED-AGAIN\nthree\n")
		d2, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #2: %v", err)
		}
		h2 := findFile(d2, "a.txt").Hash

		if h1 == h2 {
			t.Errorf("editing a.txt did not change its hash (%q) — DIFF-04 auto-reset would never fire", h1)
		}
	})

	t.Run("base movement that does not touch F leaves F's hash unchanged (D-01)", func(t *testing.T) {
		repo, wt := oneCommitRepo(t, "a.txt", "one\ntwo\nthree\n")
		write(t, wt, "a.txt", "one\nCHANGED\nthree\n")

		d1, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #1: %v", err)
		}
		h1 := findFile(d1, "a.txt").Hash

		// Advance main with a commit that never touches a.txt. Three-dot
		// semantics keep the merge-base at the fork point, so a.txt's rendered
		// diff — and therefore its hash — must not move (D-01: base movement
		// does not reset Viewed).
		write(t, repo, "unrelated.txt", "MAIN-ADVANCED-LINE\n")
		git(t, repo, "add", "unrelated.txt")
		git(t, repo, "commit", "-m", "main advances, a.txt untouched")

		d2, err := Compute(context.Background(), wt, "main")
		if err != nil {
			t.Fatalf("Compute #2: %v", err)
		}
		if findFile(d2, "unrelated.txt") != nil {
			t.Errorf("unrelated.txt leaked into the diff — base movement is not three-dot")
		}
		h2 := findFile(d2, "a.txt").Hash
		if h1 != h2 {
			t.Errorf("unrelated base movement reset a.txt's hash: %q != %q (violates D-01)", h1, h2)
		}
	})

	t.Run("rename and modify with identical hunks hash differently (Status/OldPath participate)", func(t *testing.T) {
		// Identical rendered hunks; only Status and OldPath differ. hashFile MUST
		// distinguish them, else renaming a file to a path that once held
		// byte-identical hunks would wrongly restore its Viewed checkmark.
		hunks := []Hunk{{
			Header: "@@ -1,2 +1,2 @@",
			Lines: []Line{
				{Kind: "context", Text: " keep"},
				{Kind: "del", Text: "-old line"},
				{Kind: "add", Text: "+new line"},
			},
		}}
		oldPath := "src/old-name.txt"
		renamed := File{Path: "src/new-name.txt", Status: "renamed", OldPath: &oldPath, Hunks: hunks}
		modified := File{Path: "src/new-name.txt", Status: "modified", Hunks: hunks}

		if hashFile(renamed) == hashFile(modified) {
			t.Errorf("rename and modify with identical hunks hashed identically — Status/OldPath are not participating in the hash")
		}
		// Sanity anchor: two structurally identical Files hash identically.
		if hashFile(modified) != hashFile(File{Path: "src/new-name.txt", Status: "modified", Hunks: hunks}) {
			t.Errorf("two identical File values hashed differently — hashFile is not deterministic")
		}
	})

	t.Run("binary hash is stable across Computes and byte-invariant (strict D-01)", func(t *testing.T) {
		// A binary change renders a content-invariant "Binary files ... differ"
		// header with no hunks, so its hash depends only on Status/Binary/path —
		// never the bytes. This is the ACCEPTED, DOCUMENTED strict-D-01 limitation
		// (parse.go deliberately does NOT capture index <oid>..<oid> blob OIDs):
		// a binary whose bytes change without a status/path/header change keeps its
		// Viewed state. Do NOT "fix" this — the assertions below pin it as intended.
		binRepo := func(t *testing.T, bytesAfter []byte) (wt string) {
			t.Helper()
			repo := t.TempDir()
			git(t, repo, "init", "-b", "main")
			if err := os.WriteFile(filepath.Join(repo, "logo.bin"), []byte{0, 0, 0, 0}, 0o644); err != nil {
				t.Fatalf("write base logo.bin: %v", err)
			}
			git(t, repo, "add", "-A")
			git(t, repo, "commit", "-m", "base binary")
			wt = filepath.Join(t.TempDir(), "wt")
			git(t, repo, "worktree", "add", "-b", "task", wt, "main")
			if err := os.WriteFile(filepath.Join(wt, "logo.bin"), bytesAfter, 0o644); err != nil {
				t.Fatalf("rewrite logo.bin: %v", err)
			}
			git(t, wt, "commit", "-am", "change binary")
			return wt
		}

		wtA := binRepo(t, []byte{1, 2, 3, 4, 5, 6})
		dA1, err := Compute(context.Background(), wtA, "main")
		if err != nil {
			t.Fatalf("Compute A#1: %v", err)
		}
		dA2, err := Compute(context.Background(), wtA, "main")
		if err != nil {
			t.Fatalf("Compute A#2: %v", err)
		}
		bA1, bA2 := findFile(dA1, "logo.bin"), findFile(dA2, "logo.bin")
		if bA1 == nil || bA2 == nil {
			t.Fatalf("logo.bin missing (files=%v / %v)", fileNames(dA1), fileNames(dA2))
		}
		if !bA1.Binary {
			t.Errorf("logo.bin Binary = false, want true")
		}
		if bA1.Hash == "" {
			t.Errorf("binary file has an empty hash")
		}
		if bA1.Hash != bA2.Hash {
			t.Errorf("binary hash not stable across two Computes: %q != %q", bA1.Hash, bA2.Hash)
		}

		// Different bytes, same rendered header → same hash (strict D-01).
		wtB := binRepo(t, []byte{9, 8, 7, 6, 5, 4, 3, 2})
		dB, err := Compute(context.Background(), wtB, "main")
		if err != nil {
			t.Fatalf("Compute B: %v", err)
		}
		bB := findFile(dB, "logo.bin")
		if bB == nil {
			t.Fatalf("logo.bin missing from B (files=%v)", fileNames(dB))
		}
		if bA1.Hash != bB.Hash {
			t.Errorf("binary hash changed with an only-bytes change: %q != %q — strict D-01 requires the rendered-only hash to stay stable", bA1.Hash, bB.Hash)
		}
	})
}

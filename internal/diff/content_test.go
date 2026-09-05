package diff

// Content is tested against REAL git in t.TempDir() repos, same no-mocks
// posture as diff_test.go (TestMain there isolates host git config for this
// whole package, and the shared git/write helpers carry throwaway identities).
// The fixture mirrors the Markdown viewer's three file shapes: a modified
// .md, a deleted .md, an untracked .md, plus an unchanged sentinel to pin
// ErrNotInDiff.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// contentFixture builds a main repo with SPEC.md + UNTOUCHED.md + GONE.md,
// then a task-branch worktree that rewrites a SPEC.md line (committed) and
// deletes GONE.md (committed), leaving NEW.md untracked. Returns the worktree
// and the base ref ("main").
func contentFixture(t *testing.T) (wt, base string) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "SPEC.md", "# Spec\n\nold line\nnew-ish line\n")
	write(t, repo, "UNTOUCHED.md", "stable\n")
	write(t, repo, "GONE.md", "delete me\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")

	parent := t.TempDir()
	wt = filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task-branch", wt, "main")

	write(t, wt, "SPEC.md", "# Spec\n\nrewritten line\nnew-ish line\n")
	git(t, wt, "rm", "-q", "GONE.md")
	git(t, wt, "add", "SPEC.md")
	git(t, wt, "commit", "-m", "task work")
	write(t, wt, "NEW.md", "# Fresh\n")
	return wt, "main"
}

func TestContentModified(t *testing.T) {
	requireGit(t)
	wt, base := contentFixture(t)

	fc, err := Content(context.Background(), wt, base, "SPEC.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if fc.Status != "modified" {
		t.Errorf("status = %q, want modified", fc.Status)
	}
	if fc.OldText == nil || *fc.OldText != "# Spec\n\nold line\nnew-ish line\n" {
		t.Errorf("oldText = %v, want the merge-base version", fc.OldText)
	}
	if fc.NewText == nil || *fc.NewText != "# Spec\n\nrewritten line\nnew-ish line\n" {
		t.Errorf("newText = %v, want the worktree version", fc.NewText)
	}
}

func TestContentUntrackedIsNew(t *testing.T) {
	requireGit(t)
	wt, base := contentFixture(t)

	fc, err := Content(context.Background(), wt, base, "NEW.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if fc.Status != "new" {
		t.Errorf("status = %q, want new", fc.Status)
	}
	if fc.OldText != nil {
		t.Errorf("oldText = %v, want nil for an untracked file", fc.OldText)
	}
	if fc.NewText == nil || *fc.NewText != "# Fresh\n" {
		t.Errorf("newText = %v, want the worktree version", fc.NewText)
	}
}

func TestContentDeleted(t *testing.T) {
	requireGit(t)
	wt, base := contentFixture(t)

	fc, err := Content(context.Background(), wt, base, "GONE.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if fc.Status != "deleted" {
		t.Errorf("status = %q, want deleted", fc.Status)
	}
	if fc.NewText != nil {
		t.Errorf("newText = %v, want nil for a deleted file", fc.NewText)
	}
	if fc.OldText == nil || *fc.OldText != "delete me\n" {
		t.Errorf("oldText = %v, want the merge-base version", fc.OldText)
	}
}

func TestContentNotInDiff(t *testing.T) {
	requireGit(t)
	wt, base := contentFixture(t)

	// Unchanged in the diff…
	if _, err := Content(context.Background(), wt, base, "UNTOUCHED.md"); !errors.Is(err, ErrNotInDiff) {
		t.Errorf("unchanged file: err = %v, want ErrNotInDiff", err)
	}
	// …and never tracked at all (both sides absent).
	if _, err := Content(context.Background(), wt, base, "NOPE.md"); !errors.Is(err, ErrNotInDiff) {
		t.Errorf("nonexistent file: err = %v, want ErrNotInDiff", err)
	}
}

func TestContentTooLarge(t *testing.T) {
	requireGit(t)
	wt, base := contentFixture(t)
	// An untracked .md over the cap (only the new side exists — enough).
	write(t, wt, "HUGE.md", strings.Repeat("x", maxContentBytes+1)+"\n")
	if _, err := Content(context.Background(), wt, base, "HUGE.md"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized file: err = %v, want ErrTooLarge", err)
	}
}

func TestContentRenamedNoTextualChange(t *testing.T) {
	requireGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	write(t, repo, "OLD-NAME.md", "# Same content\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "base")

	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	git(t, repo, "worktree", "add", "-b", "task-branch", wt, "main")
	git(t, wt, "mv", "OLD-NAME.md", "NEW-NAME.md")
	git(t, wt, "commit", "-m", "pure rename")

	fc, err := Content(context.Background(), wt, "main", "NEW-NAME.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if fc.Status != "renamed" {
		t.Errorf("status = %q, want renamed", fc.Status)
	}
	// No textual change: both sides carry the same bytes (from the old path
	// and the new one respectively).
	if fc.OldText == nil || *fc.OldText != "# Same content\n" {
		t.Errorf("oldText = %v, want the old-path text", fc.OldText)
	}
	if fc.NewText == nil || *fc.NewText != "# Same content\n" {
		t.Errorf("newText = %v, want the new-path text", fc.NewText)
	}
}

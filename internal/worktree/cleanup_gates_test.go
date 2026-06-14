package worktree

// Hermetic tests for the reaper's conservative-gate primitives (D-05 gates
// b/c). Real git in t.TempDir() repos, no network, no gh — mirroring the
// existing worktree_test.go git fixtures (makeRepo/makeWorktree/gitCmd).

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestUnpushedCount: rev-list <base>..HEAD. On a fresh worktree there are zero
// commits ahead of HEAD; after a LOCAL commit (invisible to porcelain — the
// whole reason this gate exists, Pitfall 1) the count against the pre-commit
// sha is >= 1.
func TestUnpushedCount(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, _ := makeWorktree(t, svc, repo, "unpushed", 1)

	// Clean worktree: base==HEAD -> 0 commits ahead.
	n, err := svc.UnpushedCount(ctx, path, "HEAD")
	if err != nil {
		t.Fatalf("UnpushedCount clean: %v", err)
	}
	if n != 0 {
		t.Errorf("UnpushedCount(HEAD) on a clean worktree = %d, want 0", n)
	}

	// Capture the pre-commit sha, then make a LOCAL commit in the worktree.
	preSha := gitCmd(t, path, "rev-parse", "HEAD")
	preSha = preSha[:len(preSha)-1] // strip the trailing newline gitCmd returns
	if err := os.WriteFile(filepath.Join(path, "fixup.txt"), []byte("local-only\n"), 0o644); err != nil {
		t.Fatalf("write fixup.txt: %v", err)
	}
	gitCmd(t, path, "add", "fixup.txt")
	gitCmd(t, path, "commit", "-m", "local fixup")

	// status --porcelain is CLEAN here (the committed fixup is invisible) — the
	// rev-list gate is the only thing that catches it.
	if dirty, err := svc.DirtyCount(ctx, path); err != nil || dirty != 0 {
		t.Fatalf("DirtyCount after local commit = %d (err=%v), want 0 (Pitfall 1 premise)", dirty, err)
	}
	n, err = svc.UnpushedCount(ctx, path, preSha)
	if err != nil {
		t.Fatalf("UnpushedCount after local commit: %v", err)
	}
	if n < 1 {
		t.Errorf("UnpushedCount(preSha) after a local commit = %d, want >= 1 (gate b)", n)
	}
}

// TestStashCount: 0 with no stash, >= 1 after a tracked-file change is stashed.
func TestStashCount(t *testing.T) {
	ctx := context.Background()
	repo := makeRepo(t)
	svc := NewService(t.TempDir())
	path, _ := makeWorktree(t, svc, repo, "stash", 1)

	n, err := svc.StashCount(ctx, path)
	if err != nil {
		t.Fatalf("StashCount no stash: %v", err)
	}
	if n != 0 {
		t.Errorf("StashCount with no stash = %d, want 0", n)
	}

	// Modify a tracked file (file.txt exists from makeRepo) and stash it.
	if err := os.WriteFile(filepath.Join(path, "file.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	gitCmd(t, path, "stash", "push", "-m", "wip")

	n, err = svc.StashCount(ctx, path)
	if err != nil {
		t.Fatalf("StashCount after stash: %v", err)
	}
	if n < 1 {
		t.Errorf("StashCount after stashing a change = %d, want >= 1 (gate c)", n)
	}
}

package diff

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Content size cap. The Markdown viewer renders prose documents; anything past
// 2 MiB of text is not a document a human reads in a popup, and unbounded
// reads would let a pathological file balloon the JSON response.
const maxContentBytes = 2 << 20

// ErrNotInDiff reports that the requested path is not part of the task's diff
// (unchanged, binary, or never tracked). The API layer maps it to a 404 — the
// viewer affordance only exists for files the diff GET listed, so reaching it
// means the diff changed between the two calls (or a hand-crafted request).
var ErrNotInDiff = errors.New("path not in diff")

// ErrTooLarge reports one of the texts exceeded maxContentBytes.
var ErrTooLarge = errors.New("file is too large to render")

// FileContent is the full two-sided text of ONE file's diff — the payload of
// GET /api/tasks/{id}/diff/content (the Diff tab's Markdown viewer). The
// hunks in the main diff response carry only ±3 context lines, so the NEW
// document (and the OLD text of a between-hunks deletion) cannot be
// reconstructed client-side; this supplies both sides whole and lets the
// client map the hunks it already has onto them.
//
//	OldText = `git show <mb>:<oldPath ?? path>` — the diff's OLD side is the
//	          merge-base version by construction; nil for added files.
//	NewText = the worktree file on disk — the diff's NEW side (commits +
//	          staged + unstaged + untracked vs the merge-base is exactly the
//	          working tree); nil for deleted files.
type FileContent struct {
	Status  string  `json:"status"` // modified | new | deleted | renamed
	OldText *string `json:"oldText"`
	NewText *string `json:"newText"`
}

// Content returns the two full texts for one file in the worktree wt vs base
// (same base the diff GET resolves — callers share the resolution chain).
// The file's status/oldPath come from Compute itself — NOT a pathspec-limited
// single-file diff — because rename detection needs both sides of the tree:
// `git diff mb -- newpath` hides the old side and misreads a pure rename as a
// plain add, disagreeing with the diff the client is rendering. Running the
// same pipeline keeps the two endpoints byte-consistent (and is the same
// on-demand cost the diff GET already pays, D-61).
func Content(ctx context.Context, wt, base, path string) (*FileContent, error) {
	d, err := Compute(ctx, wt, base)
	if err != nil {
		return nil, err
	}
	// The old side of `git diff <mb>` is the MERGE-BASE version, not the
	// base ref's tip (three-dot semantics — base movement is excluded), so
	// resolve the merge-base sha the same way Compute does.
	mbOut, err := run(ctx, wt, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	mb := strings.TrimSpace(mbOut)

	var f *File
	for i := range d.Files {
		if d.Files[i].Path == path {
			f = &d.Files[i]
			break
		}
	}
	if f == nil || f.Binary {
		return nil, ErrNotInDiff
	}

	fc := &FileContent{Status: f.Status}

	// Old side: the merge-base blob of the old path for renames.
	if f.Status != "new" {
		oldPath := path
		if f.OldPath != nil {
			oldPath = *f.OldPath
		}
		rev := mb + ":" + oldPath
		if objectExists(ctx, wt, rev) {
			s, err := run(ctx, wt, "show", rev)
			if err != nil {
				return nil, err
			}
			if len(s) > maxContentBytes {
				return nil, ErrTooLarge
			}
			fc.OldText = &s
		}
	}

	// New side: the worktree file on disk. path is pre-validated by the
	// handler (relative, no .., bounded), so Join cannot escape wt.
	if b, err := os.ReadFile(filepath.Join(wt, path)); err == nil {
		if len(b) > maxContentBytes {
			return nil, ErrTooLarge
		}
		s := string(b)
		fc.NewText = &s
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	// Defensive: a diff file always has at least one side; neither means the
	// diff changed between the Compute above and the reads (or a hand-crafted
	// request that dodged the earlier checks).
	if fc.OldText == nil && fc.NewText == nil {
		return nil, ErrNotInDiff
	}

	return fc, nil
}

// objectExists is a quiet, network-free check that rev (e.g. "<ref>:<path>")
// resolves to an object (cat-file -e exit 0). Non-zero exit means "absent",
// not an error — callers treat it as false. Same posture as refExists.
func objectExists(ctx context.Context, wt, rev string) bool {
	return exec.CommandContext(ctx, "git", "-C", wt, "cat-file", "-e", rev).Run() == nil
}

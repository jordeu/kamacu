package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// run executes `git -C dir args...` and returns stdout. Failure is any nonzero
// exit; the returned error is git's stderr, trimmed and with the "fatal: "
// prefix stripped — it surfaces verbatim in the UI error card (D-59 / Pitfall
// 7). Mirrors internal/worktree.gitRun deliberately: same exit-0-only contract,
// same error shaping. The diff package keeps its OWN copy because the untracked
// path needs runNoIndex's exit-1 contract, which this runner must never grant.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// runNoIndex runs `git -C dir diff --no-index --no-color --no-ext-diff
// /dev/null <relpath>` — the ONLY way to render an untracked file as a full
// addition without mutating the user's index (never `git add -N`). It has its
// own exit-code contract (Pitfall 1, verified on git 2.43.0):
//
//	exit 0  → files identical (an empty untracked file) → ("", nil), caller skips
//	exit 1  → differences found → (stdout, nil) — THIS IS SUCCESS
//	exit >1 → a real error → ("", error with trimmed stderr)
//
// Routing --no-index through the shared exit-0 run() would make every untracked
// file "error". Hence this dedicated runner.
func runNoIndex(ctx context.Context, dir, relpath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir,
		"-c", "core.quotePath=false",
		"diff", "--no-index", "--no-color", "--no-ext-diff", "/dev/null", relpath)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return "", nil // exit 0: identical, nothing to show
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == 1 {
			return out.String(), nil // exit 1: differences found — success
		}
	}
	msg := strings.TrimSpace(errb.String())
	msg = strings.TrimPrefix(msg, "fatal: ")
	if msg == "" {
		msg = err.Error()
	}
	return "", fmt.Errorf("%s", msg)
}

// Compute returns the full D-59 picture for the worktree at wt vs base: every
// tracked change since the merge-base (commits + staged + unstaged) plus
// untracked files as full additions, base movement excluded by three-dot
// semantics. Base is NOT set on the result — the handler fills Diff.Base from
// ResolveBase so the totals bar and empty state share one value.
//
// Verified pipeline (05-RESEARCH.md §Diff Plumbing, git 2.43.0):
//
//	mb        := merge-base base HEAD                       # exit 128 → error
//	numstat   := diff --numstat -z <mb>                     # per-file stats + binary/rename
//	patch     := diff <mb>                                  # hunks, ONE call
//	untracked := ls-files --others --exclude-standard -z
//	per untracked: diff --no-index /dev/null <f>            # exit 1 = success
//
// numstat is the single source of truth for tracked-file additions/deletions
// (it also feeds the frontend's >400-line collapse gate — no server flag).
func Compute(ctx context.Context, wt, base string) (*Diff, error) {
	mbOut, err := run(ctx, wt, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err // base weirdness / unborn HEAD → UI error card
	}
	mb := strings.TrimSpace(mbOut)

	statsOut, err := run(ctx, wt, "-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--numstat", "-z", mb)
	if err != nil {
		return nil, err
	}
	patchOut, err := run(ctx, wt, "-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", mb)
	if err != nil {
		return nil, err
	}

	stats := parseNumstatZ(statsOut)
	sections := parsePatch(patchOut)

	// Index patch sections by path for correlation with numstat records.
	byPath := make(map[string]*File, len(sections))
	for i := range sections {
		byPath[sections[i].Path] = &sections[i]
	}

	var files []File
	for _, rec := range stats {
		f := File{Path: rec.Path, Status: "modified"}
		if sec := byPath[rec.Path]; sec != nil {
			// Patch section carries status, binary, hunks, oldPath.
			f.Status = sec.Status
			f.Binary = sec.Binary
			f.OldPath = sec.OldPath
			f.Hunks = sec.Hunks
		}
		if rec.Binary {
			f.Binary = true
		}
		if rec.OldPath != "" {
			old := rec.OldPath
			f.OldPath = &old
			f.Status = "renamed"
		}
		if f.Hunks == nil {
			f.Hunks = []Hunk{}
		}
		if !f.Binary {
			// numstat is the source of truth for tracked-file stats.
			add, del := rec.Added, rec.Deleted
			f.Additions = &add
			f.Deletions = &del
		}
		files = append(files, f)
	}

	// Untracked files: numstat does not cover them; render each as a full
	// addition via the no-index exit-1 path. Count +/- from the parsed patch.
	untrackedOut, err := run(ctx, wt, "-c", "core.quotePath=false",
		"ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, rel := range strings.Split(untrackedOut, "\x00") {
		if rel == "" {
			continue
		}
		patch, err := runNoIndex(ctx, wt, rel)
		if err != nil {
			return nil, err
		}
		if patch == "" {
			continue // empty file: identical to /dev/null, skip
		}
		secs := parsePatch(patch)
		if len(secs) == 0 {
			continue
		}
		f := secs[0]
		f.Path = rel // authoritative path (the no-index a//b/ encoding may differ)
		f.Status = "new"
		if f.Hunks == nil {
			f.Hunks = []Hunk{}
		}
		if !f.Binary {
			add, del := countHunkLines(f.Hunks)
			f.Additions = &add
			f.Deletions = &del
		}
		files = append(files, f)
	}

	// Stable ordering: sort everything by path ascending (planner's pick).
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	d := &Diff{Files: files}
	if d.Files == nil {
		d.Files = []File{} // D-63 empty state: never null
	}
	d.Totals.Files = len(d.Files)
	for _, f := range d.Files {
		if f.Additions != nil {
			d.Totals.Additions += *f.Additions
		}
		if f.Deletions != nil {
			d.Totals.Deletions += *f.Deletions
		}
	}
	return d, nil
}

// countHunkLines sums add/del lines across hunks — used for untracked files,
// which numstat never reports.
func countHunkLines(hunks []Hunk) (add, del int) {
	for _, h := range hunks {
		for _, ln := range h.Lines {
			switch ln.Kind {
			case "add":
				add++
			case "del":
				del++
			}
		}
	}
	return add, del
}

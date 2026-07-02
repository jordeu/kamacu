package diff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
		// Prefix "./" so a filename beginning with "-" can never be parsed by
		// git as an option (CR-01); rel stays the authoritative display path.
		patch, err := runNoIndex(ctx, wt, "./"+rel)
		if err != nil {
			// A single vanished/unreadable untracked file (agent churn, broken
			// symlink, FIFO) must not abort the whole diff (WR-01): skip it and
			// continue so every other file still renders.
			slog.Warn("skipping untracked file in diff", "path", rel, "err", err)
			continue
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

	// Single post-sort pass: derive the rendered-per-file hash for every file so
	// the tracked and untracked branches above share ONE hashing code path. Hash
	// keys the per-file Viewed persistence (Plan 02) and drives DIFF-04 auto-reset
	// (a changed rendered diff gets a new hash → no matching Viewed row). Viewed is
	// left at its zero value here — Compute is DB-free; the API handler fills it.
	for i := range files {
		files[i].Hash = hashFile(files[i])
	}

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

// hashFile derives a deterministic sha256 hex fingerprint of a file's RENDERED
// diff — the exact per-file content the client displays (structured-diff-on-
// server, dumb-map-on-client). It is a pure function (no I/O, ordered slices
// only, no map iteration), so the same File value hashes identically across runs
// and machines. It mirrors the shape of countHunkLines: a package-level helper
// over File/[]Hunk.
//
// INCLUDES (a change to any of these is a visible change worth re-reviewing):
//   - Status (modified/new/deleted/renamed) — a status flip is visible.
//   - the Binary flag.
//   - OldPath (the rendered "old → new" for renames).
//   - every Hunk.Header and every Line.Kind + Line.Text, in order.
//
// EXCLUDES:
//   - Base — D-01: the hash is NOT base-inclusive, so base movement that leaves a
//     file's hunks untouched (three-dot semantics) does not reset its Viewed.
//   - Path — it is the separate persistence key column (file_path); two files
//     with byte-identical rendered content share a hash but differ by path.
//   - Additions/Deletions — derived from the hunks (redundant).
//
// Serialization is LENGTH-PREFIXED (each string written as "<len>:<bytes>") so no
// field value can be confused with a delimiter — collision-safe against a value
// that happens to contain the separator (T-22-06).
//
// Binary files carry a content-invariant header and NO hunks, so their hash is
// stable regardless of the underlying bytes. A binary whose bytes change without
// a status/path/header change keeps its hash and thus its Viewed state — this is
// the accepted, documented strict-D-01 limitation (blob OIDs are intentionally
// NOT captured in parse.go); do NOT "fix" it by folding in index <oid>..<oid>.
func hashFile(f File) string {
	h := sha256.New()
	ws := func(s string) { fmt.Fprintf(h, "%d:", len(s)); io.WriteString(h, s) }
	ws(f.Status)
	if f.Binary {
		ws("bin")
	} else {
		ws("txt")
	}
	if f.OldPath != nil {
		ws(*f.OldPath)
	} else {
		ws("")
	}
	fmt.Fprintf(h, "H%d:", len(f.Hunks))
	for _, hk := range f.Hunks {
		ws(hk.Header)
		fmt.Fprintf(h, "L%d:", len(hk.Lines))
		for _, ln := range hk.Lines {
			ws(ln.Kind)
			ws(ln.Text)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

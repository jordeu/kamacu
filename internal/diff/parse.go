// Package diff computes the read-only review picture for a task's worktree
// (REVW-01, D-59): everything the task changed vs the merge-base of its base
// branch — commits on the task branch PLUS staged/unstaged changes PLUS
// untracked files rendered as full additions — pre-structured into the JSON
// shape the hand-written DiffTab renders.
//
// All git plumbing was empirically verified against git 2.43.0 in
// 05-RESEARCH.md. This package owns its OWN git runner (diff.go) because the
// untracked-file path needs `git diff --no-index`'s exit-code-1 success
// contract, which the worktree package's exit-0-only gitRun can never honor.
//
// The two parsers here are pure functions over git's machine output:
//   - parseNumstatZ: `--numstat -z` records (the single source of truth for
//     per-file additions/deletions of TRACKED files, feeding the totals bar,
//     per-file headers, and the frontend's >400-line collapse gate).
//   - parsePatch: the unified `diff` output split into per-file hunks.
//
// Invariants:
//   - NUL-tokenize numstat FIRST, then split on TAB — rename records carry an
//     empty path then two NUL-separated paths (Pitfall 2 desync).
//   - Section boundaries are line-anchored "diff --git" only; a "+"-prefixed
//     content line mentioning the phrase is never a boundary.
//   - `\ No newline at end of file` markers are dropped, never rendered.
package diff

import "strings"

// Diff is the response shape GET /api/tasks/{id}/diff returns — the EXACT
// contract the 05-04 frontend renders against (05-RESEARCH.md Pattern 4).
type Diff struct {
	Base   string `json:"base"`
	Totals Totals `json:"totals"`
	Files  []File `json:"files"`
}

// Totals is the "N files changed, +A −B" header data. Uncommitted/Unpushed
// count the files carrying the corresponding status marker (see File).
type Totals struct {
	Files       int `json:"files"`
	Additions   int `json:"additions"`
	Deletions   int `json:"deletions"`
	Uncommitted int `json:"uncommitted"`
	Unpushed    int `json:"unpushed"`
}

// File is one changed path. Additions/Deletions are nil for binary files
// (rendered stat-only per D-62); OldPath is set only for renames.
type File struct {
	Path string `json:"path"`
	// Hash is a deterministic sha256 hex of the RENDERED per-file content:
	// Status/Binary/OldPath/Hunks are IN; Base/Path/counts are OUT (see hashFile
	// in diff.go). It keys the per-file "Viewed" persistence and drives the
	// DIFF-04 auto-reset — a file whose rendered diff changes gets a new hash and
	// therefore no matching Viewed row. Base movement that leaves a file's hunks
	// untouched leaves its hash unchanged (D-01). Binary files hash a
	// content-invariant "Binary file changed" header (no hunks); the underlying
	// blob OIDs are intentionally NOT captured (strict D-01, rendered-only), so a
	// binary whose bytes change without a status/path/header change keeps its
	// hash — an accepted, documented limitation, not a bug.
	Hash string `json:"hash"`
	// Viewed reflects whether this exact rendered diff (task+path+Hash) is marked
	// reviewed. Populated by the API handler from the diff_viewed store; it is the
	// zero value (false) here because Compute is DB-free.
	Viewed bool `json:"viewed"`

	// Uncommitted marks a file whose current content is NOT fully captured in
	// commits on the task branch: it differs from HEAD (staged or unstaged) or
	// is untracked — the `git status` signal. Unpushed marks a file whose
	// COMMITTED state (HEAD) is missing from the remote: origin/<branch> exists
	// but is behind, or the branch was never pushed (every committed change is
	// local-only). Both can be true at once (commit half, keep editing). A
	// file with neither marker is committed and pushed. Like Viewed, both are
	// metadata populated by Compute and DELIBERATELY EXCLUDED from Hash:
	// committing or pushing never changes the rendered diff, so it must never
	// reset a file's Viewed state.
	Uncommitted bool `json:"uncommitted"`
	Unpushed    bool `json:"unpushed"`

	OldPath   *string `json:"oldPath"`
	Status    string  `json:"status"` // modified | new | deleted | renamed
	Binary    bool    `json:"binary"`
	Additions *int    `json:"additions"` // nil for binary
	Deletions *int    `json:"deletions"` // nil for binary
	Hunks     []Hunk  `json:"hunks"`
}

// Hunk is one `@@ ... @@` block; Header is preserved verbatim for display.
type Hunk struct {
	Header string `json:"header"`
	Lines  []Line `json:"lines"`
}

// Line is one diff line. Kind drives add/del/context coloring; Text keeps the
// leading +/-/space so the renderer can show it verbatim.
type Line struct {
	Kind string `json:"kind"` // context | add | del
	Text string `json:"text"`
}

// numstatRecord is one `--numstat -z` entry. For binary files Added/Deleted
// are zero and Binary is true (git emits "-\t-"). OldPath is set on renames.
type numstatRecord struct {
	Added   int
	Deleted int
	Binary  bool
	Path    string
	OldPath string
}

// parseNumstatZ tokenizes `--numstat -z` output. NUL is the record terminator,
// so the parser walks NUL tokens and, for each record, splits the leading
// token on TAB into <added> <deleted> <path-or-empty>. When the path field is
// EMPTY, the record is a rename: the next TWO NUL tokens are old then new
// (Pitfall 2 — consuming them here is what stops every later record desyncing).
// "-" in the added/deleted columns marks a binary file.
func parseNumstatZ(out string) []numstatRecord {
	if out == "" {
		return nil
	}
	tokens := strings.Split(out, "\x00")
	var recs []numstatRecord
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" {
			continue // trailing NUL produces an empty final token
		}
		parts := strings.SplitN(tok, "\t", 3)
		if len(parts) < 3 {
			continue // malformed; skip rather than desync
		}
		rec := numstatRecord{}
		if parts[0] == "-" || parts[1] == "-" {
			rec.Binary = true
		} else {
			rec.Added = atoiSafe(parts[0])
			rec.Deleted = atoiSafe(parts[1])
		}
		if parts[2] == "" {
			// Rename: the path field is empty; consume the next two NUL tokens
			// as old then new. Guard the indices so a truncated stream can't
			// panic.
			if i+2 < len(tokens) {
				rec.OldPath = tokens[i+1]
				rec.Path = tokens[i+2]
				i += 2
			}
		} else {
			rec.Path = parts[2]
		}
		recs = append(recs, rec)
	}
	return recs
}

// parsePatch splits a multi-file unified diff into per-file sections on
// line-anchored "diff --git " boundaries, then parses each section's headers
// and hunks. Status is derived from the file headers; binary sections and
// 100%-similarity renames carry zero hunks.
func parsePatch(out string) []File {
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var files []File
	var cur *File
	var hunk *Hunk

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			// New section boundary (line-anchored — a content "+...diff --git"
			// never reaches here because it's consumed as a hunk body line).
			flushFile()
			cur = &File{Status: "modified", Hunks: []Hunk{}}
			// Seed Path from the header so mode-only and binary sections (which
			// carry no ---/+++ lines) still have a path; ---/+++ override it.
			if p, ok := pathFromDiffGit(line); ok {
				cur.Path = p
			}
		case cur == nil:
			// Preamble before the first section (none in practice) — ignore.
			continue
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			hunk = &Hunk{Header: hunkHeader(line), Lines: []Line{}}
		case hunk != nil:
			// Inside a hunk body: classify by the leading byte.
			switch {
			case strings.HasPrefix(line, "\\"):
				// "\ No newline at end of file" — never a content line.
			case strings.HasPrefix(line, "+"):
				hunk.Lines = append(hunk.Lines, Line{Kind: "add", Text: line})
			case strings.HasPrefix(line, "-"):
				hunk.Lines = append(hunk.Lines, Line{Kind: "del", Text: line})
			case strings.HasPrefix(line, " "):
				hunk.Lines = append(hunk.Lines, Line{Kind: "context", Text: line})
			default:
				// Unknown marker (e.g. a bare "" from the trailing split, or a
				// stray header) — ignore. A genuine empty context line in a
				// unified diff is always a single space (" "), handled above.
			}
		default:
			// File-header region (between "diff --git" and the first hunk).
			applyHeader(cur, line)
		}
	}
	flushFile()
	return files
}

// applyHeader updates the current File from one pre-hunk header line.
func applyHeader(f *File, line string) {
	switch {
	case strings.HasPrefix(line, "Binary files "):
		f.Binary = true
	case strings.HasPrefix(line, "new file mode"):
		f.Status = "new"
	case strings.HasPrefix(line, "deleted file mode"):
		f.Status = "deleted"
	case strings.HasPrefix(line, "rename from "):
		f.Status = "renamed"
		old := strings.TrimPrefix(line, "rename from ")
		f.OldPath = &old
	case strings.HasPrefix(line, "rename to "):
		f.Status = "renamed"
		f.Path = strings.TrimPrefix(line, "rename to ")
	case strings.HasPrefix(line, "--- "):
		// Source path; for deletions +++ is /dev/null so this is the only path.
		if p, ok := patchPath(line, "--- "); ok && f.Path == "" {
			f.Path = p
		}
	case strings.HasPrefix(line, "+++ "):
		// Target path; the authoritative path for adds/modifies. Overrides a
		// path taken from "--- " unless this side is /dev/null (a deletion).
		if p, ok := patchPath(line, "+++ "); ok {
			f.Path = p
		}
	}
}

// pathFromDiffGit extracts a path from a `diff --git a/<p> b/<p>` header. It
// returns the b-side path (always present, identical to a-side except for
// renames where the rename headers override). Paths with spaces are rare in
// this app and git quotes them only when core.quotePath is on (we force it
// off); the heuristic takes the b/ token to end-of-line.
func pathFromDiffGit(line string) (string, bool) {
	rest := strings.TrimPrefix(line, "diff --git ")
	idx := strings.Index(rest, " b/")
	if idx < 0 {
		return "", false
	}
	return rest[idx+len(" b/"):], true
}

// patchPath extracts the path from a "--- a/<path>" or "+++ b/<path>" line,
// returning ok=false for /dev/null. The a/ or b/ prefix is stripped.
func patchPath(line, marker string) (string, bool) {
	rest := strings.TrimPrefix(line, marker)
	if rest == "/dev/null" {
		return "", false
	}
	rest = strings.TrimPrefix(rest, "a/")
	rest = strings.TrimPrefix(rest, "b/")
	return rest, true
}

// hunkHeader returns the "@@ -a,b +c,d @@" portion verbatim, dropping any
// trailing section context git appends after the second "@@".
func hunkHeader(line string) string {
	// git appends the enclosing function/context after the closing @@:
	//   "@@ -1,4 +1,5 @@ func foo()" — keep only through the second "@@".
	if idx := strings.Index(line, "@@ "); idx == 0 {
		if end := strings.Index(line[3:], "@@"); end >= 0 {
			return line[:3+end+2]
		}
	}
	return line
}

// atoiSafe parses a base-10 int, returning 0 on any error (numstat columns are
// always well-formed ints except the binary "-" sentinel handled by callers).
func atoiSafe(s string) int {
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

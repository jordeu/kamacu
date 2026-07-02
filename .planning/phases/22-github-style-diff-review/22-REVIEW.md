---
phase: 22-github-style-diff-review
reviewed: 2026-07-02T09:17:07Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - internal/api/diffs.go
  - internal/api/diffs_test.go
  - internal/diff/diff.go
  - internal/diff/diff_test.go
  - internal/diff/parse.go
  - internal/store/migrations/00011_diff_viewed.sql
  - web/src/api/diffs.ts
  - web/src/components/layout/AppLayout.tsx
  - web/src/components/task/DiffFileSection.tsx
  - web/src/components/task/DiffTab.tsx
  - web/src/components/task/FileTree.tsx
  - web/src/components/ui/checkbox.tsx
  - web/src/hooks/use-scroll-spy.ts
findings:
  critical: 1
  warning: 2
  info: 2
  total: 5
status: issues_found
---

# Phase 22: Code Review Report

**Reviewed:** 2026-07-02T09:17:07Z
**Depth:** standard
**Files Reviewed:** 13
**Status:** issues_found

## Summary

Reviewed the Phase 22 GitHub-style diff review slice: the Go diff computation
(`internal/diff`), the read-only REST endpoints + Viewed persistence
(`internal/api/diffs.go`), the keep-history migration, and the React diff UI
(DiffTab / DiffFileSection / FileTree / use-scroll-spy). The SQL is correctly
parameterized, the keep-history / auto-reset (DIFF-04) model is coherent and
well tested, and the git plumbing is careful about exit-code contracts and
three-dot semantics.

One real, easily-triggered correctness defect stands out and is a BLOCKER:
untracked file paths are passed to `git diff --no-index` as bare positional
arguments, so any legal filename beginning with `-` is parsed as an option and
aborts the **entire** diff (confirmed exit 129 against real git). Two WARNINGs
concern robustness (a single vanished/unreadable untracked file kills the whole
diff) and a scroll-spy that silently stops tracking files after a content-only
refresh. Two INFO items note a latent path-parsing flaw (currently masked) and a
500-instead-of-4xx on a bad task id.

No structural findings block was provided; the sections below are narrative
findings from direct review.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Dash-prefixed untracked filename aborts the entire diff (arg injection into `git diff --no-index`)

**File:** `internal/diff/diff.go:48-70` (definition) and `internal/diff/diff.go:156` (call site)

**Issue:** `runNoIndex` builds the command as
`git -C dir ... diff --no-index --no-color --no-ext-diff /dev/null <relpath>`
where `<relpath>` is taken verbatim from `git ls-files --others --exclude-standard -z`
(diff.go:147-156). git parses dash-prefixed positional tokens as **options**, not
paths. So an untracked file whose name legitimately begins with `-` (e.g. `-p`,
`-x`, `-config`, or a file literally named `-`) turns the invocation into an
option, git prints usage and exits **129**. `runNoIndex` only treats exit 1 as
success (diff.go:59-63); exit 129 falls through to the error return, `Compute`
returns that error (diff.go:157-159), the handler relays it as HTTP 500
(diffs.go:106-108), and the **whole Diff tab shows an error card** — the user
cannot review *any* of their changes, with no in-app workaround short of renaming
the file.

Reproduced against real git:
```
$ git diff --no-index --no-color --no-ext-diff /dev/null -p
usage: git diff --no-index [<options>] <path> <path>
...
exit=129
```
This is triggered by an ordinary, filesystem-legal filename (dash-prefixed files
are common — dotfile-adjacent config, scratch files, or an agent-created `-`).
The same vector is a latent file-write primitive: git's `--output <file>` is a
real option (present in the usage dump above), so an untracked file named
`--output=<path>` would redirect git's diff output to that path. In this
single-user localhost model the agent already has shell access so that is not a
privilege escalation, but the plain correctness break (total feature failure on a
legal filename) is the reason this is a BLOCKER.

**Fix:** Anchor the path so it can never be read as an option — prefix `./` on the
argument while keeping the raw `rel` as the authoritative `File.Path`:
```go
// diff.go — pass a path that cannot be parsed as an option.
patch, err := runNoIndex(ctx, wt, "./"+rel)
// ... f.Path = rel  // unchanged: rel stays the authoritative display path
```
```go
// runNoIndex: /dev/null is a fixed literal; only relpath is attacker-influenced.
cmd := exec.CommandContext(ctx, "git", "-C", dir,
    "-c", "core.quotePath=false",
    "diff", "--no-index", "--no-color", "--no-ext-diff", "/dev/null", relpath)
```
`"./"+rel` handles subdirectory paths (`./sub/file`) fine. (A `--` separator does
not reliably guard `--no-index` positional paths; the `./` prefix does.) Consider
the same hardening for the `h.wt.FetchRef(ctx, repo, baseName)` call in
diffs.go:93 — `baseName` flows from `pr_base_ref` and becomes
`git fetch origin <baseName>`, where a leading-dash ref would likewise be read as
an option (`resolvePRBase` itself is already safe because it prepends
`refs/heads/` / `origin/`).

## Warnings

### WR-01: One unreadable/vanished untracked file aborts the whole diff

**File:** `internal/diff/diff.go:156-159`

**Issue:** Inside the untracked loop, any error from `runNoIndex` is returned
immediately, failing all of `Compute`. Between `ls-files` (diff.go:147) and the
per-file `runNoIndex` call there is a real race window: an agent actively writing
in the worktree can delete or rewrite a file, or the entry can be an unreadable
special file (broken symlink, FIFO). `git diff --no-index /dev/null <gone>` exits
128, so a single transient file turns the entire review into an error card even
though every other file is fine. Given the app's premise (a live coding agent
churning files in the worktree), this is more than theoretical.

**Fix:** Tolerate per-file failures instead of aborting the whole computation —
skip the offending entry (optionally `slog` it) and continue:
```go
patch, err := runNoIndex(ctx, wt, "./"+rel)
if err != nil {
    slog.Warn("skipping untracked file in diff", "path", rel, "err", err)
    continue
}
```

### WR-02: Scroll-spy stops tracking files after a content-only refresh (key mismatch)

**File:** `web/src/components/task/DiffTab.tsx:49` (also `web/src/hooks/use-scroll-spy.ts:31-74`)

**Issue:** `DiffFileSection` is keyed on `` `${file.path}:${file.hash}` `` (DiffTab.tsx:207),
so editing a file's content (new hash, same path) **remounts** the section and
creates a *new* `[data-diff-path]` DOM node. But the scroll-spy is re-observed
only when its `pathListKey` changes, and that key is
`files.map((f) => f.path).join("\n")` (DiffTab.tsx:49) — paths only, no hash. On a
manual "Refresh diff" where existing files changed but the set of paths is
identical, `pathListKey` is unchanged, so the effect in `useScrollSpy`
(use-scroll-spy.ts:74 deps `[scrollRef, pathListKey]`) does **not** re-run. The
IntersectionObserver keeps its reference to the now-detached old nodes and never
calls `observe()` on the freshly-mounted ones, so the file tree stops
highlighting the currently-scrolled file for every changed file until the path
set itself changes. Refresh-after-edit is a core flow (D-61), so this degradation
is routinely reachable.

**Fix:** Key the scroll-spy on the same `path:hash` identity the sections remount
on, so the observer rebuilds whenever any node is replaced:
```ts
const activePath = useScrollSpy(
  scrollRef,
  files.map((f) => `${f.path}:${f.hash}`).join("\n"),
);
```

## Info

### IN-01: `patchPath` strips both `a/` and `b/` prefixes sequentially (latent path corruption, currently masked)

**File:** `internal/diff/parse.go:255-263`

**Issue:** `patchPath` does `TrimPrefix(rest, "a/")` then `TrimPrefix(rest, "b/")`
in sequence. For a `--- a/b/foo.txt` line (a file under a top-level dir named
`b`), this yields `foo.txt` instead of `b/foo.txt` — both prefixes get stripped.
Today this is unreachable because `pathFromDiffGit` (parse.go:244-251) already
seeds `cur.Path` from the `diff --git` header, and the `"--- "` branch only writes
when `f.Path == ""` (parse.go:227); the `"+++ "` branch overrides unconditionally
but its argument always starts with `b/`, so only one strip applies. It remains a
latent correctness trap for anyone who later removes the `pathFromDiffGit` seed or
the `f.Path == ""` guard.

**Fix:** Strip exactly the marker's own prefix rather than trying both:
```go
// caller passes the expected prefix ("a/" for "--- ", "b/" for "+++ ")
rest = strings.TrimPrefix(rest, sidePrefix)
```
or strip only the first occurrence of `a/`-or-`b/` once.

### IN-02: `setViewed` returns 500 (FK violation) for a non-existent task id

**File:** `internal/api/diffs.go:202-216`

**Issue:** `foreign_keys` is ON (store.go), and `diff_viewed.task_id` REFERENCES
`tasks(id)`. A `PUT /api/tasks/{id}/diff/viewed` with a valid-looking but
non-existent `{id}` passes all input validation and then fails the INSERT on the
FK constraint, surfacing as HTTP 500 with a raw driver message rather than a clean
404/409. Low impact (the client only toggles files on a task it just loaded), but
inconsistent with the `get` handler, which cleanly 404s an unknown task
(diffs.go:53-55).

**Fix:** Either check task existence first (mirroring `get`) or map the FK
constraint error to a 404:
```go
// minimal: translate the constraint error
if err != nil {
    if strings.Contains(err.Error(), "FOREIGN KEY") {
        writeError(w, http.StatusNotFound, "task not found")
        return
    }
    writeError(w, http.StatusInternalServerError, err.Error())
    return
}
```

---

_Reviewed: 2026-07-02T09:17:07Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_

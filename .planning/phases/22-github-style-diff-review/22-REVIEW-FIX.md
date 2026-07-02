---
phase: 22-github-style-diff-review
fixed_at: 2026-07-02T09:28:28Z
review_path: .planning/phases/22-github-style-diff-review/22-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
status: all_fixed
---

# Phase 22: Code Review Fix Report

**Fixed at:** 2026-07-02T09:28:28Z
**Source review:** .planning/phases/22-github-style-diff-review/22-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 3 (CR-01, WR-01, WR-02)
- Fixed: 3
- Skipped: 0

Info findings (IN-01, IN-02) were out of scope for this pass and left untouched.

## Fixed Issues

### CR-01: Dash-prefixed untracked filename aborts the entire diff

**Files modified:** `internal/diff/diff.go`
**Commit:** 2c14263
**Applied fix:** Changed the untracked-file call site to pass `"./" + rel` to
`runNoIndex` instead of the bare `rel`. The `./` prefix anchors the argument so a
filename beginning with `-` (e.g. `-p`, `--output=...`) can never be parsed by
git as an option (previously exit 129, aborting the whole diff). The raw `rel`
remains the authoritative display path (`f.Path = rel` unchanged), so subdirectory
paths render as `./sub/file` to git while the UI still shows the clean path.
Verified with `go build ./...` and `go test ./internal/diff/...` (pass) — no test
asserts the argument shape, and the display-path assertions still hold.

### WR-01: One unreadable/vanished untracked file aborts the whole diff

**Files modified:** `internal/diff/diff.go`
**Commit:** 7ca2b89
**Applied fix:** In the untracked-file loop, a per-file `runNoIndex` error now logs
`slog.Warn("skipping untracked file in diff", ...)` and `continue`s past the
offending entry instead of returning and failing all of `Compute`. Added the
`log/slog` import. This tolerates the `ls-files` → `runNoIndex` race window (agent
churn, broken symlinks, FIFOs) so every other file still renders. Coordinated with
the CR-01 edit at the same call site. Verified with `go build ./...` and
`go test ./internal/diff/... ./internal/api/...` (pass).

### WR-02: Scroll-spy stops tracking files after a content-only refresh

**Files modified:** `web/src/components/task/DiffTab.tsx`
**Commit:** 72579f1
**Applied fix:** Changed the `useScrollSpy` key from `files.map((f) => f.path)` to
`files.map((f) => `${f.path}:${f.hash}`)` so it matches the `path:hash` identity
the `DiffFileSection` components remount on. The IntersectionObserver now re-runs
its effect whenever any file's hash changes (content-only "Refresh diff" where the
path set is unchanged), re-`observe()`ing the freshly-mounted nodes instead of
holding detached ones. Updated the adjacent comment to describe the new key.
Verified: `cd web && npm run build` (tsc -b + vite) exits 0 — tsc confirms `f.hash`
is a valid field. `npm run lint` on DiffTab.tsx shows only the pre-existing
`setState within effect` warning (present at the same `setShowLoading(false)` call
before the edit, merely shifted line number) — 0 new lint errors introduced.

## Verification

- `go build ./...` — OK
- `go test ./internal/diff/... ./internal/api/...` — both `ok` (diff 0.7s, api 83.3s)
- `cd web && npm run build` — exit 0 (tsc -b + vite build succeeded)
- `cd web && npm run lint` on `DiffTab.tsx` — only the pre-existing cascading-render
  warning remains; 0 new errors from the WR-02 edit.

---

_Fixed: 2026-07-02T09:28:28Z_
_Iteration: 1_
</content>
</invoke>

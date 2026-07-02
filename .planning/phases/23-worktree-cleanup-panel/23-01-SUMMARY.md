---
phase: 23-worktree-cleanup-panel
plan: 01
subsystem: api
tags: [git-worktree, porcelain-parser, cleanup, fs.ErrPermission, go, tdd]

# Dependency graph
requires:
  - phase: 13-pr-worktree-auto-cleanup
    provides: CleanupWorktreeGated shared removal core (handler + reaper callers)
  - phase: 03-worktree-per-task
    provides: worktree.Service gitRun arg-array idiom + DirtyCount/UnpushedCount/StashCount/Remove
provides:
  - "worktree.List(ctx, repo) + Entry: git worktree list --porcelain -z parser (typed records incl. locked/prunable/detached/bare)"
  - "parseWorktreeList: unit-testable pure parse split out from List"
  - "BlockedError{Path}: D-01 permission-blocked signal carrying the offending path"
  - "classifyRemoveBlocked: fs.ErrPermission/*fs.PathError + git-stderr-string EACCES detection"
  - "CleanupWorktreeGated orphan mode (taskID==0 skips null-columns UPDATE) + blocked outcome (reason=='blocked', no --force retry)"
affects: [23-02, 23-03, 23-04, 23-05, worktree-cleanup-panel]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Faithful porcelain -z parser: records \\x00\\x00-split, attributes \\x00-split, trailing-NUL trimmed; main worktree NOT filtered (API layer excludes it)"
    - "Typed BlockedError as a distinct removal outcome (reason string 'blocked') instead of a raw error, mapping to a 200 {outcome:blocked} at the handler"
    - "Additive extensions to a shared byte-equivalent core, guarded so existing callers stay byte-identical (D-04 regression guard)"

key-files:
  created:
    - internal/api/cleanup_test.go
  modified:
    - internal/worktree/worktree.go
    - internal/worktree/worktree_test.go
    - internal/api/cleanup.go

key-decisions:
  - "classifyRemoveBlocked factored as a pure helper so both the *fs.PathError branch and the git-stderr-string fallback are unit-testable independently of live git"
  - "The git-shell-out path surfaces a STRING error (not a wrapped *fs.PathError), so the stderr fallback is the branch that fires in practice; the *fs.PathError branch is kept defensively (RESEARCH Option B, verified)"
  - "List does NOT filter the main worktree — kept a faithful parser; exclusion is the API layer's job (filepath.Clean match against project.repo_path)"

patterns-established:
  - "Pattern 1: git worktree list --porcelain -z parser producing typed Entry records (WTREE-01/04 enumeration foundation)"
  - "Pattern 4: D-01 permission-blocked outcome as a path-carrying BlockedError that never triggers a --force retry"

requirements-completed: [WTREE-01, WTREE-04, WTREE-02]

# Metrics
duration: 12min
completed: 2026-07-02
---

# Phase 23 Plan 01: Worktree Enumeration + Cleanup-Core Extensions Summary

**A `git worktree list --porcelain -z` parser (typed Entry records) plus two additive extensions to the shared removal core — orphan mode (taskID==0 skips the null-columns UPDATE) and a D-01 permission-blocked outcome (BlockedError carrying the offending path, never retrying --force).**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-02T15:58:00Z (approx)
- **Completed:** 2026-07-02T16:06:20Z
- **Tasks:** 2 (both TDD)
- **Files modified:** 4 (2 source, 2 test — 1 test file created)

## Accomplishments
- `worktree.List(ctx, repo)` + exported `Entry` struct: a faithful `--porcelain -z` parser (records `\x00\x00`-split, attributes `\x00`-split, trailing-NUL trimmed) covering `worktree`/`HEAD`/`branch`/`bare`/`detached`/`locked`(+reason)/`prunable`(+reason). Verified against real git 2.43.0 (main + linked, deleted-dir reverse-orphan) and crafted `-z` buffers.
- `parseWorktreeList` split out from `List` so the crafted-buffer edge cases (locked reasons, bare, trailing-NUL hygiene) are unit-testable without live git.
- `CleanupWorktreeGated` orphan mode: `taskID==0` removes an orphan worktree without running the null-columns `UPDATE` (guarded behind `taskID > 0`).
- `CleanupWorktreeGated` D-01 blocked outcome: a permission-blocked `wt.Remove` returns `(false, "blocked", *BlockedError)` with the offending path, and NEVER retries `--force` (Pitfall 1). Detection covers both the Go-native `*fs.PathError`/`fs.ErrPermission` path and git's stderr string (`Permission denied` / `Directory not empty`+`failed to delete`).
- The two existing callers (handler + reaper, `taskID>0`, clean path) stay byte-identical — the D-04 regression tests (`internal/api`, `internal/reaper`) remain green.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1 (RED): failing List parser tests** - `fb4c101` (test)
2. **Task 1 (GREEN): List + Entry + parseWorktreeList** - `a4ec13e` (feat)
3. **Task 2 (RED): failing CleanupWorktreeGated extension tests** - `9ca8397` (test)
4. **Task 2 (GREEN): orphan mode + BlockedError outcome** - `b92ec87` (feat)

No REFACTOR commits — the GREEN implementations were already clean (helpers split for testability up front).

## Files Created/Modified
- `internal/worktree/worktree.go` - Added `Entry` struct, `List`, and `parseWorktreeList` (porcelain `-z` parser). `Remove` untouched — its only `--force` retry still gates on `"submodules"`.
- `internal/worktree/worktree_test.go` - Added `TestListParsesRealGit` (live git integration) + `TestListParseBuffer` (crafted `-z` buffers).
- `internal/api/cleanup.go` - Added `BlockedError`, `classifyRemoveBlocked`, `parseCouldNotOpenDir`; wired Extension A (blocked detection at the `wt.Remove` failure) and Extension B (`taskID > 0` guard on the UPDATE) into `CleanupWorktreeGated`.
- `internal/api/cleanup_test.go` - New: orphan-skips-update, referenced-runs-update (D-04 parity), blocked-on-stderr (real git mode-000 subdir), unrelated-error-unchanged, and direct `classifyRemoveBlocked` cases (PathError + stderr fallback).

## Decisions Made
- **`classifyRemoveBlocked` as a pure helper** (not inlined): lets both detection branches be unit-tested independently, and made the "git returns a string, not a `*fs.PathError`" reality explicit and testable.
- **`List` stays a faithful parser** (does not exclude the main worktree): matches RESEARCH Pattern 1 and the plan's directive; the main-worktree exclusion is deferred to the API layer (a later 23-0x plan).
- **Offending-path resolution order**: `*fs.PathError.Path` (exact) → parse git's `could not open directory '<dir>'` → fall back to the worktree path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Relaxed an over-specified live-git test assertion**
- **Found during:** Task 1 (GREEN, `TestListParsesRealGit`)
- **Issue:** The RESEARCH-quoted sample showed a manually-deleted-dir worktree as `detached` + dropped `branch` + `prunable`. On this machine's git (2.43.0) a merely-deleted-dir worktree is reported as `prunable` (with the correct `gitdir file points to non-existent location` reason) but KEEPS its `branch` line and does NOT emit `detached`. The parser was correct (faithful to git's actual bytes); the *test assertion* over-specified git's version-dependent behavior.
- **Fix:** Relaxed the live-git integration assertion to the stable, load-bearing signal the panel keys on — `Prunable == true` + its reason mentioning the non-existent location. The crafted `TestListParseBuffer` case still exercises the full `detached`+dropped-`branch`+`prunable` porcelain shape (a valid record git emits in other scenarios), so parser coverage of that shape is retained.
- **Files modified:** internal/worktree/worktree_test.go
- **Verification:** `go test ./internal/worktree/` green.
- **Committed in:** a4ec13e (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug — a test-assertion correction, not a code fix).
**Impact on plan:** No scope creep. The implementation matches the plan verbatim; only a version-dependent test expectation was corrected to match real git output while preserving full parser coverage.

## Issues Encountered
- **git-shell-out errors are strings, not `*fs.PathError`.** Empirically confirmed that `git worktree remove` on a mode-000 subdir fails with exit 255 and a stderr string (`Permission denied` / `Directory not empty`), which `gitRun` returns as a plain `error`. This means the plan-mandated `*fs.PathError`/`errors.Is(fs.ErrPermission)` branch does not fire on the real git path — the stderr-string fallback does. Both branches were implemented as the plan required; the tests exercise the `*fs.PathError` branch directly (via `os.RemoveAll`, RESEARCH Option B) and the stderr branch via real git. No change to the plan's intent.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The two load-bearing backend mechanisms this phase needs are complete and below the HTTP layer, giving Wave 2 (23-02+) a full contract: `worktree.List`/`Entry` for enumeration/classification, and `CleanupWorktreeGated`'s orphan + blocked outcomes for per-item force-remove and bulk clean.
- No blockers. Threat register honored: `List` uses the arg-array `gitRun` (T-23-01, no shell); the blocked path stops at the first EACCES and never proceeds to `--force` (T-23-02, no misleading deregistered shell).

## Threat Flags
None — this plan is entirely below the HTTP layer (no new endpoints, auth paths, or schema). No security surface beyond the threat register's already-mitigated items.

## Self-Check: PASSED

---
*Phase: 23-worktree-cleanup-panel*
*Completed: 2026-07-02*

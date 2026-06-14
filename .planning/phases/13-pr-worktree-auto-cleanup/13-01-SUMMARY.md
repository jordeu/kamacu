---
phase: 13-pr-worktree-auto-cleanup
plan: 01
subsystem: api
tags: [github, gh, git, worktree, reaper, pr-state, cleanup, go, react]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "github.PRDetail/ViewPR, prWire/prWireFrom detail wire, PRDetailWire (TS), worktreeHandlers.remove gated cleanup, worktree.Service.DirtyCount/Remove/FetchRef"
provides:
  - "github.PRDetail.State + ViewPR requesting `state` (single source for the D-09 banner and the reaper)"
  - "github.Service.PRState(ctx, repo, n) — cheap per-tick UPPERCASE state reader satisfying the reaper's PRStateGetter (13-02)"
  - "cleanupWorktreeGated in internal/api — byte-equivalent gate+stop+kill+remove+null-columns helper; the HTTP handler now delegates"
  - "worktree.Service.UnpushedCount (rev-list base..HEAD) + StashCount (repo-global stash list) — the reaper's conservative gate primitives (13-02)"
  - "state plumbed end-to-end onto the detail wire: prWire.State + prWireFrom + PRDetailWire.state (OPEN|CLOSED|MERGED)"
affects: [13-02-reaper, 13-03-frontend-banner-menu]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Shared gated-cleanup helper: one core path (cleanupWorktreeGated) consumed by the HTTP handler now and the reaper next; never writes HTTP — returns (removed, reason, err)"
    - "Single-source PR state: one `state` field on PRDetail feeds BOTH the detail-endpoint banner and (via a thin PRState reader) the reaper, with no `merged` boolean ever requested from gh (it errors)"
    - "Reaper-only conservative gates kept OUT of the helper so the extraction stays verifiable against unchanged worktree DELETE tests"

key-files:
  created:
    - internal/github/state.go
    - internal/github/state_test.go
    - internal/api/cleanup.go
    - internal/worktree/cleanup_gates_test.go
  modified:
    - internal/github/github.go
    - internal/github/github_test.go
    - internal/api/worktrees.go
    - internal/worktree/worktree.go
    - internal/api/pullrequests.go
    - internal/api/pullrequests_test.go
    - web/src/api/pullRequests.ts

key-decisions:
  - "PRState is a dedicated minimal `gh pr view --json state` method (one fewer field than ViewPR) on *Service, not a ViewPR wrapper — smallest per-tick reaper call (RESEARCH Open Q3 lean)"
  - "cleanupWorktreeGated is a BYTE-EQUIVALENT extraction of today's dirty+sessions logic ONLY; the two new gates (rev-list unpushed, stash) live in 13-02's reaper, not the helper (RESEARCH Open Q1 lowest-regression default)"
  - "Helper placed in internal/api (reaper -> api is acyclic, smaller change than a new internal/cleanup package)"
  - "The per-kill tmux slog.Warn line is dropped (swallowed with _=) per plan: KillSession was always best-effort/never-blocking, and the detached-tmux test asserts the KILL, not a log line — observable behavior identical"

patterns-established:
  - "Pattern: shared cleanup core as a non-HTTP helper returning (removed bool, reason string, err error); callers map the reason to their own response (409 here, skip-and-log in the reaper)"
  - "Pattern: degrade-don't-break leaf readers (PRState) return (\"\", err) on gh failure so background callers warn-and-skip rather than crash"

requirements-completed: [GHCLN-01, GHCLN-02, GHCLN-03]

# Metrics
duration: 9min
completed: 2026-06-14
---

# Phase 13 Plan 01: Backend Foundations for PR Worktree Auto-Cleanup Summary

**PR `state` plumbed end-to-end (PRDetail/ViewPR -> prWire -> PRDetailWire) plus a cheap `github.Service.PRState` reader, a byte-equivalent `cleanupWorktreeGated` extraction the DELETE handler now delegates to, and `worktree.Service.UnpushedCount`/`StashCount` conservative-gate primitives — the three seams 13-02 (reaper) and 13-03 (banner/menu) consume.**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-14T10:20:13Z
- **Completed:** 2026-06-14T10:28:51Z
- **Tasks:** 3
- **Files modified:** 11 (4 created, 7 modified)

## Accomplishments

- **PR state as a single source:** `github.PRDetail` gained a `State` field (plain `json:"state"`, decoded directly), `ViewPR` requests `state` in its `--json` list, and `github.Service.PRState` is a dedicated minimal reader returning the UPPERCASE `OPEN|CLOSED|MERGED` for the reaper's per-tick use — degrading to `("", err)` when `gh` is absent/fails (D-03).
- **Byte-equivalent cleanup extraction:** the inline gate+stop+kill+remove+null-columns sequence in `worktreeHandlers.remove` was lifted into `cleanupWorktreeGated` (`internal/api/cleanup.go`); the DELETE handler now delegates and maps the helper's `(removed, reason)` back to the SAME 409 / 500 / 204 responses. All four existing worktree DELETE tests pass UNCHANGED (the D-04 regression guard).
- **Conservative-gate primitives:** `worktree.Service.UnpushedCount(ctx, wt, base)` (rev-list `base..HEAD`, catches the committed-but-unpushed fixup that is invisible to porcelain) and `StashCount(ctx, wt)` (repo-global stash list) — both with a hermetic real-git unit test — ready for 13-02's reaper to call.
- **State on the detail wire:** `prWire` gained `State`, `prWireFrom` sets it from `d.State` (keeping the GET `.../{n}` detail and POST `/review` envelope byte-identical), and `PRDetailWire` (TS) declares the `OPEN|CLOSED|MERGED` union for 13-03's banner.

## Task Commits

Each task was committed atomically (TDD: tests + impl per task):

1. **Task 1: PR state on PRDetail/ViewPR + github.Service.PRState** - `3e89a24` (feat)
2. **Task 2: cleanupWorktreeGated extraction + UnpushedCount/StashCount** - `21de560` (refactor)
3. **Task 3: state onto the detail wire (prWire/prWireFrom/PRDetailWire)** - `8be849e` (feat)

## Files Created/Modified

- `internal/github/state.go` - `github.Service.PRState` cheap per-tick state reader (gh pr view --json state), degrade-don't-break
- `internal/github/state_test.go` - PRState fake-gh (OPEN/CLOSED/MERGED) + no-gh degrade tests
- `internal/github/github.go` - `PRDetail.State` field; `ViewPR` `--json` list gains `state`
- `internal/github/github_test.go` - fixture `state` field + State decode assertion
- `internal/api/cleanup.go` - `cleanupWorktreeGated` shared helper (the extraction)
- `internal/api/worktrees.go` - `remove()` delegates to the helper; dropped the now-unused `time` import
- `internal/worktree/worktree.go` - `UnpushedCount` + `StashCount` gate primitives
- `internal/worktree/cleanup_gates_test.go` - hermetic real-git unit test for both primitives
- `internal/api/pullrequests.go` - `prWire.State` + `prWireFrom` sets `State: d.State`
- `internal/api/pullrequests_test.go` - `state` on the detail-response struct + MERGED assertion
- `web/src/api/pullRequests.ts` - `PRDetailWire.state` union

## Decisions Made

- **PRState is a dedicated minimal method** (not a `ViewPR` wrapper): one fewer JSON field per reaper call, smallest possible per-PR read (RESEARCH Open Q3).
- **The two new gates (rev-list/stash) stay OUT of the helper** and will live in 13-02's `reconcilePRsOnce`: this keeps `cleanupWorktreeGated` a pure mechanical extraction provable against the unchanged worktree DELETE tests (RESEARCH Open Q1, the lowest-regression reading of "regression-sensitive").
- **Helper lives in `internal/api`** (reaper -> api is acyclic): the smaller change vs a new `internal/cleanup` package.
- **Per-kill tmux warn line dropped** (swallowed with `_ =`): the plan sanctioned this since KillSession failures were always best-effort/never-blocking and `TestWorktreeCleanupCountsAndKillsDetachedTmux` asserts the kill (via `awaitHasSession(...false)`), not a log line. Verified: that test passes unchanged.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- After delegating `remove()` to the helper, `internal/api/worktrees.go` no longer used the `time` import (it moved into `cleanup.go`). The plan anticipated this ("let `go build` flag any now-unused imports"); removed `time` only — `context` (still used by `liveTmuxNames`/`cleanupSessionCount` signatures) and `os` (still used by `get()`) stayed. Build clean afterward.

## Known Stubs

None - no placeholder/empty-data patterns introduced. The reaper that calls `PRState`/`UnpushedCount`/`StashCount` and the banner that reads `PRDetailWire.state` are built in 13-02 and 13-03 respectively (by design — these are the seams those plans consume).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- **13-02 (reaper):** `github.Service.PRState` (satisfies a local `PRStateGetter`), `cleanupWorktreeGated` (call with `force=false, stopSessions=false`), and `worktree.Service.UnpushedCount`/`StashCount` are all in place. The reaper still needs: the `reconcilePRsOnce` pass, wiring `wt`/`tmuxClient`/`github.Service` into `reaper.New` in `main.go`, and the post-remove `DELETE FROM tmux_sessions` -> `DELETE FROM tasks` (FK ordering, Pitfall 3).
- **13-03 (frontend):** `PRDetailWire.state` is declared; the banner can branch on `state !== "OPEN"`. The PR-specific `⋯` menu (D-08) and banner copy/placement remain.

## Self-Check: PASSED

---
*Phase: 13-pr-worktree-auto-cleanup*
*Completed: 2026-06-14*

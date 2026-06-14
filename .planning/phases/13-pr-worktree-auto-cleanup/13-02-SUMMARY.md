---
phase: 13-pr-worktree-auto-cleanup
plan: 02
subsystem: reaper
tags: [reaper, github, pr-state, worktree, cleanup, gate, fk-delete, go]

# Dependency graph
requires:
  - phase: 13-pr-worktree-auto-cleanup
    plan: 01
    provides: "github.Service.PRState; cleanupWorktreeGated (now exported CleanupWorktreeGated); worktree.Service.UnpushedCount/StashCount/DirtyCount/FetchRef"
provides:
  - "reaper.PRStateGetter — local interface (mirrors SessionStopper) the real *github.Service satisfies via PRState"
  - "reaper.reconcilePRsOnce — the server-side merge/close detector: per-tick second pass over source='github_pr' worktree tasks, conservative gate, auto-remove + FK-ordered row delete on MERGED/CLOSED"
  - "reaper.NewWithPR — full constructor wiring db+mgr+wt+tmuxClient+PRStateGetter; New(db, mgr) stays Done-TTL-only (pr==nil disables the PR pass)"
  - "api.CleanupWorktreeGated — the 13-01 helper EXPORTED so the reaper (different package) calls the one shared cleanup path (D-04, one path two callers)"
affects: [13-03-frontend-banner-menu]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Reaper two-pass tick: reapOnce (Done-TTL) + reconcilePRsOnce (PR merge/close), both run at immediate-start and on the ticker; pr==nil cleanly disables the PR pass for Done-TTL-only construction (keeps the existing reaper_test.go New(db, spy) calls untouched)"
    - "Conservative auto-removal gate computed from FRESH git state in the reaper (dirty + unpushed-vs-re-fetched-PR-head + repo-global stash), with the session gate enforced INSIDE the shared helper via force=false/stopSessions=false — never forces, never stops a live session"
    - "Auto-cleanup DELETES the task row (D-07) in FK order (tmux_sessions before tasks) ONLY on a successful helper remove; the gated-skip and OPEN/error cases mutate nothing"
    - "Spy + real-git-fixture test seam: inject a spyPRState for the gh read, but run the four gates against actual throwaway repos/worktrees (bare origin carrying refs/pull/<n>/head) so the gate logic is verified for real, not mocked"

key-files:
  created:
    - internal/reaper/pr_reconcile_test.go
  modified:
    - internal/reaper/reaper.go
    - internal/api/cleanup.go
    - internal/api/worktrees.go
    - cmd/kangent/main.go

key-decisions:
  - "NewWithPR is a SECOND constructor (not a widened New): keeps the Done-TTL reaper_test.go's New(db, spy) calls compiling unchanged; pr==nil is the switch that makes Run skip reconcilePRsOnce so the existing tests' Run path is unaffected"
  - "cleanupWorktreeGated RENAMED to exported CleanupWorktreeGated (the one cross-package edit to 13-01's helper) — mechanical rename + single call-site update in worktrees.go; the worktree DELETE tests pass unchanged (they exercise the HTTP handler, not the symbol name)"
  - "The unpushed gate's fetch runs IN THE WORKTREE (t.wtPath), not the common repo dir — FETCH_HEAD is a per-worktree ref (Rule 1 bug fix, see Deviations)"
  - "Reaper replicates worktreeHandlers' runningSessions/liveTmuxNames/cleanupSessionCount fold (they are unexported methods on a different type) with nil-sessionMgr / zero-tmuxClient guards so the Done-TTL-only construction is safe"

patterns-established:
  - "Pattern: a background reconcile pass mirrors the existing reapOnce structure verbatim (query -> per-row degrade-don't-break loop -> slog.Warn+continue on any error) so a gh/git hiccup never crashes the tick"
  - "Pattern: gate from fresh state, fetch-then-rev-list in the worktree, conservative-skip on any inability to verify (a fetch failure skips rather than removes)"

requirements-completed: [GHCLN-01, GHCLN-02]

# Metrics
duration: 9min
completed: 2026-06-14
---

# Phase 13 Plan 02: Reaper PR Reconcile Pass Summary

**The server-side merge/close detector that closes the v1.3 loop: a second reaper pass (`reconcilePRsOnce`) reads each `source='github_pr'` worktree task's PR state via a local `PRStateGetter` (the real `*github.Service`), and on MERGED/CLOSED auto-removes a pristine, idle worktree through the shared `CleanupWorktreeGated` helper (force=false, stopSessions=false) then FK-deletes the row (D-07) — conservatively gated on dirty / unpushed (rev-list vs a re-fetched PR head) / stash / running session, with `source='manual'` never touched (D-87).**

## Performance

- **Duration:** ~9 min
- **Started:** 2026-06-14T10:32:35Z
- **Completed:** 2026-06-14
- **Tasks:** 3
- **Files modified:** 5 (1 created, 4 modified)

## Accomplishments

- **`reconcilePRsOnce` second pass:** queries `tasks t JOIN projects p WHERE t.source='github_pr' AND t.worktree_path IS NOT NULL AND p.github_repo IS NOT NULL`, reads each PR's state, and only MERGED/CLOSED proceed (OPEN / unknown / any gh error -> warn-and-skip, D-03). It mirrors `reapOnce`'s defensive structure exactly (never returns an error, never panics on a bad row).
- **Conservative gate (GHCLN-02), four checks from fresh git state:** dirty (`DirtyCount`), unpushed (`FetchRef refs/pull/<n>/head` then `UnpushedCount FETCH_HEAD..HEAD` — catches the committed-but-unpushed fixup invisible to porcelain), stash (repo-global `StashCount`), and sessions (folded count enforced inside the shared helper). ANY trip -> `slog.Info` reason + skip; nothing mutated.
- **Auto-remove + FK-ordered delete (GHCLN-01, D-07):** on a clean pass the reaper calls `api.CleanupWorktreeGated(..., false /*stopSessions*/, false /*force*/)`; on `removed==true` it deletes `tmux_sessions` rows BEFORE the `tasks` row (FKs ON, no CASCADE — Pitfall 3) so the auto-delete never hits a foreign-key violation.
- **`PRStateGetter` + `NewWithPR`:** the local interface (mirroring `SessionStopper`) the real `*github.Service` satisfies; `New(db, mgr)` stays Done-TTL-only (`pr==nil` disables the PR pass) so the existing reaper tests compile and behave unchanged.
- **Helper exported:** `cleanupWorktreeGated` -> `CleanupWorktreeGated` (the one cross-package edit), one shared cleanup path for both the HTTP handler and the reaper (D-04).
- **main.go wiring:** `reaper.New(db, mgr)` -> `reaper.NewWithPR(db, mgr, wtSvc, tmuxClient, ghSvc)` — the PR pass runs against the SAME `*github.Service` the PR routes use (no new construction).
- **Spy-driven matrix over real git:** 7 reconcile tests (merged+clean / closed+clean -> removed+deleted; merged+dirty / +unpushed / +stash -> skipped; open -> untouched; manual -> zero PRState calls), each running the four gates against actual throwaway repos/worktrees.

## Task Commits

1. **Task 1: PRStateGetter + reconcilePRsOnce + reaper deps + export helper** - `a37f50b` (feat)
2. **Task 2: spy-driven reconcile matrix + FETCH_HEAD-in-worktree fix** - `6d51c47` (test)
3. **Task 3: wire github service + worktree/tmux into reaper in main.go** - `15053a9` (feat)

## Files Created/Modified

- `internal/reaper/reaper.go` - `PRStateGetter` interface; `Reaper` struct gains `sessionMgr`/`wt`/`tmuxClient`/`pr`; `NewWithPR` constructor; `reconcilePRsOnce` pass; `runningSessions`/`liveTmuxNames`/`extraLiveTmux` helpers (replicating worktreeHandlers, nil-guarded); `Run` calls both passes
- `internal/reaper/pr_reconcile_test.go` - `spyPRState` + real-git `prFixture` (bare origin with `refs/pull/<n>/head`); the 7-test reconcile matrix
- `internal/api/cleanup.go` - `cleanupWorktreeGated` renamed to exported `CleanupWorktreeGated` (+ doc note on the cross-package call)
- `internal/api/worktrees.go` - the single call site updated to `CleanupWorktreeGated`
- `cmd/kangent/main.go` - `reaper.NewWithPR(db, mgr, wtSvc, tmuxClient, ghSvc)` + updated construction comment

## Decisions Made

- **`NewWithPR` as a second constructor** (not a widened `New`): preserves `reaper_test.go`'s `New(db, spy)` calls and the existing tests' `Run` behavior verbatim; `pr==nil` is the switch that skips the PR pass.
- **Export `CleanupWorktreeGated`:** the reaper is a different package, so the one shared cleanup path must be exported; mechanical rename + one call-site edit, the worktree DELETE tests unchanged.
- **Fetch the PR head IN THE WORKTREE** (`t.wtPath`), not the common repo dir — see Deviations (Rule 1).
- **Replicate the session-count + tmux probe in the reaper** rather than exporting `worktreeHandlers`' unexported methods: they are methods on a type the reaper has no instance of; the replicas are nil-`sessionMgr` / zero-`tmuxClient` guarded so the Done-TTL-only construction is safe.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Unpushed-gate fetch must run in the worktree, not the repo dir**
- **Found during:** Task 2 (the clean-remove tests failed with `ambiguous argument 'FETCH_HEAD..HEAD': unknown revision`).
- **Issue:** The plan's action wrote `r.wt.FetchRef(ctx, t.repoDir, refs/pull/<n>/head)` then `r.wt.UnpushedCount(ctx, t.wtPath, "FETCH_HEAD")`. `FETCH_HEAD` is a PER-WORKTREE ref — a fetch in the common repo dir writes `FETCH_HEAD` into the main worktree's git dir, where it is NOT visible from the linked PR worktree. The subsequent `rev-list FETCH_HEAD..HEAD` in `t.wtPath` errored, so the unpushed gate would conservatively skip EVERY merged/closed PR forever — auto-cleanup would never fire in production (a silent no-op, masked by degrade-don't-break).
- **Fix:** Fetch in the worktree (`r.wt.FetchRef(ctx, t.wtPath, ...)`); the worktree shares `origin`'s config via the common git dir, so the fetch resolves and `FETCH_HEAD` is then visible to the rev-list. Verified standalone (fetch-in-repo -> rev-list-in-wt fails; fetch-in-wt -> rev-list-in-wt returns 0) and via the now-passing `TestReconcileMergedCleanRemovesAndDeletes` / `TestReconcileMergedUnpushedSkips`.
- **Files modified:** internal/reaper/reaper.go (with an explanatory comment on the per-worktree FETCH_HEAD subtlety)
- **Commit:** 6d51c47

## Issues Encountered

- None beyond the Rule 1 fix above. The acyclic `reaper -> api` import (verified: `api` does not import `reaper`) held, so the exported-helper approach compiled cleanly.

## Known Stubs

None - the reconcile pass is fully wired end-to-end (real `*github.Service` in `main.go`, real `worktree.Service`/`tmux.Client`). 13-03 (frontend banner + `⋯` menu) consumes the already-shipped `PRDetailWire.state` (13-01) and the manual cleanup affordance; nothing here is a placeholder.

## User Setup Required

None - no external configuration. The PR pass degrades to a no-op per tick if `gh` is absent/unauthenticated (PRState errors -> warn-and-skip), exactly the degrade-don't-break posture; the Done-TTL pass and all manual flows are unaffected.

## Next Phase Readiness

- **13-03 (frontend):** the server-side loop is closed — a merged/closed PR with a pristine, idle worktree self-cleans and its row is deleted, so the review view's "Task not found" degrade (D-09 clean case) is now reachable. The banner (`state !== "OPEN"`) and the PR-specific `⋯` "Clean up worktree" item (D-08, GHCLN-03) remain; both ride existing seams (`PRDetailWire.state` from 13-01, the already-rendered `CleanupWorktreeDialog`).

## Self-Check: PASSED

---
*Phase: 13-pr-worktree-auto-cleanup*
*Completed: 2026-06-14*

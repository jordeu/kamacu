---
phase: 13-pr-worktree-auto-cleanup
plan: 03
subsystem: web
tags: [github, pr-review, worktree, cleanup, banner, menu, react, human-verify]

# Dependency graph
requires:
  - phase: 13-pr-worktree-auto-cleanup
    plan: 01
    provides: "PRDetailWire.state (OPEN|CLOSED|MERGED) on the live detail wire; usePullRequestDetail re-hydrating it on mount"
  - phase: 13-pr-worktree-auto-cleanup
    plan: 02
    provides: "reconcilePRsOnce auto-cleanup + conservative gate (the reaper behavior the human-verify exercises)"
  - phase: 12-open-a-review
    provides: "TaskPage PR review shell; CleanupWorktreeDialog (gated, any taskId); the shrink-0 header block + 12-07 fix-#6 height chain"
provides:
  - "PR review ⋯ menu with a SINGLE 'Clean up worktree' item (no Delete task, D-08) opening the existing gated CleanupWorktreeDialog"
  - "Inline non-blocking merged/closed banner (D-09) shown when prDetail.state !== 'OPEN', inside the shrink-0 header block with a Clean-up link while the worktree exists"
  - "Phase 13 human-verified end-to-end: GHCLN-01 (auto-cleanup clean), GHCLN-02 (safety skip + branch kept), GHCLN-03 (manual cleanup + banner)"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pure reuse: the ⋯ menu and banner both call the already-rendered CleanupWorktreeDialog via setCleanupOpen(true); no new dialog, no new mutation, no new state — cleanupOpen and usePullRequestDetail already existed"
    - "Banner/menu placed INSIDE the w-full shrink-0 header block (sibling of the merge line, wrapped in a fragment) so neither joins the tabs/terminal flex chain (12-07 fix #6 — no terminal-height regression)"
    - "Separate PR-only menu gate (isPR && task.worktree_path) rather than relaxing the existing {!isPR && (...)} gate: the manual-task menu (with Delete task) is left byte-for-byte unchanged"

key-files:
  created:
    - .planning/phases/13-pr-worktree-auto-cleanup/13-03-SUMMARY.md
  modified:
    - web/src/pages/TaskPage.tsx
    - web/dist/index.html
    - .gitignore

key-decisions:
  - "PR ⋯ menu gated on isPR && task.worktree_path (mirrors the non-PR menu's Clean-up gate): with no worktree there is nothing to clean up — a clean auto-removed PR has its row deleted by the reaper (never reaches here), a skipped one still has worktree_path set"
  - "Banner is an inline role=status advisory, NEVER a modal/blocking; it degrades to nothing when prDetail is undefined (hard reload pre-hydration / gh down), the header keeping its task-field fallbacks"
  - "Banner + menu both reuse the existing CleanupWorktreeDialog (trigger='menu'): manual semantics keep the task row and null the worktree, so re-opening the PR re-provisions a fresh worktree"
  - "Build artifact hygiene: ignore the root ./kangent (go build ./cmd/kangent output) in .gitignore; only web/dist/index.html stays tracked as the //go:embed placeholder (hashed assets are gitignored and rebuilt)"

patterns-established:
  - "Pattern: surface a server-side lifecycle state (PRDetailWire.state) as an inline, degrade-don't-break advisory wired to an existing remediation flow — no new modal, no blocking"

requirements-completed: [GHCLN-01, GHCLN-02, GHCLN-03]

# Metrics
duration: 2min active (+ ~2h human-verify gate)
completed: 2026-06-14
---

# Phase 13 Plan 03: PR Review Cleanup Menu + Merged/Closed Banner Summary

**The manual escape hatch + discoverability path for Phase 13: a PR review view re-gains a `⋯` menu with a SINGLE "Clean up worktree" item (no Delete task, D-08) opening the existing gated `CleanupWorktreeDialog`, plus an inline non-blocking merged/closed banner (D-09) shown when `prDetail.state !== "OPEN"` — both inside the `shrink-0` header block (no 12-07 fix-#6 terminal-height regression) — then a human-verify confirmed GHCLN-01/02/03 end-to-end (auto-cleanup of a clean merged worktree, safety skip of a dirty/busy one with the branch kept, manual cleanup + banner).**

## Performance

- **Duration:** ~2 min active executor work for the two autonomous tasks; Task 3 was a blocking human-verify gate (~2h wall, mostly human-run scenarios against a real GitHub repo)
- **Started:** 2026-06-14T10:45:58Z
- **Completed:** 2026-06-14 (human-verify approved)
- **Tasks:** 3 (2 autonomous + 1 human-verify)
- **Files modified:** 3 (1 source, 2 build/config)

## Accomplishments

- **PR-specific `⋯` menu (D-08, GHCLN-03):** a separate menu gated on `isPR && task.worktree_path` with a single "Clean up worktree" item — no "Delete task" (a PR review is GitHub-synced, not a board task). It calls `setCleanupOpen(true)`, opening the already-rendered gated `CleanupWorktreeDialog` (confirm/force/stop-sessions). The existing `{!isPR && (...)}` manual-task menu (with its Delete-task item) is left exactly as-is.
- **Merged/closed banner (D-09):** an inline `role="status"` amber advisory rendered when `prDetail && prDetail.state !== "OPEN"`, reading "This PR was merged." / "This PR was closed." with a "Clean up worktree" link (shown while `task.worktree_path` exists) that opens the same dialog. It lives inside the `w-full shrink-0` header block as a sibling of the merge line (wrapped in a fragment), so it never participates in the tabs/terminal flex chain — the 12-07 fix-#6 height chain stays intact.
- **Degrade-don't-break:** when `prDetail` is undefined (hard reload before re-hydration, or `gh` down) the banner simply doesn't render and the header keeps its task-field fallbacks; the clean auto-removed case has its row deleted by the reaper (the open tab degrades to "Task not found" on next fetch — acceptable, D-09), so the banner serves the skipped / not-yet-reaped window.
- **Fresh embedded build:** rebuilt `web/dist` (vite) so the `//go:embed dist` SPA the Go binary serves reflects the 13-03 UI; the verification server was restarted on the freshly built binary (forcing an immediate `reconcilePRsOnce` pass) for the human-verify run.
- **Phase 13 human-verified (GHCLN-01/02/03):** the user confirmed end-to-end in the running app — (1) a clean merged PR's pristine, idle review worktree was auto-removed and its task row deleted; (2) a dirty/busy merged PR's worktree was left in place with its branch kept; (3) manual cleanup via the `⋯` menu / banner link drove the gated dialog to remove the worktree while keeping the row.

## Task Commits

1. **Task 1: PR review ⋯ menu (Clean up only) + merged/closed banner** - `0afd7e0` (feat)
2. **Task 2: rebuild embedded SPA for the 13-03 review UI** - `60dbea4` (chore)
3. **Task 3: human-verify the three phase success criteria** - APPROVED 2026-06-14 (no code commit — verification gate)

## Files Created/Modified

- `web/src/pages/TaskPage.tsx` - added a PR-only `⋯` menu (single Clean-up item, gated `isPR && task.worktree_path`) and an inline merged/closed banner (gated `prDetail.state !== "OPEN"`), both inside the shrink-0 header block; non-PR menu unchanged
- `web/dist/index.html` - rebuilt embed placeholder (new hashed asset references) so the Go binary serves the 13-03 UI
- `.gitignore` - ignore the root `./kangent` `go build ./cmd/kangent` output

## Decisions Made

- **PR menu gated on `task.worktree_path`** (mirroring the non-PR Clean-up gate): with no worktree there is nothing to clean up. A clean auto-removed PR has had its row deleted by the reaper (it never reaches this view); a gate-skipped one still has `worktree_path` set.
- **Separate PR-only menu, not a relaxed gate:** keeping the manual-task menu's `{!isPR && (...)}` block byte-for-byte unchanged means its Delete-task item is provably untouched, and the PR menu provably has only "Clean up worktree".
- **Banner inside the shrink-0 header block, wrapped in a fragment with the merge line:** the simplest placement that keeps it out of the flex chain (12-07 fix #6) — never a modal, never blocking.
- **`.gitignore` the root `kangent` binary:** `go build ./cmd/kangent` drops a `kangent` binary at the repo root (distinct from the Makefile's `bin/kangent`); ignored so it's never committed. Only `web/dist/index.html` stays tracked as the embed placeholder.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Restarted a stale pre-13-03 server before the human-verify gate**
- **Found during:** Task 3 prep (starting the server for the user).
- **Issue:** A kangent server (pid 828317, the Makefile's `./bin/kangent`) was already running on 127.0.0.1:7333 from a PREVIOUS build — it served the OLD UI without the 13-03 ⋯ menu/banner, so the human-verify would have tested stale code.
- **Fix:** Rebuilt `bin/kangent` with the fresh embed, sent SIGTERM to the stale process (port freed cleanly), and started the fresh binary in the background. The restart also forced an immediate `reconcilePRsOnce` pass (the reaper runs both passes at start), which conveniently lets the tester trigger auto-cleanup without waiting the ~10-min tick. Live PTY sessions from the old process were killed (expected — tmux survivors auto-reattach on reopen; agents offer Resume).
- **Files modified:** none (operational, not a code change)
- **Commit:** n/a (build/runtime action)

**2. [Rule 3 - Blocking] Ignored the root `./kangent` build output**
- **Found during:** Task 2 (post-build untracked-file check).
- **Issue:** The plan's verify uses `go build ./cmd/kangent`, which writes a `kangent` binary at the repo root. It was untracked and `.gitignore` only ignored `bin/`, risking an 18MB binary being committed.
- **Fix:** Added `/kangent` to `.gitignore`; confirmed `git check-ignore kangent` matches. Committed alongside the dist rebuild (Task 2 chore commit).
- **Files modified:** .gitignore
- **Commit:** 60dbea4

## Issues Encountered

- None beyond the two blocking-issue fixes above. The plan's described seams (line numbers, `prDetail.state`, `cleanupOpen`, the shrink-0 block) matched the live code exactly; `npx tsc --noEmit` passed first try.

## Known Stubs

None — the menu and banner are fully wired to live data (`PRDetailWire.state` from 13-01, the existing `CleanupWorktreeDialog`). No placeholder/empty-data patterns introduced.

## User Setup Required

None — `gh` (already authenticated on the host) is the only external dependency, and the feature degrades to nothing when `prDetail` is unavailable.

## Next Phase Readiness

- **Phase 13 is feature-complete and human-verified:** GHCLN-01/02/03 confirmed end-to-end (auto-cleanup clean, safety skip + branch kept, manual cleanup + banner). REQUIREMENTS.md marks all three Complete.
- This is the last v1.3 plan. The orchestrator runs the gsd-verifier next to close Phase 13; after that the milestone (v1.3 GitHub PR Review) is ready for completion review.

## Self-Check: PASSED

---
*Phase: 13-pr-worktree-auto-cleanup*
*Completed: 2026-06-14*

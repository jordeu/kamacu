---
phase: 23-worktree-cleanup-panel
plan: 03
subsystem: ui
tags: [react, tanstack-query, shadcn, worktrees, settings, alert-dialog, badge]

# Dependency graph
requires:
  - phase: 23-02
    provides: "backend endpoints GET /api/worktrees, POST remove/clean-eligible/clear-pointer with blocked-outcome + classification/flag annotation"
  - phase: 23-04
    provides: "useWorktreeList/useRemoveWorktree/useCleanEligible/useClearPointer hooks + TS types + shadcn Badge primitive"
provides:
  - "Settings worktree-cleanup panel: per-project grouped list of every worktree with classification + Dirty/Unpushed/Stash chips + task/PR association"
  - "Per-row force-remove behind a type-gated confirm that names what is destroyed and keeps the branch"
  - "D-01 blocked-row banner with a copyable (never executed) sudo rm -rf hint"
  - "Bulk clean-eligible preview→confirm over the exact server-computed safe set (non-destructive by construction)"
  - "Fetch-on-open + spinning manual Refresh, no polling"
affects: [24-review-menu-polish, worktree-cleanup, settings]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Reset-on-open dialogs via mount-only-while-open inner body (no setState-in-effect)"
    - "Blocked outcome renders an informational banner, distinct from a red error line"

key-files:
  created:
    - web/src/components/settings/WorktreeRow.tsx
    - web/src/components/settings/ForceRemoveDialog.tsx
    - web/src/components/settings/CleanEligibleDialog.tsx
    - web/src/components/settings/WorktreeCleanupSection.tsx
  modified:
    - web/src/pages/SettingsPage.tsx

key-decisions:
  - "The WorktreeRow snapshot feeds ForceRemoveDialog directly (no per-dialog GET) since the server re-checks all gates at remove time; the type-gate target is re-derived from row.path basename (Pitfall 8)."
  - "Reset-on-open state is achieved by mounting each dialog's body only while open (fresh mount = fresh state), avoiding the setState-in-effect ESLint advisory the CleanupWorktreeDialog analog carries."
  - "The blocked banner's sudo hint uses the server-named blocked_path when present, else the worktree path; it is passed only to navigator.clipboard.writeText — never executed (D-01 / T-23-12)."

patterns-established:
  - "Mount-only-while-open dialog body for reset-on-open without setState-in-effect"
  - "Section-scoped loading/empty/error states (never blank the whole Settings page)"

requirements-completed: [WTREE-01, WTREE-02, WTREE-03, WTREE-04]

# Metrics
duration: 40min
completed: 2026-07-02
---

# Phase 23 Plan 03: Worktree Cleanup Panel UI Summary

**Settings worktree-cleanup panel: per-project grouped worktree list with classification/flag chips, type-gated force-remove, a copyable-but-never-run sudo hint for permission-blocked shells, and a non-destructive bulk clean-eligible preview — fetch-on-open with a spinning manual Refresh, no polling.**

## Performance

- **Duration:** ~40 min
- **Started:** 2026-07-02T14:12:00Z
- **Completed:** 2026-07-02T14:51:39Z
- **Tasks:** 2
- **Files modified:** 5 (4 created, 1 modified)

## Accomplishments
- `WorktreeRow`: a dense flex row with the classification badge (Referenced/Orphan/Stale pointer/Blocked), a truncating mono path, Dirty (amber glyph)/Unpushed/Stash flag chips rendered only when present, Task/PR association with a quiet `· merged/closed/open` suffix, and the correct per-classification action (Remove / Clear pointer / blocked banner).
- D-01 blocked-row banner: informational left-accent banner with a `font-mono` `sudo rm -rf <path>` line and a Copy→Copied button that only calls `navigator.clipboard.writeText` — the app never executes the command.
- `ForceRemoveDialog`: mirrors `CleanupWorktreeDialog` — names sessions/dirty/unpushed/stash, always ends with the branch-kept reassurance, type-gates on the worktree basename when dirty, keeps the dialog open until the mutation lands, and degrades a `{outcome:"blocked"}` response to the row banner (not a red error).
- `CleanEligibleDialog`: previews the exact server-computed safe set (dry-run on open), lists each item as `project · path · why`, shows the reassurance copy, and removes on confirm with a DEFAULT-variant (non-destructive) CTA plus a `Removed X; Y skipped.` summary.
- `WorktreeCleanupSection`: owns the single `["worktrees"]` query (fetch-on-mount, no poll), renders the count `{total} worktrees · {orphaned} orphaned`, the spinning DiffTab-pattern Refresh, the Clean-eligible header button, per-project `ProjectAvatar` group headers, and section-scoped skeleton/empty/error states. Wired in as one more `<section>` in the 640px Settings column.

## Task Commits

Each task was committed atomically:

1. **Task 1: WorktreeRow + ForceRemoveDialog + CleanEligibleDialog** - `84c9df0` (feat)
2. **Task 2: WorktreeCleanupSection + wire into SettingsPage** - `04dcd9c` (feat)

_Task 2's commit also refactored the two dialog bodies from Task 1 (mount-only-while-open) so the section passes lint with no new advisories — see Deviations._

## Files Created/Modified
- `web/src/components/settings/WorktreeRow.tsx` - One worktree row: classification/flag chips, association, Remove/Clear-pointer actions, and the D-01 blocked banner with a copyable sudo hint.
- `web/src/components/settings/ForceRemoveDialog.tsx` - Force-remove confirm (type-gated when dirty, branch-kept, blocked→banner).
- `web/src/components/settings/CleanEligibleDialog.tsx` - Bulk preview→confirm over the server-computed safe set; non-destructive CTA.
- `web/src/components/settings/WorktreeCleanupSection.tsx` - The Settings section: query owner, header controls, per-project groups, dialogs.
- `web/src/pages/SettingsPage.tsx` - Appends `<WorktreeCleanupSection />` after `GithubSection` in the 640px column.

## Decisions Made
- Passed the `WorktreeRow` snapshot straight into `ForceRemoveDialog` (no separate fetch) because the backend re-checks every gate at remove time; the dirty type-gate target is re-derived from `row.path.split("/").pop()` per Pitfall 8.
- The blocked state is tracked locally in `WorktreeRow`, seeded from `row.blocked`/`row.blocked_path` and also settable when a remove returns `{outcome:"blocked", path}`, so the banner appears without waiting for the list refetch.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Refactored dialog bodies to avoid new lint errors**
- **Found during:** Task 2 (lint verification)
- **Issue:** The initial `ForceRemoveDialog`/`CleanEligibleDialog` copied `CleanupWorktreeDialog`'s reset-on-open `useEffect` that calls setState synchronously, adding 2 NEW `react-hooks/set-state-in-effect` errors (raising the count from 8 to 10). The plan's Task 2 done-criteria requires "no NEW errors beyond the pre-existing advisories."
- **Fix:** Split each dialog into a thin outer wrapper (owns `open`) and an inner body mounted only while `open` (`{open && <Body/>}`), so typed-confirm/preview state and the mutation reset on a fresh mount instead of a setState-in-effect. The `CleanEligibleDialog` preview effect now only *calls* the mutation (result captured in `onSuccess`), never setState synchronously.
- **Files modified:** web/src/components/settings/ForceRemoveDialog.tsx, web/src/components/settings/CleanEligibleDialog.tsx
- **Verification:** `npx eslint` on all five files → 0 errors; full-tree `npm run lint` back to the pre-existing baseline (21 problems, `set-state-in-effect` back to 8).
- **Committed in:** `04dcd9c` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** The refactor removed self-introduced lint noise and left behavior identical (reset-on-open, keep-open-until-mutation-lands). No scope creep; the panel matches the approved UI-SPEC.

## Issues Encountered
None beyond the lint deviation above. The `web/dist/index.html` `//go:embed` placeholder was overwritten by `vite build` during verification and reverted (`git checkout --`) before each commit so only source files landed.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The full worktree-cleanup feature (backend 23-02, hooks/primitive 23-04, UI 23-03) is now assembled; the plan's phase-level human-verify checkpoint (browser walkthrough of the panel) is the remaining gate.
- `npm run build` (tsc -b + vite) is green; `npm run lint` introduces no new advisories.

---
*Phase: 23-worktree-cleanup-panel*
*Completed: 2026-07-02*

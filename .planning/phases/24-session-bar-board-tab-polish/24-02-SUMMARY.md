---
phase: 24-session-bar-board-tab-polish
plan: 02
subsystem: ui
tags: [react, active-sessions-bar, kanban, quick-add, outside-click, localStorage]

# Dependency graph
requires:
  - phase: 24-session-bar-board-tab-polish
    provides: ActiveSessionsBar collapsed-bar CountGroups + collapse() helper + kamacu:sessions-bar-collapsed persistence; Column/QuickAdd To Do quick-add
provides:
  - Collapsed active-sessions bar without the total-sessions count (working/waiting/idle colored counts only)
  - Outside-click auto-collapse for the expanded active-sessions bar via a ref-guarded document mousedown listener reusing collapse()
  - To Do column with the inline "+ New task" quick-add removed and QuickAdd.tsx deleted
affects: [active-sessions-bar, board, task-creation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Ref-guarded document mousedown outside-click listener registered/torn down via useEffect cleanup (mirrors the TaskPage document-listener idiom); no-op while already collapsed"
    - "Inside clicks handled by barRef.current.contains() so the bar's own onClick={toggle} keeps working without the outside-click listener double-firing"

key-files:
  created: []
  modified:
    - web/src/components/layout/ActiveSessionsBar.tsx
    - web/src/components/board/Column.tsx
    - web/src/components/board/Board.tsx
  deleted:
    - web/src/components/board/QuickAdd.tsx

key-decisions:
  - "D-11 phase: used the mousedown BUBBLE phase (not capture) so nested surfaces inside the bar register as inside via the ref-contains check"
  - "D-11 Escape: deliberately did NOT add Escape-to-collapse this pass — the auto-collapse gesture is click-only (scope choice)"
  - "Dropped the now-unused projectId prop from Column (QuickAdd was its sole consumer) rather than leaving an unused prop that trips no-unused-vars"

patterns-established:
  - "Outside-click dismissal: useEffect guarded on collapsed state + ref-contains check calling the shared collapse() helper for persistence consistency"

requirements-completed: [POLISH-01, POLISH-02, POLISH-03]

# Metrics
duration: ~12min
completed: 2026-07-03
---

# Phase 24 Plan 02: Session-Bar & Board Tab Polish Summary

**Collapsed active-sessions bar drops the total count and now auto-collapses on an outside click via the shared collapse() helper; the To Do column's inline "+ New task" quick-add is removed and QuickAdd.tsx is deleted.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-03T17:18Z
- **Completed:** 2026-07-03T17:30Z
- **Tasks:** 2
- **Files modified:** 3 (+1 deleted)

## Accomplishments
- Removed the total-sessions `{total}` render span from the collapsed active-sessions bar; the working/waiting/idle `CountGroup`s remain (POLISH-01).
- Added a ref-guarded document `mousedown` listener that collapses the expanded bar via the existing `collapse()` helper (persists `"1"` under `kamacu:sessions-bar-collapsed`); inside clicks are ignored so the bar's own `onClick={toggle}` still toggles (POLISH-02).
- Removed the To Do column's inline `+ New task` quick-add and deleted `QuickAdd.tsx` with zero dangling references; task creation survives via BoardPage's header "New task" button and the `n` shortcut → NewTaskDialog (POLISH-03).

## Task Commits

Each task was committed atomically:

1. **Task 1: Drop the total count + add outside-click auto-collapse (POLISH-01/02)** - `c342276` (feat)
2. **Task 2: Remove the To Do quick-add + retire QuickAdd.tsx (POLISH-03)** - `94888d7` (feat)

## Files Created/Modified
- `web/src/components/layout/ActiveSessionsBar.tsx` - Removed the `{total}` collapsed-bar span; added `useEffect`/`useRef` imports, a `barRef` on the outer `fixed inset-x-0 bottom-0` wrapper, and a `collapsed`-guarded document `mousedown` listener that calls `collapse()` on outside clicks. `total` variable retained (still read by the `total === 0` empty-state guards).
- `web/src/components/board/Column.tsx` - Removed the `QuickAdd` import and the `status === "todo" && <QuickAdd />` render; updated the `topSlot` doc-comment; dropped the now-unused `projectId` prop.
- `web/src/components/board/Board.tsx` - Stopped passing `projectId` to `<Column>` (prop removed); `projectId` still flows to `<ReviewColumn>`.
- `web/src/components/board/QuickAdd.tsx` - **Deleted** (Column was its only importer).

## Decisions Made
- **D-11 listener phase:** used the `mousedown` bubble phase (not capture) so surfaces nested inside the bar count as "inside" via the ref-contains check.
- **D-11 Escape scope:** deliberately kept the auto-collapse gesture click-only for this pass — no Escape-to-collapse added (noted as an intentional scope choice, not an omission).
- **Effect guard:** the outside-click effect early-returns when already `collapsed`, so no global listener is registered while the bar is collapsed (mitigates T-24-05; cleanup tears down on state change).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed the now-unused `projectId` prop from Column**
- **Found during:** Task 2 (Remove the To Do quick-add + retire QuickAdd.tsx)
- **Issue:** After deleting the `<QuickAdd projectId={projectId} />` render, `QuickAdd` was `Column`'s only consumer of `projectId`. The destructured `projectId` param became unused, which the project's ESLint (`no-unused-vars`) would flag — a blocking lint failure caused directly by this task's change.
- **Fix:** Removed `projectId` from the `ColumnProps` interface and the destructure, and stopped passing it at the single call site in `Board.tsx` (`projectId` still flows to `<ReviewColumn>`, so `Board`'s own prop stays used).
- **Files modified:** web/src/components/board/Column.tsx, web/src/components/board/Board.tsx
- **Verification:** `npm run build` exits 0; `npm run lint` reports no errors in `Column.tsx` or `Board.tsx` line 201 area (the only Board.tsx error is the pre-existing line-75 `set-state-in-effect`); total lint problem count unchanged at 21 (zero new errors introduced).
- **Committed in:** 94888d7 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking).
**Impact on plan:** The `projectId`-prop removal was a mechanical consequence of deleting the quick-add and was required to keep the lint gate from regressing. No scope creep — the plan's intent (retire the quick-add cleanly) is fully served.

## Issues Encountered
- **Fresh worktree lacked `node_modules`.** `npm run build`/`lint` initially failed with `tsc: not found` / `eslint: not found`. Resolved by running `npm ci` (lockfile-pinned install, not a new-package install) to restore the dependency tree for verification.
- **Stale tracked `web/dist/index.html`.** Each `npm run build` rewrote the tracked `dist/index.html` with new hashed asset names AND pre-existing template drift (title `Kangent`→`Kamacu`, a favicon link) that predates this plan. Since `dist/assets/*` is gitignored and the template drift is unrelated to 24-02, the rebuilt `dist/index.html` was restored (`git checkout -- web/dist/index.html`) after each build so only source changes were committed. No `dist` artifact is part of these commits.

## Pre-existing Lint Failures (Out of Scope)
`npm run lint` exits 1 with **21 pre-existing `react-hooks/set-state-in-effect` errors** across 16 files this plan did not modify (e.g. `hooks/use-mobile.ts`, `pages/TaskPage.tsx`, `components/board/Board.tsx:73-75`). The count was 21 before and after this plan — **zero introduced by 24-02**. Logged to `.planning/phases/24-session-bar-board-tab-polish/deferred-items.md`; not fixed per the scope boundary (unrelated to this UI-polish plan).

## Known Stubs
None. No hardcoded empty values, placeholders, or unwired data sources were introduced.

## Threat Flags
None. This plan is pure client-side UI (removed a render span, added a DOM click listener bounded by useEffect cleanup, deleted a component). No new server input, network endpoints, auth paths, or persisted data beyond the existing `kamacu:sessions-bar-collapsed` key already written by `collapse()`. No packages installed (T-24-SC N/A).

## Next Phase Readiness
- Both POLISH items on the active-sessions bar and the board quick-add are complete; nothing blocks other Wave-1 24-xx plans (this plan has `depends_on: []`).
- Task creation paths (header "New task" button + `n` shortcut → NewTaskDialog) remain intact as the D-12 safety net.

## Self-Check: PASSED

- FOUND: `.planning/phases/24-session-bar-board-tab-polish/24-02-SUMMARY.md`
- FOUND: `web/src/components/layout/ActiveSessionsBar.tsx`
- FOUND: `web/src/components/board/Column.tsx`
- CONFIRMED DELETED: `web/src/components/board/QuickAdd.tsx`
- FOUND commit: `c342276` (Task 1)
- FOUND commit: `94888d7` (Task 2)

---
*Phase: 24-session-bar-board-tab-polish*
*Completed: 2026-07-03*

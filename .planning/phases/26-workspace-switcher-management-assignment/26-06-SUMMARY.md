---
phase: 26-workspace-switcher-management-assignment
plan: 06
subsystem: ui
tags: [react, react-router, tanstack-query, workspaces, sidebar, dropdown-menu, dnd-free-transfer]

# Dependency graph
requires:
  - phase: 26-03
    provides: "useActiveWorkspace() context + useWorkspaces()/useMoveProject() hooks"
  - phase: 26-04
    provides: "WorkspaceSwitcher component (expanded-only, self-hiding in the icon rail)"
  - phase: 26-05
    provides: "BoardWorkspaceSync URL-wins reconciliation + workspace-scoped index redirect/empty state"
provides:
  - "WorkspaceSwitcher mounted at the top of the expanded sidebar (WSNAV-01)"
  - "Sidebar project list (expanded rows AND collapsed icon rail) filtered to the active workspace via a single .filter feeding both surfaces (WSNAV-02, D-12)"
  - "Move-to submenu on the project ⋯ menu: radio items per workspace, current workspace checked+disabled, transfer + follow-the-project (WSPROJ-01, D-08/D-09)"
  - "Cross-workspace Active Sessions bar confirmed unaffected (WSBAR-01 non-regression)"
  - "Switcher-initiated active-workspace changes (incl. to EMPTY workspaces) no longer reverted by BoardWorkspaceSync (post-gate fix)"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Single .filter((p) => p.workspace_id === activeWorkspaceId) on the shared project map filters BOTH the expanded rows and the collapsed rail at once"
    - "DropdownMenuSub + DropdownMenuRadioGroup transfer submenu with the current workspace checked+disabled (no drag-and-drop for project→workspace assignment)"
    - "URL-project-change-gated reconciliation: BoardWorkspaceSync re-anchors the active workspace ONLY when the URL's projectId changes (ref guard), never when activeWorkspaceId shifts under a stationary URL"

key-files:
  created: []
  modified:
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/src/components/sidebar/ProjectMenu.tsx
    - web/src/App.tsx

key-decisions:
  - "Phase 26-06: BoardWorkspaceSync reconciles the active workspace to the URL project ONLY on a genuine URL-project change (guarded by a lastSyncedProjectId ref) — a workspace switch under a stationary URL is left alone. This closes the empty-workspace-switch bug while preserving D-14 URL-wins deep-link behavior."
  - "Phase 26-06: the sidebar filters both surfaces with a single .filter on the shared project map (D-12); when activeWorkspaceId is null (resolving) the list is briefly empty and fills in once the hook resolves."
  - "Phase 26-06: the transfer submenu uses radio items with the project's current workspace checked+disabled (D-08); moving the currently-open project also flips the active workspace so the view follows it (D-09)."
  - "Phase 26-06: the Move submenu trigger shows 'Move to' (single-line) with aria-label='Move to workspace' preserving the descriptive accessible name."

patterns-established:
  - "Idempotent-per-URL-project reconciliation: gate a URL→state sync effect behind a ref of the last-synced route param so orthogonal state changes on the same route don't re-trigger (and revert) the sync"

requirements-completed: [WSNAV-01, WSNAV-02, WSPROJ-01, WSBAR-01]

# Metrics
duration: 12min
completed: 2026-07-06
---

# Phase 26 Plan 06: Sidebar Switcher Mount, Per-Workspace Filter & Project Transfer Summary

**The workspace layer is wired into the sidebar end-to-end: the switcher sits atop the expanded sidebar, both project surfaces filter to the active workspace, projects transfer (and the open one follows) from the ⋯ menu, and the cross-workspace Active Sessions bar is confirmed untouched — human-verified across all 10 checks after two post-gate fixes.**

## Performance

- **Duration:** ~12 min active (implementation + post-gate fixes; excludes the overnight human-verify wait)
- **Started:** 2026-07-05T21:54:37Z (Task 1 commit)
- **Completed:** 2026-07-06T06:32:18Z (final fix commit; gate approved thereafter)
- **Tasks:** 3 (2 auto + 1 human-verify gate — PASSED)
- **Files modified:** 3

## Accomplishments
- Mounted `<WorkspaceSwitcher />` at the top of `SidebarContent` (expanded-only; it self-hides in the collapsed icon rail), and filtered the shared project map with `.filter((p) => p.workspace_id === activeWorkspaceId)` so BOTH the expanded rows and the collapsed rail show only the active workspace's projects (WSNAV-01, WSNAV-02, D-12). `waitingByProject`/`useAgentStatuses` untouched (WSBAR-01).
- Added the "Move to" transfer submenu to the project ⋯ menu: a `DropdownMenuRadioGroup` valued at the project's current workspace with one radio item per workspace (current one checked + disabled); selecting another workspace calls `useMoveProject().mutate(...)`, and when the moved project is the currently-open one it also flips the active workspace so the view follows it (WSPROJ-01, D-08/D-09).
- Confirmed the global Active Sessions bar remains cross-workspace and unaffected by the active-workspace filter (WSBAR-01 non-regression).
- **Post-gate fix (Issue 1):** stopped `BoardWorkspaceSync` from reverting switcher-initiated active-workspace changes, fixing the "switching to an empty workspace does nothing" bug.
- **Post-gate fix (Issue 2):** shortened the submenu trigger label from "Move to workspace" (which wrapped to two lines) to "Move to", keeping "Move to workspace" as the accessible name.

## Task Commits

Each task/fix was committed atomically:

1. **Task 1: Mount the switcher + filter the project list by active workspace** - `518a09b` (feat)
2. **Task 2: "Move to workspace" transfer submenu + follow-the-project** - `23d15c9` (feat)
3. **Task 3: End-of-phase human verification (WSMGMT / WSNAV / WSPROJ / WSBAR)** - human-verify gate, **PASSED** (all 10 checks confirmed by the user)

Post-gate fixes (applied while the gate was open, before approval):

4. **Fix 1: BoardWorkspaceSync ref-guard (empty-workspace switch)** - `8367783` (fix)
5. **Fix 2: "Move to" submenu label** - `f09cfba` (fix)

**Plan metadata:** _(this docs commit)_

## Files Created/Modified
- `web/src/components/sidebar/ProjectSidebar.tsx` - Mounts `<WorkspaceSwitcher />` at the top of `SidebarContent`; the single project `.map` is prefixed with `.filter((p) => p.workspace_id === activeWorkspaceId)`, filtering both the expanded rows and the collapsed rail. Active Sessions inputs (`useAgentStatuses`/`waitingByProject`) unchanged.
- `web/src/components/sidebar/ProjectMenu.tsx` - Adds the transfer submenu (`DropdownMenuSub`/`DropdownMenuRadioGroup`/`DropdownMenuRadioItem`) with transfer + follow-the-project via `useMoveProject()` and `setActiveWorkspaceId(...)`; the trigger label is "Move to" with `aria-label="Move to workspace"`.
- `web/src/App.tsx` - `BoardWorkspaceSync` now gates its URL→active-workspace reconciliation behind a `lastSyncedProjectId` ref so it only re-anchors on a genuine URL-project change (post-gate Fix 1).

## Decisions Made
- **BoardWorkspaceSync reconciles only on URL-project change (ref-gated), not on activeWorkspaceId change.** This is the core of Fix 1: reconciliation is now idempotent per URL project, so the switcher (which changes the active workspace while leaving the URL momentarily stationary) is never reverted, while D-14 URL-wins deep-link/reload reconciliation is fully preserved.
- **A single `.filter` on the shared project map filters both sidebar surfaces (D-12)** — the expanded rows and the collapsed `size-10` rail avatars are the same `.map`, so one filter covers both.
- **Transfer is radio-based with the current workspace checked+disabled (D-08); the open project follows on move (D-09).** No drag-and-drop for project→workspace assignment — it lives in the existing ⋯ menu.
- **The Move submenu label is "Move to" with a fuller `aria-label="Move to workspace"`** — fixes the two-line wrap while keeping a descriptive accessible name.

## Deviations from Plan

The two autonomous tasks executed as written. Two issues surfaced at the end-of-phase human-verify gate and were fixed while the gate was still open (before approval):

### Auto-fixed Issues

**1. [Rule 1 - Bug] Switching to an EMPTY workspace did nothing (silent revert)**
- **Found during:** Task 3 (human-verify gate, check #4)
- **Issue:** Picking a workspace with no projects left the user on (or bounced back to) the old workspace's project instead of showing the workspace-scoped empty state. **Root cause (traced):** React Router v7 runs `navigate()` inside `React.startTransition`, so the URL change is a low-priority update that lags the synchronous `setActiveWorkspaceId()` `useState` update in `WorkspaceSwitcher.handlePick`. That yields an intermediate render where `activeWorkspaceId` has already flipped to the empty target but the URL is still `/projects/<old>` — so `BoardWorkspaceSync` is still mounted, its effect (deps included `activeWorkspaceId`) re-fired, saw `projectWorkspaceId(old) !== activeWorkspaceId(new)`, and called `setActiveWorkspaceId(old)`, reverting the switch; the follow-up `navigate("/")` then hit `RedirectToFirstProject`, now back in the old workspace, which bounced to that workspace's first project. Non-empty targets self-healed because the new project URL re-anchored to the target workspace; empty targets had no anchor, so the revert stuck.
- **Fix:** Gated `BoardWorkspaceSync`'s reconciliation behind a `lastSyncedProjectId` ref — it re-anchors the active workspace ONLY when the URL's `projectId` changes since the last sync (deep-link / navigation TO a project), never when `activeWorkspaceId` changes underneath a stationary URL. Effect deps stay honest (`[projectId, projectWorkspaceId, activeWorkspaceId, setActiveWorkspaceId]`); no eslint-disable / exhaustive-deps suppression.
- **Files modified:** web/src/App.tsx
- **Verification:** `cd web && npm run build` green; `npx eslint src/App.tsx` clean; human-verify check #4 (switch to empty workspace) and check #9 (deep-link URL-wins) both confirmed by the user.
- **Committed in:** `8367783`

**2. [Rule 1 - UI polish] "Move to workspace" submenu label wrapped to two lines**
- **Found during:** Task 3 (human-verify gate, check #7)
- **Issue:** The `DropdownMenuSubTrigger` label "Move to workspace" wrapped onto two lines in the dropdown.
- **Fix:** Changed the visible label to "Move to" and added `aria-label="Move to workspace"` to preserve the descriptive accessible name. Submenu behavior unchanged.
- **Files modified:** web/src/components/sidebar/ProjectMenu.tsx
- **Verification:** `cd web && npm run build` green; `npx eslint src/components/sidebar/ProjectMenu.tsx` clean; human-verify check #7 confirmed.
- **Committed in:** `f09cfba`

---

**Total deviations:** 2 auto-fixed (2 bugs surfaced at the human-verify gate — 1 behavioral, 1 UI).
**Impact on plan:** Both fixes were necessary for the plan's behavior to match the 26-UI-SPEC contract; no scope creep. The `useMoveProject` invalidate-not-optimistic pattern meant the follow-the-project path shared the same latent fragility, which the ref-gate also hardens.

## Issues Encountered
- `cd web && npm run lint` still reports the ~22 pre-existing `react-hooks/*` and `react-refresh/only-export-components` errors in unrelated files (Board.tsx, PRCard.tsx, ReviewColumn.tsx, terminal hooks, ui primitives, etc.), already logged in `deferred-items.md` and STATE.md Pending Todos. Out of scope per the scope boundary — not fixed. The files touched by this plan lint clean in isolation (`npx eslint src/App.tsx src/components/sidebar/ProjectMenu.tsx` exits 0), and `npm run build` (`tsc -b && vite build`) is green.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 26 (Workspace Switcher, Management & Assignment) is now fully implemented and human-verified end to end. All 10 workspace requirements (WSMGMT-01..04, WSNAV-01..03, WSPROJ-01/02, WSBAR-01) are satisfied; this plan closes WSNAV-01, WSNAV-02, WSPROJ-01, WSBAR-01.
- The backend binary (`bin/kamacu`) was rebuilt to embed the current SPA. No blockers. The orchestrator owns phase-level verification / marking the whole phase complete.

## Known Stubs
None - no stubs introduced. Both feature files wire real data (`useActiveWorkspace`, `useWorkspaces`, `useMoveProject`, `useProjects`).

## Self-Check: PASSED

- FOUND: web/src/components/sidebar/ProjectSidebar.tsx
- FOUND: web/src/components/sidebar/ProjectMenu.tsx
- FOUND: web/src/App.tsx
- FOUND: .planning/phases/26-workspace-switcher-management-assignment/26-06-SUMMARY.md
- FOUND commit: 518a09b (Task 1)
- FOUND commit: 23d15c9 (Task 2)
- FOUND commit: 8367783 (Fix 1)
- FOUND commit: f09cfba (Fix 2)

---
*Phase: 26-workspace-switcher-management-assignment*
*Completed: 2026-07-06*

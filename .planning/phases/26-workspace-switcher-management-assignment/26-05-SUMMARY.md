---
phase: 26-workspace-switcher-management-assignment
plan: 05
subsystem: ui
tags: [react, react-router, tanstack-query, workspaces, routing, localstorage]

# Dependency graph
requires:
  - phase: 26-03
    provides: "useActiveWorkspace() context + useWorkspaces() query + useCreateProject workspace_id body"
  - phase: 26-02
    provides: "backend create resolves/validates workspace_id, falls back to Personal default"
provides:
  - "Workspace-aware index redirect: lands on the ACTIVE workspace's first project (ORDER BY name → [0], D-11)"
  - "Workspace-scoped empty state: 'No projects in {workspace} yet' instead of a global 'No projects yet' (D-13)"
  - "URL-wins deep-link sync: a deep-linked/reloaded project flips the active workspace to its own workspace (D-14)"
  - "AddProjectDialog creates new projects into the active workspace (WSPROJ-02 client side, D-10)"
affects: [26-06]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Route-element wrapper (BoardWorkspaceSync) that reconciles localStorage-backed state from the URL, then renders the underlying page unchanged"
    - "Workspace never enters the URL — active workspace is localStorage-only; the URL's projectId is the single source of truth for reconciliation"

key-files:
  created: []
  modified:
    - web/src/App.tsx
    - web/src/components/sidebar/AddProjectDialog.tsx

key-decisions:
  - "Phase 26-05: the index redirect filters projects to the active workspace before choosing [0]; returns null while activeWorkspaceId is unresolved (never redirects to a foreign-workspace project)"
  - "Phase 26-05: URL-wins reconciliation lives in a thin route-element wrapper (BoardWorkspaceSync) so both /projects/:projectId and /projects/:projectId/tasks/:taskId share it; the wrapper only mutates the localStorage-backed active workspace, never the route path"
  - "Phase 26-05: AddProjectDialog passes workspace_id: activeWorkspaceId ?? undefined — a null (unresolved) active workspace omits the field so the backend falls back to Personal"

patterns-established:
  - "Deep-link reconciliation via useEffect keyed on (resolved project workspace_id, activeWorkspaceId): sync only fires when the URL project is present in useProjects() and both sides differ"

requirements-completed: [WSNAV-03, WSPROJ-02]

# Metrics
duration: 6min
completed: 2026-07-05
---

# Phase 26 Plan 05: Workspace-Aware Routing & Creation Summary

**Routing and project creation now honor the active workspace: the index redirect lands on the active workspace's first project (or a workspace-scoped empty state), a deep-linked project flips the active workspace to its own (URL wins), and new projects are created into the active workspace.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-05T19:46:47Z
- **Completed:** 2026-07-05T19:47:47Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- `RedirectToFirstProject` filters projects by `p.workspace_id === activeWorkspaceId` before picking `[0]` (first-by-name, D-11), and returns `null` while `activeWorkspaceId` is still resolving.
- Workspace-scoped empty state — heading `No projects in {workspaceName} yet` (name resolved from `useWorkspaces()`), body "Add a project to this workspace to get a board.", primary "Add project" button opening `AddProjectDialog` (D-13). No more global "No projects yet" while other workspaces hold projects.
- New `BoardWorkspaceSync` route-element wrapper reconciles the active workspace from a deep-linked/reloaded project's `workspace_id` (URL wins, D-14) without touching the route path; used for both the board and task routes.
- `AddProjectDialog` reads `useActiveWorkspace()` and threads `workspace_id: activeWorkspaceId ?? undefined` into both create branches (repo + folder), so new projects land in the active workspace (WSPROJ-02, D-10) and stay visible under the sidebar filter with no extra work.

## Task Commits

Each task was committed atomically:

1. **Task 1: Workspace-aware redirect, empty state, and URL-wins deep-link sync** - `7902068` (feat)
2. **Task 2: AddProjectDialog creates into the active workspace** - `f98e045` (feat)

## Files Created/Modified
- `web/src/App.tsx` - Workspace-aware `RedirectToFirstProject` (filter + workspace-scoped empty state) and a new `BoardWorkspaceSync` wrapper providing URL-wins deep-link reconciliation for `/projects/:projectId` and `/projects/:projectId/tasks/:taskId`.
- `web/src/components/sidebar/AddProjectDialog.tsx` - Reads `useActiveWorkspace()`; both create bodies carry `workspace_id: activeWorkspaceId ?? undefined`.

## Decisions Made
- URL-wins reconciliation is a thin route-element wrapper rather than logic embedded in each page, so both board and task routes share one implementation and the pages stay unchanged.
- The reconciliation `useEffect` only fires when the URL project exists in `useProjects()` (T-26-07: never trust a `workspace_id` we can't see) and `activeWorkspaceId` has resolved (non-null), so it never sets a workspace from an unloaded/unknown project.
- Empty-state workspace name is auto-escaped React text (T-26-09), and a null active workspace omits `workspace_id` on create so the backend's Personal fallback applies (T-26-10) — matching the plan's threat register.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `cd web && npm run lint` reports 22 pre-existing `react-hooks/*` and `react-refresh/only-export-components` errors in files this plan does not touch (Board.tsx, PRCard.tsx, ReviewColumn.tsx, terminal hooks, ui primitives, etc.). These match STATE.md's "Pending Todos" note ("~18–20 pre-existing react-hooks eslint errors"). Out of scope per the scope boundary — logged to `deferred-items.md`, not fixed. The files modified by this plan lint clean (`npx eslint src/App.tsx src/components/sidebar/AddProjectDialog.tsx` exits 0), and `npm run build` (`tsc -b && vite build`) is green.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 06 (the phase's human-verify gate) can now exercise the full runtime flow: switch/restore lands on the active workspace's first project or its own empty state, deep-links reconcile the active workspace on reload, and new projects are created into the active workspace.
- No blockers.

## Known Stubs
None - no stubs introduced. Both modified files wire real data (`useActiveWorkspace`, `useProjects`, `useWorkspaces`).

## Self-Check: PASSED

- FOUND: web/src/App.tsx
- FOUND: web/src/components/sidebar/AddProjectDialog.tsx
- FOUND: .planning/phases/26-workspace-switcher-management-assignment/26-05-SUMMARY.md
- FOUND commit: 7902068 (Task 1)
- FOUND commit: f98e045 (Task 2)

---
*Phase: 26-workspace-switcher-management-assignment*
*Completed: 2026-07-05*

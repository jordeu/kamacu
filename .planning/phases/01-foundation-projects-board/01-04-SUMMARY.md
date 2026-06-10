---
phase: 01-foundation-projects-board
plan: 04
subsystem: ui
tags: [react, shadcn, sidebar, react-router, tanstack-query]

# Dependency graph
requires:
  - phase: 01-foundation-projects-board (plan 01-02)
    provides: REST API for projects (create with path validation, rename, delete) and its exact error messages
  - phase: 01-foundation-projects-board (plan 01-03)
    provides: typed API hooks (useProjects/useCreateProject/useRenameProject/useDeleteProject), ApiError, AppLayout shell, shadcn sidebar/dialog/alert-dialog/dropdown components
provides:
  - ProjectSidebar with project list, neutral active selection, per-row menu, Add project entry point
  - AddProjectDialog with mono repository-path input and inline sentence-cased server errors (values preserved)
  - ProjectMenu dropdown with Rename and Delete-with-AlertDialog (exact UI-SPEC copy)
  - RenameProjectDialog with prefilled name and inline errors
  - Sidebar collapse persisted via localStorage key kangent.sidebar (Ctrl/Cmd+B works)
  - No-projects empty state with centered Display heading and Add project CTA
affects: [01-05 board, 01-06 task view, 01-07 embed/build]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Controlled SidebarProvider open state backed by localStorage (shadcn cookie write is never read back in an SPA)
    - Server error mirroring - render ApiError.message sentence-cased inline under the offending field, keep field values
    - Dialog state owned by the opener; dialogs receive open/onOpenChange props

key-files:
  created:
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/src/components/sidebar/AddProjectDialog.tsx
    - web/src/components/sidebar/ProjectMenu.tsx
    - web/src/components/sidebar/RenameProjectDialog.tsx
  modified:
    - web/src/components/layout/AppLayout.tsx
    - web/src/App.tsx

key-decisions:
  - "Sidebar collapse persisted via controlled open/onOpenChange + localStorage (kangent.sidebar) because the generated shadcn provider only writes a cookie and never reads it in an SPA"
  - "409 duplicate-project message gets a trailing period appended after sentence-casing to match UI-SPEC copy exactly"
  - "Deleting the currently-routed project refetches [\"projects\"] before navigating to / so the index redirect never targets the deleted project from stale cache"
  - "Main-area sidebar trigger renders only while collapsed (offcanvas hides the in-sidebar trigger), avoiding duplicate triggers when expanded"

patterns-established:
  - "Sidebar feature components live in web/src/components/sidebar/"
  - "sentenceCase helper duplicated locally per dialog (3 lines) instead of widening shared lib surface during parallel waves"

requirements-completed: [PROJ-01, PROJ-02, PROJ-03]

# Metrics
duration: 7min
completed: 2026-06-10
---

# Phase 01 Plan 04: Projects Sidebar Summary

**Collapsible projects sidebar with add/rename/delete flows: mono-path Add dialog mirroring server validation errors inline, AlertDialog delete with repo-untouched copy, and localStorage-persisted collapse**

## Performance

- **Duration:** 7 min
- **Started:** 2026-06-10T06:56:54Z
- **Completed:** 2026-06-10T07:03:39Z
- **Tasks:** 2
- **Files modified:** 7 (4 created, 3 modified; AddProjectDialog/ProjectMenu stubbed in Task 1, filled in Task 2)

## Accomplishments

- PROJ-02: sidebar lists all projects as 28px Link rows to `/projects/{id}`; active project highlighted with the neutral zinc-800 surface (never accent)
- PROJ-01: Add project dialog with `font-mono` Repository path input; ApiError messages render sentence-cased as 12px destructive text under the field with values preserved; 409 renders "This repository is already added."
- PROJ-03: per-row dropdown with Rename (prefilled dialog) and Delete project behind an AlertDialog using the exact UI-SPEC copy ("The repository on disk is untouched."); deleting the current project navigates away safely
- D-04: collapse toggles via header trigger and Ctrl/Cmd+B; state persists across reloads through controlled provider state + localStorage
- Empty state: centered 20px/500 "No projects yet" heading, muted body, primary-inverted Add project CTA sharing the same dialog

## Task Commits

Each task was committed atomically:

1. **Task 1: ProjectSidebar — list, selection, collapse, empty state** - `39984b0` (feat)
2. **Task 2: Add project dialog, rename, delete-with-confirm** - `54867f7` (feat)

## Files Created/Modified

- `web/src/components/sidebar/ProjectSidebar.tsx` - Project list, header trigger with tooltip, Add project footer button, dialog/menu wiring
- `web/src/components/sidebar/AddProjectDialog.tsx` - Repository path + Name fields, inline server-error display, navigate on success
- `web/src/components/sidebar/ProjectMenu.tsx` - SidebarMenuAction-triggered dropdown: Rename / destructive Delete with AlertDialog confirm
- `web/src/components/sidebar/RenameProjectDialog.tsx` - Single-field rename dialog with prefill and inline errors
- `web/src/components/layout/AppLayout.tsx` - Controlled SidebarProvider (localStorage persistence, 240px width), collapsed-only reopen trigger
- `web/src/App.tsx` - No-projects empty state in RedirectToFirstProject

## Decisions Made

- **localStorage-backed collapse:** the generated shadcn SidebarProvider writes a `sidebar_state` cookie but never reads it (that read happens server-side in Next.js); D-04 persistence therefore uses controlled `open`/`onOpenChange` with key `kangent.sidebar`, exactly the plan's fallback path.
- **Duplicate-project copy:** server returns `this repository is already added` (no period); UI sentence-cases all ApiError messages and appends a period for 409 so the rendered copy matches UI-SPEC's "This repository is already added." while path errors stay verbatim (no period, per spec).
- **Safe delete navigation:** after deleting the currently-routed project, the projects query is refetched before navigating to `/`, so RedirectToFirstProject can't redirect into the deleted project's board from stale cache.
- **Single visible trigger:** main-area trigger renders only while collapsed (the offcanvas sidebar hides its own header trigger when closed) — one toggle visible in each state.

## Deviations from Plan

None - plan executed exactly as written. (Both Task 1 deviations the plan anticipated — dialog stubbing and the localStorage collapse fallback — were explicit plan instructions, not discoveries.)

## Known Stubs

None in this plan's files — both dialog stubs created in Task 1 were fully implemented in Task 2. Board/task-page stubs from plan 01-03 are owned by plans 01-05/01-06 (in progress in parallel).

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Project management UI complete; plans 01-05 (board) and 01-06 (task view) run in parallel and share no files with this plan
- Manual end-to-end flow (add real repo path, inline error for invalid path, rename, delete, Ctrl+B persistence) is exercisable once the backend (plans 01-01/01-02) is running — covered by phase-level verification

## Self-Check: PASSED

- All 6 key files verified on disk
- Commits 39984b0 and 54867f7 verified in git log
- `npm run build` exit 0; all acceptance-criteria greps pass (copy strings, hook wiring, no accent classes on rows)

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

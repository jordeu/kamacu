---
phase: 20-kamacu-rebrand-brand
plan: 03
subsystem: ui
tags: [react, tailwind, sidebar, branding, rebrand]

# Dependency graph
requires:
  - phase: 20-kamacu-rebrand-brand (plan 20-02)
    provides: KamacuMark ember-spark brand component (web/src/components/brand/KamacuMark.tsx)
  - phase: 20-kamacu-rebrand-brand (plan 20-01)
    provides: Go identity rename + index.html tab title already set to Kamacu
provides:
  - Sidebar header brand lockup rendering the KamacuMark in both expanded and collapsed states
  - Expanded-only "Kamacu" title-case wordmark replacing the old lowercase "kangent" text
  - Zero remaining user-facing "Kangent" copy in the frontend (empty state, delete dialog, settings help, comment prose)
affects: [20-04 human-verify checkpoint, README screenshot capture]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Collapsible dual-render idiom: mark shown in both states, wordmark gated expanded-only via group-data-[collapsible=icon]:hidden"

key-files:
  created: []
  modified:
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/src/App.tsx
    - web/src/components/sidebar/ProjectMenu.tsx
    - web/src/pages/SettingsPage.tsx
    - web/src/api/types.ts
    - web/src/api/sessions.ts

key-decisions:
  - "Wrapped the KamacuMark + wordmark in a single flex span so the mark stays visible when collapsed while only the wordmark hides; the existing SidebarTrigger + Tooltip block was kept intact and the header's justify classes preserved."
  - "Renamed only the product word 'Kangent' in types.ts:18 prose; left the ~/.kangent/repos/ path token on line 19 verbatim (Phase 21 keep-out)."

patterns-established:
  - "Brand lockup: mark (both states) + expanded-only wordmark inside one flex container in the SidebarHeader."

requirements-completed: [REBRAND-01, REBRAND-03, BRAND-01]

# Metrics
duration: ~20min
completed: 2026-07-01
---

# Phase 20 Plan 03: Sidebar Kamacu Brand Lockup + Copy Rename Summary

**Sidebar header now renders the KamacuMark ember spark (both expanded header and collapsed rail) plus a title-case "Kamacu" wordmark, and every remaining user-facing "Kangent" string in the frontend is now "Kamacu" — while all `kangent.*` localStorage keys and the `~/.kangent` path token are left untouched.**

## Performance

- **Duration:** ~20 min
- **Completed:** 2026-07-01T11:45:05Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Reworked `SidebarHeader` into the Kamacu brand lockup: `KamacuMark` (imported from `@/components/brand/KamacuMark`) renders in BOTH states — sitting at the top of the ~3rem collapsed icon rail above the project avatars (D-08) — plus an expanded-only "Kamacu" title-case wordmark (D-07/D-09). The old lowercase `kangent` brand text is gone; the sidebar toggle (`SidebarTrigger` + `Tooltip`) is preserved.
- Renamed the last user-facing "Kangent" copy across 5 files: empty-board prompt (`App.tsx`), project-delete confirmation (`ProjectMenu.tsx`), GitHub integration help (`SettingsPage.tsx`), and comment prose in `types.ts` and `sessions.ts`.
- Held the D-14 keep-out boundary: the three `kangent.*`/`kangent:*` localStorage keys and the `~/.kangent/repos/` path token remain verbatim (verified post-change).

## Task Commits

Each task was committed atomically:

1. **Task 1: Rework the sidebar header into the Kamacu brand lockup** - `b136add` (feat)
2. **Task 2: Rename remaining user-facing "Kangent" copy** - `cbc3aad` (feat)

**Plan metadata:** committed with this SUMMARY (docs).

## Files Created/Modified
- `web/src/components/sidebar/ProjectSidebar.tsx` - Imports/renders `KamacuMark` in both states; expanded-only "Kamacu" wordmark; old lowercase brand text removed; toggle preserved.
- `web/src/App.tsx` - Empty-board copy "Point Kamacu at a local git repository…".
- `web/src/components/sidebar/ProjectMenu.tsx` - Delete-confirmation copy "…removed from Kamacu…".
- `web/src/pages/SettingsPage.tsx` - `GITHUB_INTEGRATION_HELP` "…across Kamacu…".
- `web/src/api/types.ts` - Comment word renamed to "Kamacu cloned" (line 18); `~/.kangent/repos/` path token on line 19 left verbatim.
- `web/src/api/sessions.ts` - Comment prose "Kamacu-initiated stop" and "outlived a Kamacu restart".

## Decisions Made
- Kept the `SidebarTrigger` + `Tooltip` block intact and preserved the header's flex/justify classes, wrapping the mark and wordmark in a single flex span so the mark stays centered/visible when the sidebar is collapsed and only the wordmark hides.
- In `types.ts`, renamed only the product word on line 18 and deliberately left the `~/.kangent/repos/` path token on line 19 (Phase 21 keep-out; it documents a real runtime path).

## Deviations from Plan

None - plan executed exactly as written. No auto-fixes (Rules 1-4) were needed; both tasks matched the codebase as specified.

## Issues Encountered

- **Verification environment (build/lint tooling absent in worktree):** the worktree had no `node_modules` (worktrees carry isolated working trees). The main checkout had the identical, already-installed dependency tree (`package-lock.json` byte-identical). Resolved by symlinking the worktree's `node_modules` to the main checkout's verified deps — a local verification convenience only, not committed (`node_modules` is gitignored) and adding no new packages. `npm run build` then passed (exit 0).
- **Pre-existing frontend ESLint failures (out of scope):** `cd web && npm run lint` exits 1 with 20 `react-hooks/set-state-in-effect`-class errors, ALL in files this plan does not touch (Board.tsx, TaskPage.tsx, ui/sidebar.tsx, use-mobile.ts, etc.). The six files edited by this plan lint clean (`npx eslint <those files>` exits 0), so this plan introduces zero new lint errors. Per the executor SCOPE BOUNDARY these pre-existing errors are out of scope; logged to `.planning/phases/20-kamacu-rebrand-brand/deferred-items.md` (not fixed). The plan's "npm run lint exits 0" acceptance criterion cannot be met at the whole-repo level due to these pre-existing errors, but the intent (no new lint regressions from this plan) holds.
- **Build artifact:** `vite build` regenerated the tracked `web/dist/index.html`; reverted after each build so task commits contain source only.

## Known Stubs

None - no stubs, placeholder text, or unwired data sources introduced.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- The in-app rebrand is visually complete: sidebar lockup renders and no user-facing "Kangent" copy remains. Ready for the 20-04 human-verify checkpoint (expanded header shows mark + "Kamacu"; collapsed rail shows the mark at the top above the avatars) and the post-rebrand README screenshot capture (D-12).
- Keep-out boundary intact for Phase 21 (localStorage keys + `~/.kangent` paths untouched).

---
*Phase: 20-kamacu-rebrand-brand*
*Completed: 2026-07-01*

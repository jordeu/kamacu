---
phase: 06-settings-polish
plan: 3
subsystem: ui
tags: [settings, react, tanstack-query, shadcn, select, label, react-router]

# Dependency graph
requires:
  - phase: 06-settings-polish
    provides: "06-01: GET /api/settings + PUT /api/settings/{key} REST surface with canonical validation copy and shell options array"
  - phase: 01-foundation
    provides: AppLayout route shell, ApiError client pipe, board failure/skeleton patterns, shadcn button/input/tooltip/skeleton/sidebar
provides:
  - "/settings full-page route under AppLayout (sidebar visible) reached via the sidebar-footer gear with active state (SET-01)"
  - "SettingsField per-field commit state machine: blur/Enter commit, Esc revert, 2s muted Saved flash, Reset to default, verbatim 4xx inline errors, fixed network/5xx copy, full field isolation (SET-04)"
  - "SettingsPage with 4 groups (Agent/Worktrees/Bash tabs/Branches), verbatim UI-SPEC copy, API-driven shell options (SHELL-02 seam), skeleton + load-failure states"
  - "put<T> client helper riding the existing ApiError {\"error\"} extraction pipe"
  - "useSettings/useSaveSetting typed hooks (cache replace on success, no optimistic update)"
  - "UI-01: task-view header/meta block spans full page width; Description prose keeps its 860px island"
affects: [06-04, settings, task-page]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-field save: draft held locally, mutation per field, cache setQueryData on success — a 400 never touches the cache or any other field"
    - "withMono runtime wrapper: contract strings stay single grep-able template literals while flags/tokens render font-mono"
    - "Gear active state reuses SidebarMenuButton's selected-item classes (bg-sidebar-accent text-sidebar-accent-foreground) via useLocation"

key-files:
  created:
    - web/src/components/ui/select.tsx
    - web/src/components/ui/label.tsx
    - web/src/api/settings.ts
    - web/src/components/settings/SettingsField.tsx
    - web/src/pages/SettingsPage.tsx
  modified:
    - web/src/api/client.ts
    - web/src/App.tsx
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "Select-option mapping lives inside SettingsField (entry.options per the plan's prop spec); SettingsPage documents the SHELL-02 seam with an entry.options comment"
  - "withMono runtime wrapper reconciles 'flag span in mono' with the grep-able single-template-literal convention"
  - "4xx threshold for verbatim server copy is ApiError && status < 500; everything else gets the fixed network/5xx copy"

patterns-established:
  - "Per-field commit: skip request when draft === saved value; in-flight duplicate guard via save.isPending && save.variables === draft"
  - "Saved flash timer kept in a ref, cleared on unmount and resave"

requirements-completed: [SET-01, SET-04, AGENT-01, AGENT-02, WT-01, SHELL-01, BRANCH-01, UI-01]

# Metrics
duration: 8min
completed: 2026-06-11
---

# Phase 6 Plan 3: Settings Frontend Summary

**Full /settings page with per-field commit (blur/Enter, Esc revert, 2s Saved flash, Reset to default, verbatim inline server errors), sidebar gear with active state, API-driven shell select, and the UI-01 full-width task header**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-11T15:16:00Z
- **Completed:** 2026-06-11T15:23:32Z
- **Tasks:** 3
- **Files modified:** 9

## Accomplishments

- Settings UI complete against the 06-01 REST surface: four groups (Agent / Worktrees / Bash tabs / Branches) with every user-facing string verified verbatim against 06-UI-SPEC.md
- SET-04 by construction: each field is an independent mutation with a local draft — a validation failure renders the server's canonical message inline under that field only, the draft is never cleared, and no other field's state is touched
- Shell dropdown options map from the API payload's `entry.options` (SHELL-02 seam) — no frontend constant, no hardcoded `bash`
- Sidebar gear (28px ghost icon, tooltip + aria-label `Settings`) navigates to the full-page `/settings` route with the selected-sidebar-item active treatment (SET-01)
- UI-01: task-view header/meta wrapper converted from `max-w-[860px]` to `w-full` — ellipsis trigger now sits flush at the right padding edge where the tab strip ends; Description prose island and loading/error states untouched (3 of the original 4 `max-w-[860px]` remain, as planned)
- Zero new npm dependencies — both shadcn blocks compile against the existing `radix-ui` umbrella package

## Task Commits

Each task was committed atomically:

1. **Task 1: shadcn select + label blocks, client put helper, settings hooks** - `561bec4` (feat)
2. **Task 2: SettingsField + SettingsPage + /settings route** - `b4777b3` (feat)
3. **Task 3: Sidebar gear (SET-01) + UI-01 task-header width fix** - `dc20785` (feat)

## Files Created/Modified

- `web/src/components/ui/select.tsx` - shadcn select block (radix-nova, CLI-generated, untouched)
- `web/src/components/ui/label.tsx` - shadcn label block (CLI-generated, untouched)
- `web/src/api/client.ts` - `put<T>` helper mirroring `patch` exactly (ApiError pipe preserved)
- `web/src/api/settings.ts` - `SettingEntry`/`Settings` types, `useSettings`, `useSaveSetting` (cache replace on success, no optimistic update)
- `web/src/components/settings/SettingsField.tsx` - per-field commit state machine: draft, blur/Enter commit, Esc revert with focus kept, 2s Saved flash, Reset to default, inline error replacing help line
- `web/src/pages/SettingsPage.tsx` - heading + SET-03 sub-line, 4 groups with verbatim contract copy, withMono helper, 4-block skeleton, centered load-failure with Retry loading
- `web/src/App.tsx` - `/settings` route added inside the AppLayout route
- `web/src/components/sidebar/ProjectSidebar.tsx` - SidebarFooter one row: Add project (flex-1) + gear Link with tooltip/aria-label/active state
- `web/src/pages/TaskPage.tsx` - header wrapper `max-w-[860px]` → `w-full` (UI-01), comment updated

## Decisions Made

- **Select mapping placement:** the plan's prop spec gives SettingsField only `entry` (no options prop), so the `entry.options?.map` rendering lives inside SettingsField; SettingsPage carries an `entry.options` SHELL-02 comment at the shell field so the seam is documented where the field is declared (and the plan's SettingsPage grep criterion passes)
- **withMono helper:** the UI-SPEC requires flags/tokens/examples in mono while Phase 3's convention requires contract strings as single grep-able template literals — a small runtime wrapper splits the literal at render time, satisfying both without duplicating copy
- **Error classification:** verbatim server copy for `ApiError` with status < 500; `Couldn't save. Try again.` for network errors (fetch TypeError) and 5xx
- **Double-send guard:** Enter-then-blur on the same draft while the save is in flight is suppressed via `save.isPending && save.variables === draft`

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Known Stubs

None — all four fields are wired to the live 06-01 GET/PUT API; no placeholder data or empty-value scaffolding.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Settings UI is live against the already-shipped 06-01 API — the page is fully functional now; parallel plan 06-02 wires the stored values into the spawn/worktree/shell/branch call sites
- 06-04 (verification/UAT) can exercise: gear navigation + active state, per-field save/revert/reset/error isolation, shell dropdown from API options, UI-01 header width

---
*Phase: 06-settings-polish*
*Completed: 2026-06-11*

## Self-Check: PASSED

All 5 created files verified on disk; all 3 task commits verified in git log.

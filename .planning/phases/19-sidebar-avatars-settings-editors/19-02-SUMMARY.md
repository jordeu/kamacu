---
phase: 19-sidebar-avatars-settings-editors
plan: 02
subsystem: ui
tags: [react, tailwind, shadcn, sidebar, avatar, rail, collapsible-icon, palette, migration]

# Dependency graph
requires:
  - phase: 19-sidebar-avatars-settings-editors
    provides: "19-01 <ProjectAvatar> primitive + PROJECT_PALETTE TS const"
  - phase: 18-project-icon-data-foundation
    provides: "internal/api/icons.go projectPalette (source of truth) + validated icon_letters/icon_color on the wire"
provides:
  - "Collapsible icon RAIL: <Sidebar collapsible=icon> renders a vertical rail of per-project <ProjectAvatar>s when collapsed (active highlight, side=right name tooltip, static amber waiting dot)"
  - "Inline avatar beside the project name in the expanded rows (existing name + amber count chip preserved)"
  - "Floating CollapsedSidebarTrigger retired + <main> pl-9 dropped (AppLayout) — the rail occupies layout space; header SidebarTrigger is the re-expand entry point"
  - "Muted project avatar palette (Go source + TS mirror + remap migration 00010) — UAT-driven"
affects: [19-03-settings-editors, future-sidebar-work]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "shadcn collapsible=icon rail styled via group-data-[collapsible=icon]:* utilities; icon-mode menu button enlarged to a 40px rounded-md filled-row hit target (size-8->size-10 via same-group tailwind-merge override) so the active bg-sidebar-accent reads as a filled-row highlight matching the expanded selection"
    - "Collapsed active state is carried by the SidebarMenuButton background (isActive -> data-active:bg-sidebar-accent), NOT a ring on the avatar — collapsed and expanded selection look consistent"
    - "Palette source-of-truth lives in Go (icons.go); the TS mirror (palette.ts) is kept byte-for-byte in sync; a data migration remaps existing rows on palette change"

key-files:
  created:
    - internal/store/migrations/00010_muted_palette.sql
  modified:
    - web/src/components/sidebar/ProjectSidebar.tsx
    - web/src/components/layout/AppLayout.tsx
    - web/src/components/ui/ProjectAvatar.tsx
    - web/src/lib/palette.ts
    - internal/api/icons.go
    - internal/api/icons_test.go

key-decisions:
  - "UAT reversal of D-04 avatar shape: rounded-md (square) -> rounded-full (circle) — the square read as too boxy"
  - "UAT reversal of D-06 active indicator: drop the rail-only ring on the avatar; the active/selected project is now highlighted by the rail SidebarMenuButton's filled rounded-md bg-sidebar-accent (the SAME treatment as the expanded selection). The earlier dim sidebar-ring was invisible and a near-white ring was ugly/inconsistent. The `active` prop was removed from <ProjectAvatar>."
  - "UAT reversal of the palette (amends D-01 'Tailwind-600' / D-13 specific hexes): replaced the 9 bright hues with 9 muted, desaturated hues (still >=4.5:1 white-text contrast) so avatars are a subtle difference on the dark rail, not a bright highlight. Changed BOTH the Go source (icons.go) and the TS mirror (palette.ts); added migration 00010 to remap existing projects' icon_color by palette position."
  - "Rail avatar size reduced size-8 -> size-6 (24px) for breathing room in the 3rem rail"
  - "The 'kangent' brand title is hidden in icon mode (group-data-[collapsible=icon]:hidden) — it was overflowing the rail and overlapping the main view"

patterns-established:
  - "Icon-rail item = a 40px rounded-md SidebarMenuButton (size-10!, p-0!, justify-center) inside a px-1 menu, with a centered size-6 ProjectAvatar; active/hover = bg-sidebar-accent filled row"
  - "Changing the curated palette is a 3-part change: Go projectPalette + TS PROJECT_PALETTE (byte-for-byte) + a positional remap migration for existing rows"

requirements-completed: [ICON-05, ICON-06, ICON-07, ICON-08, ICON-09, ICON-10]

# Metrics
duration: ~35min (incl. 3 UAT revision rounds)
completed: 2026-06-19
---

# Phase 19 Plan 02: Collapsible Avatar Rail Summary

**The collapsed sidebar is now a 3rem icon rail of per-project circular monogram avatars (filled-row active highlight, side=right name tooltip, static amber waiting dot); the same avatar appears inline beside the name when expanded; the floating re-expand trigger is retired; and the avatar palette was re-muted (Go source + TS mirror + remap migration) per UAT.**

## Performance

- **Duration:** ~35 min (2 auto tasks + a blocking human-verify gate that drove 3 visual-revision rounds)
- **Completed:** 2026-06-19
- **Tasks:** 3 (2 auto, 1 human-verify checkpoint — approved)
- **Files modified:** 6 (1 created)

## Accomplishments
- `ProjectSidebar.tsx` switched to `<Sidebar collapsible="icon">`: the collapsed state renders a vertical rail of `<ProjectAvatar>`s instead of sliding off-screen (ICON-05). Clicking a rail avatar switches project without expanding (ICON-06); the open project is highlighted (ICON-07); hover shows a `side="right"` tooltip with the full name (ICON-08); a project with ≥1 waiting agent shows a static amber dot on its avatar (ICON-09).
- Expanded rows show an inline avatar before the unchanged name + the existing amber count chip (ICON-10); the footer Add/Settings is hidden in icon mode.
- `AppLayout.tsx` retired the floating `CollapsedSidebarTrigger` and dropped `<main>`'s `pl-9`; the controlled-open + `localStorage["kangent.sidebar"]` persistence and `TooltipProvider delayDuration={0}` are unchanged.
- **UAT refinements** (post-checkpoint, see Deviations): circular avatars, muted palette, smaller rail avatars, and a filled-row active highlight that matches the expanded selection.

## Task Commits

1. **Task 1: Collapsed avatar rail + inline avatars (ProjectSidebar)** - `8d192ed` (feat)
2. **Task 2: Retire floating CollapsedSidebarTrigger + drop main pl-9 (AppLayout)** - `6e890ee` (feat)
3. **Task 3: Human visual-verify (ICON-05..10)** - approved after the UAT revisions below.

**UAT revision commits (post-checkpoint):**
- `078ed33` (fix) — muted project avatar palette: Go `projectPalette` + TS `PROJECT_PALETTE` + remap migration `00010` + updated `icons_test` fixtures.
- `5ee4d42` (fix) — circular avatars (`rounded-full`), visible active ring (initial attempt), rail vertical spacing, `kangent` brand hidden in icon mode.
- `98da454` (fix) — rail avatar `size-7`→`size-6`; replaced the white avatar ring with the SidebarMenuButton's accent-bg active highlight.
- `c5d57c4` (fix) — enlarged the icon-mode button to a 40px `rounded-md` filled row so the active `bg-sidebar-accent` is clearly visible (matches the expanded selection).

## Files Created/Modified
- `web/src/components/sidebar/ProjectSidebar.tsx` - `collapsible="icon"` rail; rail + inline `<ProjectAvatar>` call sites; 40px rounded-md icon-mode button (filled-row active/hover accent); footer + brand hidden in icon mode; reuses the existing `waitingByProject` map.
- `web/src/components/layout/AppLayout.tsx` - floating trigger + `pl-9` removed; unused imports pruned; open-state persistence untouched.
- `web/src/components/ui/ProjectAvatar.tsx` - (19-01 file) shape `rounded-md`→`rounded-full`; rail `size-8`→`size-6`; removed the `active` prop/ring (active now lives on the rail button).
- `web/src/lib/palette.ts` - `PROJECT_PALETTE` re-muted (9 desaturated hexes), still byte-for-byte with Go.
- `internal/api/icons.go` - `projectPalette` re-muted (source of truth) + updated doc.
- `internal/api/icons_test.go` - `validateIconColor` fixtures updated to new palette members.
- `internal/store/migrations/00010_muted_palette.sql` - positional remap of existing `icon_color` from the old bright hues to the new muted hues (Up + Down).

## Decisions Made
See `key-decisions` frontmatter — all four were driven by the human-verify gate (shape→circle, active→filled-row accent, bright→muted palette, brand hidden in icon mode). The palette change deliberately touched a Phase 18 artifact (`internal/api/icons.go`) because the Go slice is the source of truth and a frontend-only change would not affect stored/assigned colors.

## Deviations from Plan

The 2 auto tasks matched the plan exactly. The blocking human-verify checkpoint (Task 3) surfaced 5 runtime issues across 3 rounds; all were fixed and re-verified before approval:

1. **`kangent` brand overlapped the main view when collapsed** — the header title was not hidden in icon mode. Fixed: `group-data-[collapsible=icon]:hidden` on the title, header trigger centered when collapsed. (`5ee4d42`)
2. **Rail avatars too big / touching** — the `size-8` avatar overflowed the padded icon-mode button. Fixed: rail `size-6`, icon-mode button `p-0` + rail vertical gap. (`5ee4d42`, `98da454`)
3. **Avatars too square** — UAT wanted circular. Fixed: `rounded-md`→`rounded-full`. (`5ee4d42`)
4. **Palette too bright for the dark theme** — UAT wanted muted/subtle. Fixed: 9 muted hexes (Go + TS) + remap migration `00010`, all contrast-verified ≥4.5:1 white text. (`078ed33`)
5. **Active project not visibly highlighted when collapsed** — the dim `sidebar-ring` was invisible; a near-white ring was then judged ugly; a thin circular accent halo was too subtle. Final: a 40px rounded-md filled-row `bg-sidebar-accent` highlight matching the expanded selection. (`5ee4d42`→`98da454`→`c5d57c4`)

**Impact:** No scope change — same requirements (ICON-05..10). The visual contract from the UI-SPEC was amended by the user during UAT (D-04 shape, D-06 active treatment, palette hues); recorded above as decision reversals.

## Issues Encountered
- The full-repo `npm run lint` still reports ~20 pre-existing `react-hooks` errors (documented tech debt) — none in the changed files; plan-scoped `npx eslint` is clean.

## Known Stubs
None.

## User Setup Required
- **Backend restart required** for the muted colors to land on EXISTING projects: migration `00010` runs at startup and remaps stored `icon_color` values. New projects pick muted colors automatically.

## Next Phase Readiness
- The rail + expanded surfaces consume `project.icon_letters`/`icon_color`, so edits from Plan 03 (settings) propagate here on save.
- Build (`cd web && npm run build`) + plan-scoped eslint clean; Go `build`/`vet`/`test` green.

## Self-Check: PASSED

- FOUND: web/src/components/sidebar/ProjectSidebar.tsx (`collapsible="icon"`, `size="rail"`, `size="inline"`)
- FOUND: web/src/components/layout/AppLayout.tsx (no `CollapsedSidebarTrigger`, no `pl-9`)
- FOUND: internal/store/migrations/00010_muted_palette.sql
- FOUND commit: 8d192ed (Task 1), 6e890ee (Task 2)
- FOUND commits: 078ed33, 5ee4d42, 98da454, c5d57c4 (UAT fixes)

---
*Phase: 19-sidebar-avatars-settings-editors*
*Completed: 2026-06-19*

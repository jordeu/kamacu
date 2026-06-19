---
phase: 19-sidebar-avatars-settings-editors
plan: 03
subsystem: ui
tags: [react, tailwind, shadcn, settings, dialog, avatar, palette, swatch, conditional-patch]

# Dependency graph
requires:
  - phase: 19-sidebar-avatars-settings-editors
    provides: "19-01 <ProjectAvatar> primitive + PROJECT_PALETTE TS const (re-muted in 19-02 UAT)"
  - phase: 18-project-icon-data-foundation
    provides: "useUpdateProjectSettings conditional PATCH already carrying icon_letters/icon_color; server validateIconLetters/validateIconColor"
provides:
  - "Project settings letter + color editors: live <ProjectAvatar> preview, Initials input (client-normalized to <=2 uppercase alphanumerics), 9-swatch PROJECT_PALETTE color grid (current color ring+check-marked), wired into the existing conditional PATCH (icon_letters/icon_color sent only when changed)"
affects: [future-settings-work]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Client-side normalization mirrors the Go server rule (validateIconLetters): /[^\\p{L}\\p{N}]/ strip + toUpperCase + slice(0,2); server stays the enforcer"
    - "Conditional PATCH: a field is added to mutateAsync only when changed (icon_letters/icon_color: changed ? value : undefined), same omitted-=-untouched shape as github_repo"
    - "Swatch grid iterates PROJECT_PALETTE (no hardcoded hex); selected swatch = ring-2 ring-sidebar-ring + centered white lucide Check + aria-pressed"

key-files:
  created: []
  modified:
    - web/src/components/sidebar/ProjectSettingsDialog.tsx

key-decisions:
  - "Letters normalization is UX-only and mirrors the server validateIconLetters; Save is disabled when the trimmed letters draft is empty (>=1 required, D-10)"
  - "Color grid renders the curated PROJECT_PALETTE only (no free-form hex input — ICON-FUT-03 deferred); selecting a swatch updates local draft + the live preview, and joins the conditional PATCH on Save (D-11/D-12)"
  - "The swatch grid + live preview automatically reflect the 19-02 UAT muted palette — no change needed here (PROJECT_PALETTE is the single source) beyond consuming the updated const"

patterns-established:
  - "Settings field editors seed local draft state from project.* in the existing on-open reset block (no useEffect) and flow through the single existing conditional PATCH"

requirements-completed: [ICON-11, ICON-12]

# Metrics
duration: ~10min
completed: 2026-06-19
---

# Phase 19 Plan 03: Settings Letter & Color Editors Summary

**Project settings now has a live `<ProjectAvatar>` preview, an "Initials" input (client-normalized to ≤2 uppercase alphanumerics, mirroring the server rule), and a 9-swatch `PROJECT_PALETTE` color grid with the current color ring+check-marked — all wired into the existing conditional PATCH so `icon_letters`/`icon_color` are sent only when changed and saved edits propagate to the rail + expanded sidebar.**

## Performance

- **Duration:** ~10 min (2 auto tasks + a blocking human-verify gate)
- **Completed:** 2026-06-19
- **Tasks:** 3 (2 auto, 1 human-verify checkpoint — approved)
- **Files modified:** 1

## Accomplishments
- Live avatar preview at the top of the dialog, driven by local `letters`/`color` draft state (D-09).
- "Initials" `<Input maxLength={2}>` with a change handler that strips non-alphanumerics (`\p{L}\p{N}`, Unicode-aware), uppercases, and caps at 2 — mirroring `validateIconLetters`; Save is disabled when the trimmed draft is empty (ICON-11/D-10).
- 9-swatch color grid iterating `PROJECT_PALETTE` (`size-7` buttons, inline `backgroundColor`, `aria-label`, focus ring); the swatch matching the draft color shows `ring-2 ring-sidebar-ring` + a centered white lucide `Check` + `aria-pressed` (ICON-12/D-11).
- `icon_letters`/`icon_color` join the existing `useUpdateProjectSettings` conditional PATCH (each sent only when changed); 2xx closes, a 400 surfaces inline like the repo error (D-12). Saved edits appear in the collapsed rail + expanded sidebar (Plan 02) without a manual refresh.

## Task Commits

1. **Task 1: Live preview + Initials input + draft state/reset + conditional letters save** - `27f900b` (feat)
2. **Task 2: 9-swatch PROJECT_PALETTE color grid + conditional color save** - `e122f75` (feat)
3. **Task 3: Human visual-verify (ICON-11/12 + propagation)** - approved (verified alongside the Plan 02 rail in the same UAT pass).

## Files Created/Modified
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` - added the live preview, Initials field (client-mirrored normalization), 9-swatch color grid (Check-marked current), and extended the existing conditional PATCH with `icon_letters`/`icon_color`. No backend change (Phase 18 shipped the validated mutation).

## Decisions Made
See `key-decisions` frontmatter. Notably: no free-form hex input (ICON-FUT-03 deferred); the grid is palette-only and the server re-validates palette membership. The editors automatically render the 19-02 UAT muted palette because both the swatch grid and the live preview consume the single `PROJECT_PALETTE` source.

## Deviations from Plan
None for this plan's own tasks — the 2 auto tasks and the human-verify gate matched the plan. The only cross-plan effect is the muted palette (committed under 19-02's UAT, `078ed33`): the swatch grid + preview now show muted hues, which is the intended consistent result.

## Issues Encountered
- The full-repo `npm run lint` still reports the ~20 pre-existing `react-hooks` errors (documented tech debt) — none in this file; plan-scoped `npx eslint` is clean.

## Known Stubs
None.

## User Setup Required
None for this plan (the muted-palette backend restart note lives in 19-02; the editors themselves need no setup).

## Next Phase Readiness
- Closes the v1.7 ICON loop: users can now edit both letters and color, and the edits render in every avatar surface (rail, expanded row, preview).
- Build (`cd web && npm run build`) + plan-scoped eslint clean.

## Self-Check: PASSED

- FOUND: web/src/components/sidebar/ProjectSettingsDialog.tsx (`ProjectAvatar`, `maxLength={2}`, `PROJECT_PALETTE`, `icon_letters`, `icon_color`, `Check`)
- FOUND commit: 27f900b (Task 1), e122f75 (Task 2)

---
*Phase: 19-sidebar-avatars-settings-editors*
*Completed: 2026-06-19*

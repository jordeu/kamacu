---
phase: 19-sidebar-avatars-settings-editors
plan: 01
subsystem: ui
tags: [react, tailwind, shadcn, avatar, palette, monogram, typescript]

# Dependency graph
requires:
  - phase: 18-project-icon-data-foundation
    provides: "internal/api/icons.go projectPalette (9 Tailwind-600 hexes, source of truth); validated icon_letters/icon_color on the wire (web/src/api/types.ts, mutations.ts)"
provides:
  - "PROJECT_PALETTE TS const (web/src/lib/palette.ts) — 9 lowercase hexes mirroring the Go projectPalette byte-for-byte, as const"
  - "<ProjectAvatar> shared monogram primitive (web/src/components/ui/ProjectAvatar.tsx) — rounded-md white font-medium letters on inline icon_color, rail|inline sizes, rail-only active ring + static amber waiting dot"
affects: [19-02-sidebar-rail, 19-03-settings-editors]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Avatar primitive follows the shadcn skeleton.tsx/input.tsx shape (data-slot, cn(base, className) merge, single named export) but STATIC (no animate-pulse)"
    - "Palette hex applied ONLY via inline style backgroundColor (never a Tailwind bg-* class) since the hue is a runtime value"
    - "lib/*.ts single-export const with a top provenance doc comment (time.ts convention) mirroring a Go source of truth verbatim, no endpoint"

key-files:
  created:
    - web/src/lib/palette.ts
    - web/src/components/ui/ProjectAvatar.tsx
  modified: []

key-decisions:
  - "ProjectAvatar lives in web/src/components/ui/ (with the shadcn primitives), palette in web/src/lib/ — the recommended placements from D-04 note / UI-SPEC discretion"
  - "Glyph weight is font-medium (UI-SPEC Typography binding), not the CONTEXT D-04 word 'bold' — the approved spec supersedes"
  - "active/waiting visuals are rail-only; the component ignores them at the inline size (D-06/D-07)"

patterns-established:
  - "ProjectAvatar is the ONLY place a palette hex is applied as a background (D-04) — the ICON-10 reuse point for rail, expanded row, and settings preview"
  - "PROJECT_PALETTE is the single TS swatch source, kept byte-for-byte in sync with internal/api/icons.go (no palette endpoint, Phase 18 D-02)"

requirements-completed: [ICON-07, ICON-09, ICON-10, ICON-12]

# Metrics
duration: 3min
completed: 2026-06-19
---

# Phase 19 Plan 01: Sidebar Avatar Foundation Summary

**Shared `<ProjectAvatar>` rounded-md monogram primitive (rail|inline sizes, rail-only active ring + static amber waiting dot) plus the `PROJECT_PALETTE` TS const mirroring the Go projectPalette byte-for-byte — the Wave-1 dependency root every Phase 19 surface consumes.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-06-19T06:38:57Z
- **Completed:** 2026-06-19T06:42:14Z
- **Tasks:** 2
- **Files modified:** 2 (both created)

## Accomplishments
- `PROJECT_PALETTE` — 9 lowercase hexes (`#dc2626`…`#db2777`), `as const`, in the locked Go order, with a provenance doc comment; the node byte-for-byte assertion against `internal/api/icons.go` passes.
- `<ProjectAvatar>` — white `font-medium` monogram on an inline `backgroundColor: icon_color`, two sizes (`rail` size-8/text-sm, `inline` size-5/text-[10px]), a rail-only `ring-2 ring-sidebar-ring` active marker, and a rail-only STATIC `size-2 bg-amber-400` waiting dot with an `aria-label`.
- No call sites wired (by design) — the rail (Plan 02), expanded row (Plan 02), and settings preview/swatch grid (Plan 03) consume these.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create PROJECT_PALETTE const (palette.ts)** - `b5c9b53` (feat)
2. **Task 2: Create <ProjectAvatar> shared monogram primitive** - `373c3fb` (feat)

**Plan metadata:** (docs commit — this SUMMARY + state files)

## Files Created/Modified
- `web/src/lib/palette.ts` - `PROJECT_PALETTE` const: 9 lowercase hexes mirroring `internal/api/icons.go` `projectPalette` verbatim, `as const`, the swatch-grid source and only legal avatar background (D-13).
- `web/src/components/ui/ProjectAvatar.tsx` - `ProjectAvatar` primitive: rounded-md white-on-icon_color monogram, rail|inline sizes, rail-only active ring + static amber waiting dot with aria-label (D-04/05/06/07).

## Decisions Made
- Placed `ProjectAvatar` under `web/src/components/ui/` (alongside shadcn primitives) and `palette.ts` under `web/src/lib/` — the UI-SPEC/D-04-note recommended defaults.
- Used `font-medium` for the glyph (UI-SPEC Typography is binding), not `font-bold` (the CONTEXT D-04 word "bold" is explicitly superseded by the approved spec).
- `active`/`waiting` are treated as rail-only; the component drops them at the `inline` size so callers cannot accidentally double-signal in the expanded row (D-06/D-07/D-08).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed the literal `#ffffff` hex from palette.ts doc comment**
- **Found during:** Task 1 (palette.ts)
- **Issue:** The Task 1 `<verify>` gate matches ALL `#[0-9a-f]{6}` tokens in the file and asserts they equal the 9 palette hexes in order. The doc comment's literal `#ffffff` ("fixed white text") prepended a 10th match, failing the assertion (`PALETTE MISMATCH`).
- **Fix:** Reworded the comment to say "fixed white" without the literal hex; the 9 palette hexes are now the only `#......` tokens in the file. Semantics unchanged.
- **Files modified:** web/src/lib/palette.ts
- **Verification:** `node` assertion prints `palette OK`; `tsc -b` + `npm run build` clean.
- **Committed in:** b5c9b53 (Task 1 commit)

**2. [Rule 3 - Blocking] Removed the literal `animate-pulse` token from ProjectAvatar.tsx doc comment**
- **Found during:** Task 2 (ProjectAvatar.tsx)
- **Issue:** The Task 2 static-gate (`grep -v '^[[:space:]]*//' | grep -q animate-pulse`) only strips single-line `//` comments, not the `*`-prefixed lines of a `/** */` block comment. The doc comment's mention of "never `animate-pulse`" leaked through and tripped the gate even though the component is genuinely static.
- **Fix:** Reworded the comment to "STATIC (no pulse animation)" — no `animate-pulse` token anywhere in the file. The component never used the class; only the prose mentioned it.
- **Files modified:** web/src/components/ui/ProjectAvatar.tsx
- **Verification:** static gate prints `static OK`; `npm run build` + `npx eslint` on both new files clean; all UI-SPEC tokens (size-8/size-5/text-sm/text-[10px]/size-2/bg-amber-400/ring-2 ring-sidebar-ring/text-white/font-medium) present, no `font-bold`.
- **Committed in:** 373c3fb (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (2 blocking, both Rule 3)
**Impact on plan:** Both fixes were comment-only rewordings to satisfy verify-gate regexes that cannot distinguish a token mention in a block comment from real usage. Zero behavioral change; the produced code matches the plan and UI-SPEC exactly. No scope creep.

## Issues Encountered
- The full-repo `npm run lint` reports 20 pre-existing `react-hooks` errors (documented tech debt in PROJECT.md/STATE.md) — out of scope per the deviation scope boundary; neither new file appears in the lint output (the plan-scoped `npx eslint` on the two files is clean).

## Known Stubs
None — both artifacts are complete primitives. No call sites are wired in this plan by design (consumers are Plans 02/03, per the objective); this is not a stub but the explicit Wave-1 dependency-root boundary.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `<ProjectAvatar>` and `PROJECT_PALETTE` are the Wave-1 root for Phase 19: Plan 02 (sidebar rail + expanded row) and Plan 03 (settings letters/color editors + live preview) can now import both.
- The avatar's prop contract (size, letters, color, active, waiting, waitingLabel, className) matches the plan's `<interfaces>` block exactly, so downstream plans implement against a stable shape.
- No blockers.

## Self-Check: PASSED

- FOUND: web/src/lib/palette.ts
- FOUND: web/src/components/ui/ProjectAvatar.tsx
- FOUND: .planning/phases/19-sidebar-avatars-settings-editors/19-01-SUMMARY.md
- FOUND commit: b5c9b53 (Task 1)
- FOUND commit: 373c3fb (Task 2)

---
*Phase: 19-sidebar-avatars-settings-editors*
*Completed: 2026-06-19*

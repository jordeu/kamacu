---
phase: 20-kamacu-rebrand-brand
plan: 02
subsystem: ui
tags: [react, svg, branding, favicon, vite, tailwind]

# Dependency graph
requires: []
provides:
  - "KamacuMark: reusable inline-SVG ember-spark brand mark React component (named export @/components/brand/KamacuMark)"
  - "web/public/favicon.svg: spark-derived favicon served at site root, copied into dist/ by Vite"
  - "web/index.html: browser tab titled Kamacu with a rel=icon link to /favicon.svg"
affects: [20-03-sidebar-lockup, phase-20-readme-screenshot]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Inline-SVG brand mark component following the ProjectAvatar idiom (named export, cn()/...props, role=img + aria-label, default size-* overridable via className)"
    - "Per-instance SVG gradient ids via React.useId() (colons stripped) to avoid def collisions when multiple marks share the DOM"
    - "Favicon is the documented exception to the no-.svg-files rule: a hand-authored web/public/ asset, root-served and embedded via the existing all:dist embed"

key-files:
  created:
    - web/src/components/brand/KamacuMark.tsx
    - web/public/favicon.svg
  modified:
    - web/index.html

key-decisions:
  - "Ember spark = a radial glow halo + an orange->amber gradient flame body + rising ember particles, deliberately multi-tone/shaped so it is unmistakable from the flat bg-amber-400 waiting dot (D-04)"
  - "Favicon draws the flame on a rounded dark ember badge so it stays legible on BOTH light and dark browser-tab chrome (D-06)"
  - "Shipped mark is STATIC (no animation) — animated logo variants are deferred (BRAND-FUT-01)"
  - "Did NOT commit the rebuilt web/dist/ (tracked placeholder index.html reverted) — the embedded SPA is rebuilt post-merge, and dist assets are gitignored"

patterns-established:
  - "Brand components live under web/src/components/brand/"

requirements-completed: [REBRAND-01, BRAND-01, BRAND-02]

# Metrics
duration: 7min
completed: 2026-07-01
---

# Phase 20 Plan 02: Kamacu Brand Assets Summary

**An abstract multi-tone ember-spark `KamacuMark` inline-SVG component, a spark-derived rounded-badge `favicon.svg`, and a `web/index.html` that titles the tab "Kamacu" and links the favicon.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-07-01T11:24:00Z
- **Completed:** 2026-07-01T11:30:00Z
- **Tasks:** 2
- **Files modified:** 3 (2 created, 1 edited)

## Accomplishments
- `KamacuMark` — a reusable inline-SVG React component rendering an abstract warm-ember spark (radial glow halo + orange→amber gradient flame + rising ember particles). Named export, `cn()`/`...props`, `role="img"` + `aria-label="Kamacu"`, default `size-5` overridable via `className`. Static, lint-clean, type-checking. It exports exactly the contract the 20-03 sidebar lockup consumes.
- The mark is deliberately distinct from the flat `bg-amber-400` "agent waiting" dot (D-04): it is a shaped, gradient, multi-tone glow rather than a single-hue circle.
- `web/public/favicon.svg` — the same ember spark simplified for 16–32px on a rounded dark ember badge so it reads on both light and dark tab chrome; Vite copies it into `dist/favicon.svg`.
- `web/index.html` — tab title `Kangent` → `Kamacu`, added `<link rel="icon" type="image/svg+xml" href="/favicon.svg" />`; `<html lang="en" class="dark">` preserved; `web/embed.go` untouched.
- The Quechua *kamaq / kamacu* "the one who animates / gives life-force" naming story (D-02) is documented in the component JSDoc as the mark's rationale.

## Task Commits

Each task was committed atomically:

1. **Task 1: Create the KamacuMark ember-spark SVG component** — `603d187` (feat)
2. **Task 2: Create the favicon and set the browser-tab identity** — `c399abb` (feat)

## Files Created/Modified
- `web/src/components/brand/KamacuMark.tsx` — Inline-SVG ember-spark brand mark (named export `KamacuMark`), the contract consumed by the sidebar lockup.
- `web/public/favicon.svg` — Spark-derived favicon (flame on a rounded dark badge), root-served and embedded via `all:dist`.
- `web/index.html` — Kamacu tab title + `rel="icon"` favicon link; dark-theme html attribute preserved.

## Decisions Made
- **Ember geometry:** radial-gradient glow halo behind an orange→amber (`#ea580c`→`#f97316`→`#fcd34d`) linear-gradient flame body with two rising ember particles — a fuller, multi-tone, shaped glow that cannot be confused with the flat amber waiting dot (D-01/D-03/D-04).
- **Per-instance gradient ids** via `React.useId()` (colons stripped) so multiple marks on one page never collide on shared `<defs>` ids.
- **Favicon legibility:** drew the flame on a rounded dark ember badge (`#1c1917`) rather than a transparent-only mark, guaranteeing it reads on both light and dark browser-tab chrome (D-06).
- **Static asset:** no animation shipped; animated variants are the deferred BRAND-FUT-01.

## Deviations from Plan

None to the shipped source — both tasks executed exactly as written. Two execution-environment notes (neither changed shipped code beyond the plan's scope):

### Environment handling (not code deviations)

**1. [Rule 3 - Blocking] Worktree lacked `node_modules`**
- **Found during:** Task 1 (build verification)
- **Issue:** `npm run build` failed with `tsc: not found` — the parallel worktree had no `node_modules`.
- **Fix:** Symlinked the main checkout's `node_modules` into the worktree (`web/package.json` is byte-identical at this commit, so the dependency tree matches exactly). `node_modules` is gitignored, so the symlink is build-only and never committed. No new package was installed (no supply-chain surface — honors threat T-20-SC).
- **Verification:** `npm run build` and `eslint src/components/brand/KamacuMark.tsx` both exit 0.
- **Committed in:** N/A (build-only, not tracked)

**2. Reverted the incidental `web/dist/index.html` rebuild**
- **Found during:** Task 2 (build verification)
- **Issue:** `npm run build` overwrites the tracked placeholder `web/dist/index.html` (`.gitignore` ignores `web/dist/*` except this one placeholder, required so `//go:embed all:dist` never breaks). The real release bundle is produced by `make build` post-merge and is gitignored.
- **Fix:** Reverted the working-tree change with `git checkout -- web/dist/index.html`. Committing a per-worktree rebuilt bundle would embed an incomplete rebrand (only this plan's changes) and invite merge conflicts; the embedded SPA is rebuilt once post-merge (the repo's established "embedded SPA rebuilt" step). This keeps the plan within its declared `files_modified` scope.
- **Committed in:** N/A (reverted; worktree left clean)

## Issues Encountered

- **Pre-existing project-wide lint failures (out of scope):** `npm run lint` across the whole `web/` tree reports **20 pre-existing errors** (`react-hooks/set-state-in-effect`, `react-refresh/only-export-components`, etc.) in files this plan never touches (`use-mobile.ts`, `TaskPage.tsx`, `PRCard.tsx`, `ReviewColumn.tsx`, `sidebar.tsx`, `tabs.tsx`, `button.tsx`, `Board.tsx`, `StatusDot.tsx`, `DiffTab.tsx`, `TerminalPane.tsx`, `useTerminalSocket.ts`, `SettingsField.tsx`, `RenameProjectDialog.tsx`, `CleanupWorktreeDialog.tsx`). These are the documented known tech debt (STATE.md / PROJECT.md: "~18–20 pre-existing react-hooks eslint errors … gating build is green; a dedicated lint-cleanup pass is the right home"). Per the scope boundary these were NOT fixed. **The new `KamacuMark.tsx` lints clean on its own (eslint exit 0)** and appears in none of the 20 errors.

## Deferred Issues

- The 20 pre-existing project-wide lint errors above remain deferred to the dedicated lint-cleanup pass already tracked in STATE.md/PROJECT.md. They do not gate the build (`tsc -b && vite build` is green) and are unrelated to this plan's files.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- `KamacuMark` is importable via `@/components/brand/KamacuMark` and ready for the 20-03 sidebar lockup (expanded wordmark + collapsed-rail mark).
- The favicon + tab title are live in source; the final embedded SPA (`web/dist/`) should be rebuilt via `make build` once all Phase 20 rebrand plans are merged, before capturing the Kamacu README screenshot (D-12).
- Human-verify (mark renders, favicon in tab, visually distinct from the waiting dot) is deferred to the 20-04 checkpoint per the plan.

## Self-Check: PASSED

- FOUND: `web/src/components/brand/KamacuMark.tsx`
- FOUND: `web/public/favicon.svg`
- FOUND: `web/index.html` (`<title>Kamacu</title>` in HEAD tree)
- FOUND: `.planning/phases/20-kamacu-rebrand-brand/20-02-SUMMARY.md`
- FOUND commits: `603d187` (Task 1), `c399abb` (Task 2), `64c7ce9` (SUMMARY)

---
*Phase: 20-kamacu-rebrand-brand*
*Completed: 2026-07-01*

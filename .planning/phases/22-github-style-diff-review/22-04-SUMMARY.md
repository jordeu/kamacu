---
phase: 22-github-style-diff-review
plan: 04
subsystem: ui
tags: [react, intersection-observer, file-tree, scroll-spy, diff-review, tailwind]

# Dependency graph
requires:
  - phase: 22-github-style-diff-review (Plan 03)
    provides: "DiffFileSection with data-diff-path wrapper + scroll-mt-[41px], controlled collapse, Viewed checkbox, DiffTab section keys path:hash + taskId prop"
provides:
  - "FileTree component: client-side buildTree with GitHub-style single-child path compression, nested folders-default-expanded, scroll-only leaf click"
  - "useScrollSpy hook: one IntersectionObserver rooted on the right-pane scroll container returning the topmost-visible file path"
  - "Two-pane DiffTab: fixed 288px tree pane + PanelLeft toggle wired to scroll-spy highlight and scroll-only jump"
affects: [22-05 (phase verification / human-verify), future diff-review enhancements]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Client-side recursive tree build (pure buildTree in useMemo) derived from the already-sorted flat files list"
    - "IntersectionObserver scroll-spy rooted on an overflow-y-auto container observing non-sticky [data-diff-path] wrappers"
    - "Scroll-only navigation via scrollIntoView + CSS.escape + prefers-reduced-motion (no state mutation)"

key-files:
  created:
    - web/src/components/task/FileTree.tsx
    - web/src/hooks/use-scroll-spy.ts
  modified:
    - web/src/components/task/DiffTab.tsx

key-decisions:
  - "buildTree kept module-private (not exported) to satisfy react-refresh/only-export-components while remaining useMemo-internal"
  - "FileTree memoizes buildTree on the files reference (stable via TanStack structural sharing) so optimistic Viewed toggles rebuild the tree and refresh leaf state"
  - "Binary leaf rows omit the +add −del counts (nulls) to match DiffFileSection's binary header treatment"

patterns-established:
  - "Recursive JSX render helper (closure over tree state) instead of a nested component, avoiding remount churn"
  - "Selected tree row uses a reserved 2px border (border-l-2 border-transparent → border-blue-500) to avoid layout shift"

requirements-completed: [DIFF-01, DIFF-02, DIFF-04]

# Metrics
duration: 16min
completed: 2026-07-02
---

# Phase 22 Plan 04: Two-Pane File Tree + Scroll-Spy Summary

**GitHub "Files changed" two-pane diff view: a fixed 288px path-compressed file tree whose scroll-only clicks jump the right pane, with an IntersectionObserver scroll-spy that highlights the top file and a PanelLeft toggle to hide/show the tree.**

## Performance

- **Duration:** ~16 min
- **Started:** 2026-07-02
- **Completed:** 2026-07-02
- **Tasks:** 3
- **Files modified:** 3 (2 created, 1 modified)

## Accomplishments
- New `FileTree.tsx`: pure client-side `buildTree` (nest paths → GitHub-style single-child chain compression → dirs-before-files alphabetical), folders default-expanded (tree-local open state), leaf rows mirroring the DiffFileSection language (font-mono, tabular-nums, U+2212, green/red counts), selected/viewed row states, and scroll-only `onSelect` (never mutates collapse/Viewed).
- New `use-scroll-spy.ts`: one IntersectionObserver rooted on the right-pane scroll container (`root: scrollEl`, `rootMargin: -41px 0px -70% 0px`, `threshold: 0`) observing non-sticky `[data-diff-path]` wrappers, returning the topmost intersecting path; disconnects on cleanup and re-observes on file-list change (StrictMode-safe).
- Restructured `DiffTab.tsx` into a `flex h-full min-h-0` two-pane row: `w-72` tree pane (toggleable via a `PanelLeft` ghost button, `Hide file tree`/`Show file tree` aria) + the existing right scroll pane carrying `scrollRef`; wired `activePath` (scroll-spy) and `scrollToPath` (CSS.escape + prefers-reduced-motion, scroll-only). All loading/empty/error/refetch states and the `key=path:hash` + `taskId` section wiring preserved verbatim.

## Task Commits

Each task was committed atomically:

1. **Task 1: FileTree component (buildTree + path compression + scroll-only rows)** - `a8e04b6` (feat)
2. **Task 2: use-scroll-spy IntersectionObserver hook** - `e1723e9` (feat)
3. **Task 3: Two-pane DiffTab — FileTree, scroll-spy, PanelLeft toggle, scroll-only click** - `34ae612` (feat)

## Files Created/Modified
- `web/src/components/task/FileTree.tsx` - Nested, path-compressed, folders-default-expanded file tree; scroll-only leaf click via `onSelect`; selected/viewed row states.
- `web/src/hooks/use-scroll-spy.ts` - `useScrollSpy(scrollRef, pathListKey)` returning the topmost-visible `[data-diff-path]` file path via one IntersectionObserver.
- `web/src/components/task/DiffTab.tsx` - Two-pane layout: fixed 288px `FileTree` pane + right scroll pane; `PanelLeft` toggle; `activePath` highlight; `scrollToPath` scroll-only jump.

## Decisions Made
- **`buildTree` module-private:** exporting both `buildTree` and the `FileTree` component tripped `react-refresh/only-export-components`. `buildTree` is only consumed internally via `useMemo`, so it stays unexported — the plan's acceptance grep (`buildTree` present) still holds.
- **Memoize on the `files` reference, not just the joined path list:** TanStack structural sharing keeps `files` stable between renders, and an optimistic Viewed toggle produces a new `files` array. Keying the `useMemo` on `files` (rather than only the path string) guarantees the tree rebuilds and leaf rows reflect the new `viewed`/counts — a correctness requirement, since a path-only key would leave stale `file` refs in the tree nodes.
- **Recursive render helper over nested component:** used a closure function (`renderNodes`) returning JSX for recursion instead of defining a component inside `FileTree`, avoiding per-render remounts.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `react-refresh/only-export-components` on FileTree**
- **Found during:** Task 1 (FileTree component)
- **Issue:** Exporting the pure `buildTree` alongside the `FileTree` component caused an eslint `react-refresh/only-export-components` error (the `npm run lint` gate).
- **Fix:** Made `buildTree` module-private (dropped the `export`); it is only used internally by `FileTree`'s `useMemo`. Acceptance grep for `buildTree` still passes.
- **Files modified:** web/src/components/task/FileTree.tsx
- **Verification:** `npx eslint src/components/task/FileTree.tsx` exits 0.
- **Committed in:** a8e04b6 (Task 1 commit)

**2. [Rule 2 - Missing Critical] Binary leaf rows omit ± counts**
- **Found during:** Task 1 (FileTree leaf rendering)
- **Issue:** Binary files carry `additions`/`deletions` of `null`; rendering `+0 −0` would misrepresent them (DiffFileSection shows no counts for binary, only "Binary file changed").
- **Fix:** Guarded the count span with `!file.binary` so binary leaves show the filename (and Check glyph when viewed) without spurious `+0 −0`.
- **Files modified:** web/src/components/task/FileTree.tsx
- **Verification:** Build + lint green; matches DiffFileSection binary treatment.
- **Committed in:** a8e04b6 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking lint gate, 1 missing-critical presentation correctness)
**Impact on plan:** Both fixes are within scope and necessary for a green lint gate and consistent binary-file rendering. No scope creep; all three tasks delivered as specified.

## Issues Encountered
- **Incidental `web/dist/index.html` regeneration:** running `npm run build` (the frontend verification gate) rewrote the committed go:embed placeholder `web/dist/index.html`, pulling in unrelated drift (title, favicon, new asset hashes) that predates this plan. Reverted with `git checkout -- web/dist/index.html` after each build so the worktree contains only the three source-file changes; CI produces `dist` via `make build`.
- **Pre-existing lint backlog:** the repo carries 20 pre-existing `react-hooks/set-state-in-effect` errors (documented backlog), including 1 in `DiffTab.tsx`'s 150ms loading effect (preserved verbatim). The total stays at exactly 20 after this plan — no new lint errors introduced.

## Threat Model Notes
- **T-22-07 (leaked/duplicated observers):** mitigated — `useScrollSpy` calls `observer.disconnect()` in effect cleanup and rebuilds on `pathListKey` change (StrictMode-safe).
- **T-22-08 (unescaped querySelector path):** mitigated — `scrollToPath` builds the `[data-diff-path="…"]` selector with `CSS.escape(path)`.
- **T-22-SC (package installs):** no npm packages added — `FileTree`/`useScrollSpy` use React + the native IntersectionObserver; `PanelLeft`/`ChevronRight`/`Check` are already-present lucide-react icons. (`npm ci` was run only to hydrate the empty worktree `node_modules`; `package.json`/lockfile untouched.)

## Next Phase Readiness
- DIFF-01 (left tree + scroll-only select + scroll-spy highlight), and the DIFF-02/DIFF-04 rendered outcomes are complete on the frontend.
- Ready for Plan 05 phase verification / human-verify against the UI-SPEC (tree lists all changed files path-compressed, click scrolls, scroll highlights the top file, PanelLeft hides/shows, collapse + Viewed still behave per Plan 03).
- No frontend test framework exists (house rule); verification is `cd web && npm run build && npm run lint` (both green) + human UAT.

---
*Phase: 22-github-style-diff-review*
*Completed: 2026-07-02*

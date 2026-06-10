---
phase: 01-foundation-projects-board
plan: 03
subsystem: ui
tags: [react, vite, tailwind, shadcn, tanstack-query, react-router, dnd-kit]

# Dependency graph
requires:
  - phase: 01-foundation-projects-board (plan 01-02)
    provides: backend REST contract (/api/projects, /api/tasks, move endpoint JSON shapes) — types.ts mirrors it verbatim
provides:
  - Buildable Vite + React 19 + TypeScript SPA in web/ (dist/ ready for Go embed)
  - Dark-only zinc theme encoding UI-SPEC tokens (radius 0.375rem, 14px body, system font stacks)
  - Typed API contract layer: Project/Task/Status types, fetch client with ApiError, 3 query hooks, 7 mutation hooks incl. optimistic useMoveTask
  - Router skeleton: /, /projects/:projectId, /projects/:projectId/tasks/:taskId under AppLayout with always-visible collapsible sidebar
  - 11 shadcn/ui components (button, dialog, alert-dialog, input, textarea, tabs, sidebar, dropdown-menu, tooltip, separator, skeleton)
affects: [01-04 sidebar/projects, 01-05 board, 01-06 task view, 01-07 embed/build]

# Tech tracking
tech-stack:
  added:
    - react 19 + vite 8 + typescript 6 (react-ts template)
    - tailwindcss 4.3 via @tailwindcss/vite (CSS-first, no config files)
    - shadcn CLI 4.11 (radix base, CSS variables) + tw-animate-css + shadcn/tailwind.css
    - "@tanstack/react-query 5.101, react-router 7.17 (single package), @dnd-kit/core+sortable+utilities, react-markdown 10, remark-gfm 4, @tailwindcss/typography"
  patterns:
    - Query keys are the cache contract — ["projects"], ["tasks", projectId], ["task", taskId]
    - Optimistic mutation pattern (onMutate snapshot/setQueryData, onError rollback, onSettled invalidate)
    - "@ alias to src/, /api dev proxy to 127.0.0.1:7333"
    - Dark-only theming via hardcoded class="dark" on <html>; zinc CSS variables in index.css

key-files:
  created:
    - web/vite.config.ts
    - web/src/index.css
    - web/src/api/types.ts
    - web/src/api/client.ts
    - web/src/api/queries.ts
    - web/src/api/mutations.ts
    - web/src/App.tsx
    - web/src/main.tsx
    - web/src/components/layout/AppLayout.tsx
    - web/src/pages/BoardPage.tsx
    - web/src/pages/TaskPage.tsx
    - web/src/components/ui/sidebar.tsx
  modified:
    - web/index.html
    - web/tsconfig.json
    - web/tsconfig.app.json
    - web/package.json

key-decisions:
  - "shadcn CLI 4.x replaced --base-color with presets: initialized with -b radix --preset nova, then hand-encoded the zinc CSS-variable ramp in index.css to honor the UI-SPEC contract"
  - "Removed @fontsource-variable/geist that the nova preset injected — UI-SPEC mandates system font stacks only (offline-clean single binary)"
  - "Dropped baseUrl from tsconfigs (TS 6 deprecation error TS5101); paths alone resolves the @ alias"

patterns-established:
  - "API layer is the single contract surface: feature plans import types/hooks, never call fetch directly"
  - "Mutations own their invalidation; useMoveTask is the only optimistic one"

requirements-completed: [STOR-02]

# Metrics
duration: 10min
completed: 2026-06-10
---

# Phase 01 Plan 03: Frontend Shell Summary

**Vite + React 19 + Tailwind 4 SPA with dark-only zinc shadcn theme, full typed TanStack Query API layer (10 hooks incl. optimistic move), and the 3-route shell feature plans build against**

## Performance

- **Duration:** 10 min
- **Started:** 2026-06-10T06:32:03Z
- **Completed:** 2026-06-10T06:42:33Z
- **Tasks:** 3
- **Files modified:** 41 (28 + 4 + 9 across three commits)

## Accomplishments

- `web/` builds clean (`npm run build` → `dist/index.html`, the artifact plan 01-07 embeds in the Go binary)
- Complete frontend contract surface: Status/Project/Task types matching the 01-02 backend JSON verbatim, fetch client with `ApiError`, query hooks with locked cache keys, all 7 mutation hooks including `useMoveTask` with the full optimistic snapshot/rollback/invalidate pattern
- Dark-only zinc theme per UI-SPEC: `class="dark"` hardcoded, `--radius: 0.375rem`, 14px body, system sans + mono stacks, `@tailwindcss/typography` via `@plugin`, zero webfonts
- Router shell with AppLayout (SidebarProvider, always-visible sidebar with trigger) wrapping board and task routes; plans 01-04/05/06 can now run in parallel without touching any shared file

## Task Commits

Each task was committed atomically:

1. **Task 1: Vite scaffold + Tailwind 4 + shadcn init + dependencies** - `91d86e8` (feat)
2. **Task 2: UI-SPEC design tokens — dark-only theme, density, typography** - `f83d1e0` (feat)
3. **Task 3: API contract layer + router shell with stub pages** - `44a9ec2` (feat)

## Files Created/Modified

- `web/vite.config.ts` - tailwindcss v4 plugin, `@` alias, `/api` proxy to 127.0.0.1:7333
- `web/index.html` - `class="dark"` on html, title Kangent
- `web/src/index.css` - zinc CSS-variable ramp, typography plugin, density tokens, font stacks
- `web/src/api/types.ts` - Status/STATUSES/STATUS_LABELS/Project/Task — the single type contract
- `web/src/api/client.ts` - `api<T>()` fetch wrapper + get/post/patch/del helpers, ApiError with server `{error}` message
- `web/src/api/queries.ts` - useProjects/useTasks/useTask
- `web/src/api/mutations.ts` - 7 mutation hooks; `applyMove` local resequencing for optimistic drag
- `web/src/main.tsx` - StrictMode → QueryClientProvider → BrowserRouter → App
- `web/src/App.tsx` - 3 routes nested under AppLayout + RedirectToFirstProject
- `web/src/components/layout/AppLayout.tsx` - SidebarProvider shell with placeholder sidebar
- `web/src/pages/BoardPage.tsx`, `web/src/pages/TaskPage.tsx` - param-reading stubs
- `web/src/components/ui/*` - 11 shadcn components (+ sheet, a sidebar dependency)

## Decisions Made

- **shadcn CLI 4.x interface change:** `--base-color zinc` no longer exists; the CLI now uses presets (nova/vega/...). Initialized with `-b radix --preset nova -y`, then replaced the generated neutral oklch variables with the canonical shadcn zinc ramp in index.css — the UI-SPEC color contract (zinc-950 background etc.) is encoded exactly.
- **Webfont removal:** the nova preset injected `@fontsource-variable/geist`; removed the package and import since UI-SPEC mandates system font stacks only (offline-clean single binary).
- **TS 6 `baseUrl` deprecation:** TS 6.0.3 errors (TS5101) on `baseUrl`; the `@` alias works with `paths` alone, so `baseUrl` was dropped from both tsconfigs instead of suppressing the deprecation.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] shadcn CLI 4.x removed `--base-color`, uses presets**
- **Found during:** Task 1 (shadcn init)
- **Issue:** Plan's command `npx shadcn@latest init --base-color zinc --yes` fails — CLI 4.11.0 replaced base-color flags with presets (nova/vega/...) and a `-b radix|base` library flag
- **Fix:** Ran `npx shadcn@latest init -b radix --preset nova -y`; hand-encoded the zinc variable ramp in index.css during Task 2 so the UI-SPEC zinc contract holds
- **Files modified:** web/components.json, web/src/index.css
- **Verification:** init completed; build green; index.css carries zinc oklch values
- **Committed in:** 91d86e8 / f83d1e0

**2. [Rule 3 - Blocking] Corrupted node_modules — tslib missing on disk**
- **Found during:** Task 1 (shadcn add)
- **Issue:** `shadcn add` crashed (`Cannot find module 'tslib'` from recast); npm's tree claimed tslib was installed but it was absent on disk after shadcn init's own dependency install
- **Fix:** `rm -rf node_modules && npm install` (clean reinstall from lockfile); also kept tslib as an explicit devDependency
- **Files modified:** web/package.json, web/package-lock.json
- **Verification:** `shadcn add` succeeded; all 11 components generated
- **Committed in:** 91d86e8

**3. [Rule 2 - Missing Critical] Removed Geist webfont injected by nova preset**
- **Found during:** Task 2 (design tokens)
- **Issue:** shadcn nova preset added `@fontsource-variable/geist` + CSS import — violates UI-SPEC "no webfont imports of any kind" (offline-clean single binary); woff2 files were landing in dist/
- **Fix:** Uninstalled the package, removed the import, set `--font-sans`/`--font-mono` to system stacks
- **Files modified:** web/src/index.css, web/package.json, web/package-lock.json
- **Verification:** dist/ contains no font assets; grep confirms no @font-face/fontsource references
- **Committed in:** f83d1e0

**4. [Rule 1 - Bug] TS 6 deprecation error on `baseUrl`**
- **Found during:** Task 1 (build verification)
- **Issue:** Plan instructed adding `"baseUrl": "."`; TypeScript 6.0.3 fails the build with TS5101 (baseUrl deprecated)
- **Fix:** Removed baseUrl from both tsconfigs; `paths: {"@/*": ["./src/*"]}` resolves relative to the tsconfig without it
- **Files modified:** web/tsconfig.json, web/tsconfig.app.json
- **Verification:** `npm run build` and `tsc --noEmit` clean
- **Committed in:** 91d86e8

**5. [Rule 1 - Bug] `erasableSyntaxOnly` forbids constructor parameter properties**
- **Found during:** Task 3 (client.ts)
- **Issue:** Plan's `constructor(message: string, public status: number)` fails TS1294 — the Vite template enables `erasableSyntaxOnly`
- **Fix:** Explicit `status` field assignment in the constructor body
- **Files modified:** web/src/api/client.ts
- **Verification:** build green
- **Committed in:** 44a9ec2

---

**Total deviations:** 5 auto-fixed (2 bugs, 1 missing critical, 2 blocking)
**Impact on plan:** All fixes kept the locked stack and UI-SPEC contract intact; no scope creep. The shadcn CLI preset change is worth knowing for any future `shadcn add` usage.

## Known Stubs

Intentional, planned seams — each is replaced by a named wave-2 plan:

- `web/src/pages/BoardPage.tsx` — renders muted `Board {projectId}` placeholder; plan 01-05 builds the kanban board here
- `web/src/pages/TaskPage.tsx` — renders muted `Task {taskId}` placeholder; plan 01-06 builds the task view here
- `web/src/components/layout/AppLayout.tsx` — `<SidebarContent />` is empty (app name only in header); plan 01-04 fills in the project list
- `web/src/App.tsx` `RedirectToFirstProject` — renders null when no projects exist; plan 01-04 supplies the empty state

None block this plan's goal (contract surface + buildable shell).

## Issues Encountered

None beyond the auto-fixed deviations above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 01-04 (sidebar/projects), 01-05 (board), 01-06 (task view) can execute in parallel — every shared file (types, hooks, routes, layout, theme) exists and compiles
- Plan 01-07 can embed `web/dist` (build verified to emit it)
- Dev proxy targets 127.0.0.1:7333 matching the Go server from plan 01-01

## Self-Check: PASSED

- All 12 key created files verified on disk
- Commits 91d86e8, f83d1e0, 44a9ec2 verified in git log
- `npm run build` exit 0, `tsc -p tsconfig.app.json --noEmit` clean

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

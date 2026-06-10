---
phase: 01-foundation-projects-board
plan: 07
subsystem: infra
tags: [go, embed, vite, makefile, spa-fallback, sqlite, smoke-test]

# Dependency graph
requires:
  - phase: 01-01
    provides: Go module, HTTP server skeleton, --addr/--db flags
  - phase: 01-02
    provides: SQLite store, /api routes (projects, tasks, move, healthz)
  - phase: 01-03
    provides: Vite + React SPA scaffold building into web/dist
  - phase: 01-04
    provides: projects sidebar (collapse persistence, tooltips)
  - phase: 01-05
    provides: kanban board with drag-and-drop persistence
  - phase: 01-06
    provides: full-page task view with markdown descriptions
provides:
  - Single binary (bin/kangent) embedding the built SPA via //go:embed all:dist
  - SPA fallback handler: deep links serve index.html, /api/* never does (404)
  - Makefile build pipeline (frontend -> backend -> bin/kangent) + dev targets
  - scripts/smoke.sh — automated restart-persistence proof over HTTP
  - Human-approved Phase 1 end-to-end experience
affects: [02-terminal-engine, deployment, build-pipeline]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "//go:embed all:dist with committed placeholder index.html so fresh clones compile"
    - "SPA fallback excludes /api/ prefix and sets Cache-Control no-store on index, hashed assets stay cacheable"
    - "Smoke tests drive the real binary over HTTP with kill/restart cycle, pure grep/sed (no jq)"

key-files:
  created:
    - web/embed.go
    - web/dist/index.html
    - Makefile
    - scripts/smoke.sh
  modified:
    - cmd/kangent/main.go
    - .gitignore
    - web/.gitignore
    - web/src/components/layout/AppLayout.tsx

key-decisions:
  - "Placeholder web/dist/index.html committed (gitignore: web/dist/* with !index.html) so go build never fails on fresh clone; real build output never committed"
  - "AppLayout wrapped in TooltipProvider delayDuration={0} — Radix tooltips crash the whole tree without a provider"
  - "Collapsed-sidebar toggle gets a reserved pl-9 gutter on main instead of floating over page headers"

patterns-established:
  - "Embed pattern: web/embed.go exports DistFS; main.go does fs.Sub + http.FileServerFS with /api/ exclusion"
  - "End-to-end proofs live in scripts/*.sh and run against a freshly built binary on a throwaway port/db"

requirements-completed: [STOR-01, STOR-02]

# Metrics
duration: ~30min (including two checkpoint fix cycles)
completed: 2026-06-10
---

# Phase 01 Plan 07: Single-Binary Embed & Phase Verification Summary

**One binary serves the full kanban app: //go:embed'd Vite build with SPA-fallback routing, Makefile pipeline, and a scripted kill/restart persistence proof — human-approved end to end**

## Performance

- **Duration:** ~30 min (first task commit 09:11, final checkpoint fix 09:34, plus human verification cycle)
- **Started:** 2026-06-10T07:11:16Z
- **Completed:** 2026-06-10T07:34:41Z (code) / human approval after walkthrough
- **Tasks:** 3 (2 auto + 1 human-verify checkpoint)
- **Files modified:** 8

## Accomplishments

- `make build` produces one binary (`bin/kangent`) that serves both the JSON API and the React app; deep links like `/projects/1/tasks/2` return the SPA while unknown `/api/*` paths 404 and never serve HTML
- Fresh clones always compile: a placeholder `web/dist/index.html` is committed and the real build output is gitignored
- `scripts/smoke.sh` proves STOR-01 mechanically: creates a project + task over HTTP, moves and edits it, kills the server, restarts on the same DB, and asserts the data is byte-identical — prints `SMOKE OK`
- Human verified the full Phase 1 walkthrough (10 steps: empty state, invalid-path error handling, both task-creation paths, drag persistence across reload, deep-linked task view, GFM markdown editing, deletions with repo-untouched guarantee, server-restart persistence, sidebar collapse persistence) and responded "approved"

## Task Commits

Each task was committed atomically:

1. **Task 1: Embed + SPA fallback + Makefile + .gitignore** - `ff52a1c` (feat)
2. **Task 2: Restart-persistence smoke test** - `d093dc4` (feat)
3. **Checkpoint fix: TooltipProvider crash** - `7b600c1` (fix)
4. **Checkpoint fix: collapsed-sidebar toggle overlap** - `7a020d2` (fix)
5. **Task 3: Human verification checkpoint** - approved by user (no code)

## Files Created/Modified

- `web/embed.go` - `//go:embed all:dist` exporting `DistFS`
- `web/dist/index.html` - committed placeholder ("run make build") so `go build` works on fresh clones
- `cmd/kangent/main.go` - SPA fallback handler: `/api/` prefix 404s, existing files served cacheable, everything else gets index.html with `Cache-Control: no-store`
- `Makefile` - `build` (frontend then backend into `bin/kangent`), `test`, `dev-backend`, `dev-frontend`, `clean`
- `.gitignore` / `web/.gitignore` - `bin/`, `web/node_modules/`, `web/dist/*` except the placeholder
- `scripts/smoke.sh` - 114-line end-to-end restart-persistence proof (no jq dependency)
- `web/src/components/layout/AppLayout.tsx` - TooltipProvider wrapper + collapsed-sidebar gutter (checkpoint fixes)

## Decisions Made

- Committed placeholder index.html instead of build-tag tricks — simplest way to keep `go build` green without embedding real artifacts
- Smoke script uses pure grep/sed for JSON extraction to avoid a jq dependency
- TooltipProvider mounted once at the AppLayout root with `delayDuration={0}` rather than per-tooltip providers

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Radix Tooltip used without TooltipProvider crashed the app to a blank page**
- **Found during:** Task 3 (human-verify checkpoint — user reported blank page)
- **Issue:** Sidebar components used Radix Tooltip without an ancestor TooltipProvider; the first render threw, React unmounted the entire tree, and the served app rendered a blank page. The smoke test missed it because it only asserts HTTP responses, not browser rendering.
- **Fix:** Wrapped AppLayout in `<TooltipProvider delayDuration={0}>`
- **Files modified:** web/src/components/layout/AppLayout.tsx
- **Verification:** User reloaded and walked the full checkpoint; UI renders
- **Committed in:** 7b600c1

**2. [Rule 1 - Bug] CollapsedSidebarTrigger overlapped page header titles when sidebar collapsed**
- **Found during:** Task 3 (human-verify checkpoint — user reported overlap)
- **Issue:** The trigger was absolutely positioned (`top-2 left-2`) and sat on top of page header titles whenever the sidebar was collapsed
- **Fix:** Added `pl-9` to `main` when the sidebar is closed, reserving a gutter for the toggle
- **Files modified:** web/src/components/layout/AppLayout.tsx
- **Verification:** User confirmed headers clear of the toggle during checkpoint re-walk
- **Committed in:** 7a020d2

---

**Total deviations:** 2 auto-fixed (2 bugs, both found during the human-verify checkpoint cycle)
**Impact on plan:** Both fixes confined to AppLayout.tsx; required for the checkpoint to pass. No scope creep. Lesson recorded: HTTP-level smoke tests do not catch render-time crashes.

## Issues Encountered

None beyond the two checkpoint-cycle bugs documented above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 1 complete: persistent kanban board (projects, tasks, drag-and-drop, markdown task view) served by one local binary, all five roadmap success criteria human-approved
- Build pipeline (`make build`) and smoke script give Phase 2 a stable integration baseline for the terminal engine
- Known gap to keep in mind: smoke coverage is API-level only; browser-render regressions need manual or future E2E coverage

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

## Self-Check: PASSED

All claimed files exist on disk; all four task/fix commits (ff52a1c, d093dc4, 7b600c1, 7a020d2) present in git history.

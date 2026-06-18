---
phase: quick-260618-mlu
plan: 01
subsystem: ui
tags: [react, active-sessions-bar, localStorage, navigation]

# Dependency graph
requires:
  - phase: 17-global-active-sessions-bar (v1.6)
    provides: ActiveSessionsBar component with collapse persistence + SessionRow onOpen navigation
provides:
  - Active Sessions Bar auto-collapses (and persists collapsed) when a session row is opened
affects: [active-sessions-bar, sessions-bar-collapse]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Dedicated unconditional collapse() helper (set true + persist '1') distinct from the relative toggle()"

key-files:
  created: []
  modified:
    - web/src/components/layout/ActiveSessionsBar.tsx

key-decisions:
  - "Added a separate collapse() helper rather than reusing toggle() — toggle flips relative to current state, which could re-expand; collapse() sets true unconditionally and persists '1' to survive reload."
  - "Single call-site change in SessionRow onOpen covers click AND keyboard (Enter/Space), since SessionRow already routes both through onOpen — SessionRow left untouched."
  - "Order: navigate() first, then collapse() — start the route change, then dismiss the now-redundant overlay."

patterns-established:
  - "Pattern: open-dismisses-overlay — opening an item from an expanded list collapses the list and persists the collapsed state."

requirements-completed: [SBAR-06]

# Metrics
duration: 3min
completed: 2026-06-18
---

# Phase quick-260618-mlu Plan 01: Collapse Active Sessions Bar on Row Open Summary

**Active Sessions Bar now auto-collapses and persists collapsed when a session row is opened (click or Enter/Space), so the expanded overlay no longer floats over the freshly-opened task.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-06-18T14:19:28Z
- **Completed:** 2026-06-18T14:21:46Z
- **Tasks:** 1 auto (+ 1 human-verify checkpoint pending)
- **Files modified:** 1

## Accomplishments
- Added an unconditional `collapse()` helper that sets `collapsed=true` and persists `"1"` to localStorage under the existing `kangent:sessions-bar-collapsed` key.
- Wired `SessionRow`'s `onOpen` to call `navigate(...)` then `collapse()`, giving click + keyboard (Enter/Space) parity via the single existing call site.
- Left `toggle`, the chevron/bar toggle handlers, `storageKey`, persistence idiom, and `SessionRow` itself unchanged — no regression to manual expand/collapse.

## Task Commits

Each task was committed atomically:

1. **Task 1: Collapse (and persist) the bar when a session row is opened** - `b7a5e1f` (feat)

## Files Created/Modified
- `web/src/components/layout/ActiveSessionsBar.tsx` - Added `collapse()` helper; changed `SessionRow` `onOpen` to navigate then collapse.

## Decisions Made
- Used a dedicated `collapse()` helper instead of `toggle()` — `toggle()` flips relative to current state (could re-expand); `collapse()` sets `true` unconditionally and writes `"1"` so the collapsed state persists across reloads (must_have truth #3).
- Single call-site change in `SessionRow` `onOpen` covers both click and Enter/Space because `SessionRow` already routes `onClick` and the Enter/Space `onKeyDown` through `onOpen` — `SessionRow` was not modified (must_have truth #2).
- Navigate first, then collapse, so the route change kicks off before the overlay is dismissed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Installed locked frontend dependencies to run the build**
- **Found during:** Task 1 verification (`npm run build`)
- **Issue:** `web/node_modules` was absent in the fresh worktree (`tsc: not found`), blocking the automated verification gate.
- **Fix:** Ran `npm ci` (installs exactly from the committed `package-lock.json` — not a new/unverified package install, so the Rule 3 package-install exclusion does not apply).
- **Files modified:** None tracked (node_modules is gitignored; lockfile unchanged).
- **Verification:** `npm run build` and `npm run lint` ran successfully afterward.
- **Committed in:** N/A (no tracked files changed).

---

**Total deviations:** 1 auto-fixed (1 blocking — dependency install only).
**Impact on plan:** No scope creep. The only source change is the planned one-file edit to `ActiveSessionsBar.tsx`.

## Issues Encountered
- The `vite build` regenerated `web/dist/index.html` (the tracked embed placeholder) with a new hashed asset reference. Since the hashed JS/CSS asset files themselves are gitignored and the plan scopes the change to `ActiveSessionsBar.tsx`, the dist artifact change was reverted (`git checkout -- web/dist/index.html`) and excluded from the commit to avoid committing a dangling asset-hash reference. The release build will regenerate dist normally via `make build`.

## Verification

**Automated (passed):**
- `cd web && npm run build` — `tsc -b` typecheck + `vite build` both green, no new type errors. (Chunk-size > 500 kB note is a pre-existing advisory, not an error.)
- `cd web && npm run lint` — 20 pre-existing react-hooks/react-refresh advisories in unrelated files (use-mobile.ts, TaskPage.tsx, etc.), matching the ~18-20 carried advisories noted in STATE.md. `ActiveSessionsBar.tsx` introduces ZERO new advisories.

**Human-verify (PENDING — to be performed by the orchestrator):**
The plan's `checkpoint:human-verify` gate is not yet satisfied. Pending steps:
1. Run the app with at least one LIVE agent session so the bar shows entries.
2. Expand the bar (bar body or up-chevron); click a session row — expect navigation to the task's agent view AND the bar collapsing to the thin counts strip.
3. Expand again, Tab to a row, press Enter (then Space) — expect same navigate + collapse.
4. After a row-click, reload — expect the bar comes back COLLAPSED (persistence).
5. Regression: toggle expand/collapse via the bar and chevron a few times — expect unchanged behavior; chevron does not navigate; `e.stopPropagation()` still prevents a double-toggle.

## Known Stubs
None.

## Next Phase Readiness
- Code change complete and committed; automated gate green.
- Awaiting human runtime verification of the row-open collapse behavior (orchestrator to perform).

## Self-Check: PASSED

- `web/src/components/layout/ActiveSessionsBar.tsx` — FOUND
- `.planning/quick/260618-mlu-when-the-status-bottom-bar-is-expanded-a/260618-mlu-SUMMARY.md` — FOUND
- Commit `b7a5e1f` — FOUND
- Committed file contains `collapse()` helper and `collapse()` call in `SessionRow` `onOpen` — VERIFIED

---
*Phase: quick-260618-mlu*
*Completed: 2026-06-18*

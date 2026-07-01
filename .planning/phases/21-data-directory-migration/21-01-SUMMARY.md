---
phase: 21-data-directory-migration
plan: 01
subsystem: ui
tags: [localstorage, react, migration, rebrand, kamacu]

# Dependency graph
requires:
  - phase: 20-kamacu-rebrand-brand
    provides: "kangent → kamacu code/UI rename that deliberately left the localStorage key literals on the kangent prefix for this phase to migrate"
provides:
  - "One-shot idempotent localStorage prefix migration (web/src/lib/migrateStorage.ts) copying every kangent-prefixed value to the kamacu-prefixed key on boot (MIGRATE-04, D-14)"
  - "The sidebar, sessions-bar, and per-project review-collapse components now read/write the kamacu-prefixed keys"
affects: [phase-21-plan-05-browser-checkpoint, phase-24-session-bar-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Client-side one-shot idempotent boot migration: guard flag + separator-agnostic prefix scan over all localStorage keys, run before createRoot"

key-files:
  created:
    - web/src/lib/migrateStorage.ts
  modified:
    - web/src/main.tsx
    - web/src/components/layout/AppLayout.tsx
    - web/src/components/layout/ActiveSessionsBar.tsx
    - web/src/components/board/ReviewColumn.tsx

key-decisions:
  - "Separator-agnostic prefix scan (bare-word 'kangent') over ALL localStorage keys instead of a hard-coded key list — the three live keys use a dot, a colon, AND a dynamic per-project colon suffix, so a fixed list would miss 2 of 3 (RESEARCH Pitfall 3)"
  - "Old kangent.* keys are left in place (harmless residue) and kamacu.* keys are only written when absent, so the copy is re-runnable if the guard flag is ever cleared"
  - "Migration invoked before createRoot so components read the new keys on first render"

patterns-established:
  - "Client boot migration mirrors the server BackfillProjectIcons idempotent-startup-hook shape: single entry point, up-front idempotency gate (kamacu.storage-migrated === '1'), no-op on the already-done path"

requirements-completed: [MIGRATE-04]

# Metrics
duration: 6min
completed: 2026-07-01
---

# Phase 21 Plan 01: localStorage kangent → kamacu Migration Summary

**One-shot idempotent localStorage prefix-scan migration copies every kangent-prefixed value to the kamacu key on boot, and the sidebar / sessions-bar / review-collapse components now read the new keys — completing the rebrand's client-side carry-over (MIGRATE-04).**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-01T14:36:32Z
- **Completed:** 2026-07-01T14:41:44Z
- **Tasks:** 2
- **Files modified:** 5 (1 created, 4 modified)

## Accomplishments
- New `web/src/lib/migrateStorage.ts`: a guard-flagged, separator-agnostic prefix scan over all localStorage keys that copies `kangent*` values to their `kamacu*` counterparts (only when the target key is absent), covering the dot key, the colon key, and the dynamic per-project review-collapse keys in one pass.
- Wired `migrateStorage()` into `main.tsx` immediately before `createRoot`, so the new keys exist before the first React render reads them.
- Flipped the three live key literals to the `kamacu` prefix (`AppLayout` sidebar, `ActiveSessionsBar` collapse, `ReviewColumn` per-project collapse), so nothing reads the old prefix anymore.

## Task Commits

Each task was committed atomically:

1. **Task 1: One-shot prefix-scan localStorage migration + call site** - `534d3ce` (feat)
2. **Task 2: Flip the three localStorage key literals to kamacu** - `cd5077c` (feat)

## Files Created/Modified
- `web/src/lib/migrateStorage.ts` (created) - Exports `migrateStorage(): void`; idempotency guard on `kamacu.storage-migrated`, index scan via `localStorage.length`/`localStorage.key(i)`, `.startsWith("kangent")` match, `newKey = "kamacu" + key.slice("kangent".length)`, copy only when the target is `null`.
- `web/src/main.tsx` (modified) - Imports and calls `migrateStorage()` before `createRoot`.
- `web/src/components/layout/AppLayout.tsx` (modified) - `SIDEBAR_STORAGE_KEY`: `kangent.sidebar` → `kamacu.sidebar`.
- `web/src/components/layout/ActiveSessionsBar.tsx` (modified) - `storageKey`: `kangent:sessions-bar-collapsed` → `kamacu:sessions-bar-collapsed`.
- `web/src/components/board/ReviewColumn.tsx` (modified) - `storageKey` template: `kangent:review-collapsed:${projectId}` → `kamacu:review-collapsed:${projectId}`.

## Decisions Made
- None beyond the plan — the D-14 algorithm and the three key flips were followed as specified. See frontmatter `key-decisions` for the design points inherited from the plan/RESEARCH.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Restored the web dependency tree so build/lint could run**
- **Found during:** Task 1 (verification)
- **Issue:** The worktree had no `web/node_modules`; `tsc`/`eslint`/`vite` were absent, so `npm run build`/`npm run lint` could not execute.
- **Fix:** Ran `npm ci` in `web/` to restore the already-declared dependency tree from the committed `package-lock.json` (not adding any new package — the Rule 3 package-install exclusion targets new/unknown package names, not lockfile restores).
- **Files modified:** None tracked (`node_modules` is gitignored).
- **Verification:** `npm run build` (tsc -b + vite build) subsequently exits 0.
- **Committed in:** N/A (no tracked file change).

**2. [Rule 1 - Bug] Reworded migrateStorage.ts doc comments to satisfy the Task-2 grep guard**
- **Found during:** Task 2 (verification)
- **Issue:** My Task-1 JSDoc referenced the old key names *with* their separators (`kangent` + `.`/`:`) as documentation examples, which matched the Task-2 acceptance guard `grep -rnE 'kangent[.:]' web/src/components/ web/src/lib/` and made it fail. The plan's guard is designed to pass with only the bare-word `kangent` scan literals present in that file.
- **Fix:** Reworded the comments to describe the key shapes prose-style ("the sidebar key uses a dot, the sessions-bar-collapsed key uses a colon, ...") without emitting a `kangent.`/`kangent:` sequence. The functional bare-word scan literals (`.startsWith("kangent")`, `key.slice("kangent".length)`) are unchanged.
- **Files modified:** web/src/lib/migrateStorage.ts
- **Verification:** `grep -rnE 'kangent[.:]' web/src/components/ web/src/lib/` now returns nothing; build green.
- **Committed in:** `cd5077c` (Task 2 commit).

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both were mechanical enablers (restore deps; keep a doc comment from tripping the plan's own guard). No behavioral change, no scope creep.

## Issues Encountered

- **Pre-existing lint errors are out of scope (not fixed).** `cd web && npm run lint` exits 1 with 20 pre-existing `react-hooks` errors in files untouched by this plan — the known tech debt already documented in STATE.md ("~18–20 pre-existing react-hooks eslint errors ... gating build is green; a dedicated lint-cleanup pass is the right home") and PROJECT.md "Known tech debt". Per the scope-boundary rule these were logged to `deferred-items.md` and left alone. The files this plan created/modified are lint-clean: `npx eslint` scoped to `migrateStorage.ts` + `main.tsx` exits 0; the one lint error in a file I touched (`ReviewColumn.tsx:184` `Date.now()`-in-render) is pre-existing tech debt at a line unrelated to my line-56 literal flip. The gating `tsc -b && vite build` is green for the whole tree.
- Runtime end-to-end carry-over (a real `kangent.*` value → populated `kamacu.*` key on first load) is intentionally deferred to Plan 21-05's browser checkpoint against a real install, per the plan's verification section.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- MIGRATE-04 is code-complete; the localStorage half of the rebrand migration is done and builds green.
- Plan 21-05's browser checkpoint should verify the runtime carry-over end-to-end (existing kangent.* preferences survive the first kamacu boot).
- No blockers for the other Phase 21 plans (data-dir move, DB path rewrite, tmux socket flip) — this plan is self-contained frontend-only work with no shared-file overlap.

---
*Phase: 21-data-directory-migration*
*Completed: 2026-07-01*

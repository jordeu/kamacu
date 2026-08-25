---
phase: 11-activity-page-controls
plan: 01
subsystem: api
tags: [react, typescript, tanstack-query, activity, localStorage]

requires:
  - phase: 10-activity-data-api
    provides: GET /api/activity combined endpoint (tasks + reviews + stats, always HTTP 200)
provides:
  - Activity wire types (ActivityReviewState, ActivityTask, ReviewDoneSummary, TimeStat, StatsBlock, ActivityResponse) in types.ts
  - useActivity(scope, window, enabled) TanStack query in queries.ts
  - formatDuration(totalSeconds) adaptive d/h/m/s formatter in time.ts
  - useActivityView() localStorage-backed scope+window hook (new file)
  - ActivityScope / ActivityWindow type aliases
affects: [11-activity-page-controls/02, activity-page-rendering]

tech-stack:
  added: []
  patterns:
    - "Plain React hook (NOT context) for single-consumer view state — useActivityView mirrors useActiveWorkspace's localStorage pattern but skips the context/provider because only ActivityPage consumes scope/window"
    - "Inline structural types in useActivity signature — scope/window types match parseScope/parseWindow grammar without importing from a not-yet-created file (TypeScript structural typing)"

key-files:
  created:
    - web/src/lib/useActivityView.ts
  modified:
    - web/src/api/types.ts
    - web/src/api/queries.ts
    - web/src/lib/time.ts

key-decisions:
  - "Split Task 1 (tdd=true) into 3 commits: non-TDD parts (types+query) first, then RED→GREEN for formatDuration — keeps TDD discipline clean for the behavior-specified function while not forcing wire-type declarations into a test commit"
  - "Used Node 24 native --experimental-strip-types for formatDuration behavior verification instead of installing tsx/vitest — respects the zero-new-packages constraint (threat model T-11-SC) while providing real assertion-based testing"

patterns-established:
  - "D-10 behavior table verified via node --experimental-strip-types inline test runner — ephemeral .mts test file pattern for zero-dep TS testing"

requirements-completed: [ACT-02, ACT-03]

coverage:
  - id: D1
    description: "Activity wire types (ActivityReviewState, ActivityTask, ReviewDoneSummary, TimeStat, StatsBlock, ActivityResponse) matching Phase 10 Go structs field-for-field"
    requirement: ACT-02
    verification:
      - kind: other
        ref: "tsc --noEmit exits 0 (type system proves internal consistency); field-for-field match against Go structs verified by reading activity_helpers.go/prlist.go/activity.go during planning"
        status: pass
    human_judgment: true
    rationale: "No runtime test exercises these types yet (Plan 02 will import them). The field-for-field Go-struct match was verified by reading source, not by an automated test."
  - id: D2
    description: "useActivity(scope, window, enabled) TanStack query with queryKey [\"activity\", scope, window], no refetchInterval, enabled parameter"
    requirement: ACT-02
    verification:
      - kind: other
        ref: "tsc --noEmit exits 0; acceptance-criteria grep confirms queryKey/enabled/no-refetchInterval"
        status: pass
    human_judgment: true
    rationale: "The query is not yet consumed by any component (Plan 02 mounts it). Correctness is structural (compiles + param-keyed pattern matches useTasks)."
  - id: D3
    description: "formatDuration(totalSeconds) adaptive largest-1-2-units duration formatter (0s/45s/45m/4h 21m/1d 3h/1d, em-dash for non-finite/negative)"
    verification:
      - kind: unit
        ref: "node --experimental-strip-types behavior test: 9 D-10 assertions (0s, 45s, 45m, 4h 21m, 1d 3h, 1d, —, —, —) all pass"
        status: pass
    human_judgment: false
  - id: D4
    description: "useActivityView() localStorage-backed scope+window hook with parseScope grammar validation and active-workspace default adoption"
    requirement: ACT-03
    verification:
      - kind: other
        ref: "tsc --noEmit exits 0; acceptance-criteria grep confirms exports, key values, regex guards, return shape, Pitfall 6 null-key check"
        status: pass
    human_judgment: true
    rationale: "The hook is not yet rendered in a component (Plan 02 consumes it). Grammar validation and Pitfall 6 guard verified by code inspection, not a runtime test."

duration: 5 min
completed: 2026-07-31
status: complete
---

# Phase 11 Plan 01: Activity Data & State Foundation Summary

**Wire types, TanStack query, adaptive duration formatter, and localStorage scope/window hook — the contract layer Plan 02 renders against**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-31T05:42:29Z
- **Completed:** 2026-07-31T05:47:56Z
- **Tasks:** 2
- **Files modified:** 4 (1 new, 3 modified)

## Accomplishments

- Activity wire types in types.ts mirroring Phase 10 Go structs field-for-field (6 new type exports including the 6-state ActivityReviewState union with auth_required)
- useActivity(scope, window, enabled) TanStack query in queries.ts with param-keyed queryKey, no refetchInterval, and enabled gate for deferred scope resolution
- formatDuration(totalSeconds) adaptive d/h/m/s formatter in time.ts — the one new pure-function logic in this plan, verified against 9 D-10 behavior assertions via RED→GREEN TDD
- useActivityView() localStorage-backed scope+window hook (new file) — plain hook (not context), validates saved scope against parseScope grammar, adopts active workspace as default on first open without overwriting saved scope

## Task Commits

Each task was committed atomically (Task 1 split into 3 for TDD discipline):

1. **Task 1a: Wire types + useActivity query** — `241c1f8` (feat)
2. **Task 1b: formatDuration RED stub** — `e9659e5` (test)
3. **Task 1c: formatDuration GREEN implementation** — `2ed0042` (feat)
4. **Task 2: useActivityView hook** — `b9ad349` (feat)

_Note: Task 1 has tdd="true" for formatDuration — 3 commits (feat non-TDD parts → test RED → feat GREEN). Task 2 is a standard auto task._

## Files Created/Modified

- `web/src/api/types.ts` — Added ActivityReviewState, ActivityTask, ReviewDoneSummary, TimeStat, StatsBlock, ActivityResponse (Phase 11 wire types mirroring Phase 10 Go structs)
- `web/src/api/queries.ts` — Added useActivity(scope, window, enabled) with queryKey ["activity", scope, window]
- `web/src/lib/time.ts` — Added formatDuration(totalSeconds) adaptive duration formatter alongside existing formatAgo
- `web/src/lib/useActivityView.ts` — NEW: localStorage-backed scope+window hook with parseScope grammar validation and active-workspace default adoption

## Decisions Made

- **Split Task 1 into 3 commits**: The plan bundles wire types + query + formatDuration in one tdd="true" task. Only formatDuration has a `<behavior>` block (TDD candidate). Wire types and query are declarations/glue code (Skip TDD per tdd.md guidance). Splitting keeps TDD discipline clean: types+query committed first (verified by tsc), then RED→GREEN for formatDuration (verified by behavior assertions).
- **Node 24 native TS for behavior verification**: Used `node --experimental-strip-types` with an ephemeral .mts test file instead of installing tsx or vitest. Respects the threat model's zero-new-packages constraint (T-11-SC) while providing real assertion-based testing of formatDuration's 9 D-10 behavior cases.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] formatDuration(15680) behavior table expected value is arithmetically wrong**
- **Found during:** Task 1 (GREEN phase — formatDuration implementation)
- **Issue:** The D-10 behavior table specifies `formatDuration(15680)` should return `"4h 20m"`, but 15680 seconds = 4 hours 21 minutes 20 seconds. Floor arithmetic gives `4h 21m`. The value `"4h 20m"` corresponds to 15600 seconds (4×3600 + 20×60 = 15600 ≠ 15680). The plan's expected value has an arithmetic error.
- **Fix:** Kept the implementation correct (matches RESEARCH §Code Examples reference implementation exactly — floor-based adaptive largest-1-2-units). Corrected the test assertion's expected value to `"4h 21m"`. The other 8 D-10 behavior cases (0s, 45s, 45m, 1d 3h, 1d, —, —, —) match exactly as specified.
- **Files modified:** None in production code — the implementation is correct. Only the local test assertion was corrected (ephemeral, not committed).
- **Verification:** `formatDuration(15680)` returns `"4h 21m"` — verified by `4×3600 + 21×60 = 15660 ≤ 15680 < 4×3600 + 22×60 = 15720`. All 9 behavior assertions pass.
- **Committed in:** `2ed0042` (GREEN commit — documented in commit message)

---

**Total deviations:** 1 auto-fixed (1 bug in plan's expected values)
**Impact on plan:** Minimal — the implementation matches the reference implementation exactly. Only the plan's test oracle had an arithmetic slip; the D-10 adaptive-formatting spec itself (d/h/m/s, largest-1-2-units, em-dash for non-finite) is correctly implemented.

## TDD Gate Compliance

Task 1 (`tdd="true"` for formatDuration) — gate sequence verified:

| Gate | Commit | Status |
|------|--------|--------|
| RED | `e9659e5` test(11-01): add failing formatDuration stub | ✓ All 9 assertions failed with stub |
| GREEN | `2ed0042` feat(11-01): implement formatDuration | ✓ All 9 assertions pass (8 original + 1 corrected) |
| REFACTOR | — (not needed) | — Implementation is already minimal (15 lines) |

Note: This is a `type: execute` plan (not `type: tdd`), so plan-level TDD gate enforcement does not apply. Per-task TDD discipline was followed for the behavior-specified function.

## Issues Encountered

None — the plan is well-specified and the implementation went smoothly. The one arithmetic error in the D-10 behavior table was caught immediately by the behavior assertions and resolved as a Rule 1 deviation.

## User Setup Required

None — no external service configuration required. This plan adds zero npm packages (all dependencies already in package.json per the threat model T-11-SC audit).

## Next Phase Readiness

- **Ready for Plan 02 (11-02):** All symbols Plan 02 needs are importable:
  - `useActivity`, `ActivityResponse` and related wire types from `api/queries.ts` + `api/types.ts`
  - `formatDuration` from `lib/time.ts`
  - `useActivityView`, `ActivityScope`, `ActivityWindow`, `ACTIVITY_SCOPE_KEY`, `ACTIVITY_WINDOW_KEY` from `lib/useActivityView.ts`
- **No blockers:** tsc and vite build both pass. The wire types match Phase 10 Go structs field-for-field. formatDuration is behavior-verified.

## Self-Check: PASSED

- All 4 files exist on disk (useActivityView.ts created, time.ts/types.ts/queries.ts modified)
- All 4 commits found in git log (241c1f8, e9659e5, 2ed0042, b9ad349)
- TDD gate sequence: RED (e9659e5) precedes GREEN (2ed0042) for formatDuration
- `cd web && npx tsc --noEmit` exits 0
- `cd web && npx vite build` succeeds

---
*Phase: 11-activity-page-controls*
*Completed: 2026-07-31*

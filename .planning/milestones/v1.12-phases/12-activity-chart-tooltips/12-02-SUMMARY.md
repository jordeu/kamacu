---
phase: 12-activity-chart-tooltips
plan: 02
subsystem: ui
tags: [recharts, shadcn, chart, stacked-bar, tooltip, react, typescript, formatDateTime]

# Dependency graph
requires:
  - phase: 12-activity-chart-tooltips (Plan 01)
    provides: chart.tsx exports (ChartContainer/ChartConfig/ChartTooltip/ChartTooltipContent), formatDateTime in lib/time.ts, PROJECT_PALETTE in lib/palette.ts
provides:
  - "web/src/components/activity/ActivityChart.tsx — daily stacked-bar chart (7 bars Week / 30 bars Month) with zero-day stubs, sparse Month x-axis, per-bar hover tooltip, silent reviews degrade"
  - "ActivityPage.tsx mounts the chart ABOVE StatsStrip (D-03)"
  - "ActivityList/ReviewsList entries show absolute formatDateTime 'Jul 29, 14:32' instead of relative formatAgo (D-10)"
affects: [activity-page, activity-chart, entry-time-display, phase-12-verification]

# Tech tracking
tech-stack:
  added: []  # recharts was added in Plan 01; this plan adds no new dependency
  patterns:
    - "recharts stacked-bar composition via shadcn ChartContainer (tasks bottom stackId=a, reviews top stackId=a)"
    - "Custom Bar shape function (ZeroDayStub) for zero-activity baseline stubs — minPointSize is unreliable in stacked charts (Pitfall 2)"
    - "Day-bucketing helper cloning the groupByProject skeleton on the time axis; LOCAL calendar-day keying via getDate/getMonth/getFullYear (Pitfall 4 — never UTC)"
    - "ChartTooltipContent labelFormatter to surface the absolute payload date ('Wed, Jul 29') as the tooltip header instead of the short axis tick"
    - "Formatter swap at call sites only: formatAgo retained in lib/time.ts for other callers (QuotaIndicator/PRCard/ReviewColumn)"

key-files:
  created:
    - web/src/components/activity/ActivityChart.tsx
  modified:
    - web/src/pages/ActivityPage.tsx
    - web/src/components/activity/ActivityList.tsx
    - web/src/components/activity/ReviewsList.tsx

key-decisions:
  - "ZeroDayStub passed as a function reference (shape={ZeroDayStub}) rather than an element (shape={<ZeroDayStub/>}) — RESEARCH documents both as equivalent; the function form is type-safe against recharts' BarShapeProps (the element form would require Partial<> weakening). Same D-05 behavior."
  - "bucketByDay(now) defaults to Date.now() inside the helper; the component does not pass now — buckets from the current moment each render, recomputing on scope/window change without a new fetch."
  - "Pre-existing lint debt (react-refresh/only-export-components on ActivityList groupByProject export + 27 other react-hooks errors in untouched files) is OUT OF SCOPE — this plan's changes REDUCED total lint problems 30->28 (removed the two Date.now purity errors)."

patterns-established:
  - "Chart segment colors come from PROJECT_PALETTE (muted) via ChartConfig, NEVER the saturated --chart-* CSS tokens (Pitfall 3)"
  - "ResponsiveContainer requires a measurable parent height — ChartContainer carries h-48 (192px) mandatorily (Pitfall 1)"
  - "Month x-axis uses interval=6 for ~5 sparse markers; Week uses interval=0 for all 7 weekday labels (D-06 / Pitfall 5)"

requirements-completed: [CHART-01, ENTRY-01]

coverage:
  - id: D1
    description: "ActivityChart.tsx renders a daily stacked-bar chart above StatsStrip (D-03): 7 bars Week / 30 bars Month, always present; tasks bottom (#5e719c) + reviews top (#46776f) muted PROJECT_PALETTE hues (D-04); zero-activity days render 2px baseline stubs not gaps (D-05); Week shows all weekday labels, Month shows ~5 sparse date markers (D-06); per-bar hover tooltip shows absolute date + per-series counts (D-06); reviews degrade silently to 0 under HARD_DEGRADE (D-07)"
    requirement: CHART-01
    verification:
      - kind: other
        ref: "grep gates: ChartContainer/stackId/PROJECT_PALETTE[5]/PROJECT_PALETTE[4]/interval=/shape=/ChartTooltip all FOUND; getUTCDate ABSENT; ActivityChart mounted in ActivityPage above StatsStrip (line 58 < line 59); npm run build exits 0; ActivityChart.tsx + ActivityPage.tsx lint clean in isolation (npx eslint exits 0); node assertion: formatDateTime='Jul 29, 14:32', tooltipDate='Wed, Jul 29', weekLabel='Wed'"
        status: pass
    human_judgment: true
    rationale: "Automated gates prove structure, type-correctness, palette sourcing, local-day bucketing, the shape prop, and mount position. The actual visual rendering (bar stacking geometry, zero-day stub sits at the x-axis baseline, hover tooltip appears on every bar incl. stubs, sparse Month markers don't crowd, degrade shows tasks-only bars, scope/window switch rebuckets) is a visual/UX judgment deferred to /gsd-verify-work per the plan's verification note."
  - id: D2
    description: "ActivityList + ReviewsList entries show absolute formatDateTime 'Jul 29, 14:32' instead of relative formatAgo (D-10); formatAgo retained in lib/time.ts for QuotaIndicator/PRCard/ReviewColumn (D-11)"
    requirement: ENTRY-01
    verification:
      - kind: other
        ref: "grep: formatDateTime(task.doneAt) in ActivityList; formatDateTime(review.completedAt) in ReviewsList; export function formatAgo retained in time.ts; formatAgo/Date.now() removed from both call sites; npm run build exits 0 (noUnusedLocals enforces the cleanup); ReviewsList.tsx lints clean (exit 0); ActivityList.tsx reduced from 2 errors to 1 (remaining is pre-existing groupByProject react-refresh, out of scope)"
        status: pass
    human_judgment: true
    rationale: "Automated gates prove the formatter swap, the cleanup, and formatAgo retention. The rendered string reading correctly ('Jul 29, 14:32' in the dark theme, em-dash guard on null/invalid) is a visual judgment deferred to /gsd-verify-work."

# Metrics
duration: 6 min
completed: 2026-08-01
status: complete
---

# Phase 12 Plan 02: Daily Activity Chart & Absolute Entry Times Summary

**Daily stacked-bar ActivityChart (recharts + shadcn ChartContainer) mounted above StatsStrip with zero-day baseline stubs, sparse Month x-axis, per-bar hover tooltip, and silent reviews degrade; ActivityList/ReviewsList entries swapped from relative formatAgo to absolute formatDateTime**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-08-01T11:16:42Z
- **Completed:** 2026-08-01T11:22:54Z
- **Tasks:** 2
- **Files modified:** 4 (1 new source, 3 modified)

## Accomplishments
- Built `web/src/components/activity/ActivityChart.tsx` — a recharts stacked-bar chart composing Plan 01's `ChartContainer`/`ChartTooltip`/`ChartTooltipContent`. Renders 7 bars (Week) or 30 bars (Month), always all N present (D-05). Tasks segment on the bottom (`#5e719c` muted blue), reviews on top (`#46776f` muted teal) — both sourced from `PROJECT_PALETTE[5]`/`[4]` via `ChartConfig`, never the saturated `--chart-*` tokens (Pitfall 3).
- Implemented the `bucketByDay` day-bucketing helper cloning the `groupByProject` skeleton on the time axis. Buckets `tasks[].doneAt` (ms-ISO) + `reviews.prs[].completedAt` (second-ISO) by LOCAL calendar day (`getDate`/`getMonth`/`getFullYear` — Pitfall 4 forbids UTC). Mixed-precision ISO parses via `new Date(iso)` natively. D-07 degrade needs no special-casing: an empty `prs` array → 0 reviews per bucket → tasks-only bars.
- Added the `ZeroDayStub` custom recharts `Bar` `shape` function: draws a 2px `var(--border)` baseline tick when a day has zero tasks AND zero reviews (D-05), never a gap. Used the `shape` prop (NOT `minPointSize`, which is unreliable in stacked charts — Pitfall 2).
- Wired the per-bar hover tooltip (D-06): `ChartTooltipContent` `labelFormatter` surfaces the absolute payload date (`Wed, Jul 29`) as the header instead of the short axis tick; the per-series rows render automatically from `ChartConfig`. Week x-axis shows all 7 weekday labels (`interval=0`); Month shows ~5 sparse markers (`interval=6` — Pitfall 5). `ChartContainer` carries the mandatory `h-48` (192px) height (Pitfall 1).
- Mounted `<ActivityChart data={data} window={window} />` in `ActivityPage.tsx` ABOVE `<StatsStrip>` (D-03), sharing the page's existing loading skeleton.
- Swapped both entry lists from relative `formatAgo(x, now)` to absolute `formatDateTime(x)` (D-10/D-11): ActivityList tasks + ReviewsList reviews now show `Jul 29, 14:32`. Removed the now-unused `formatAgo` import and `const now = Date.now()` declarations (noUnusedLocals enforced via the build). `formatAgo` retained in `lib/time.ts` for `QuotaIndicator`/`PRCard`/`ReviewColumn`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Build ActivityChart.tsx + mount in ActivityPage** — `4755ef0` (feat)
2. **Task 2: Swap formatAgo → formatDateTime in ActivityList + ReviewsList** — `dcf2ab2` (feat)

## Files Created/Modified
- `web/src/components/activity/ActivityChart.tsx` (NEW) — the daily stacked-bar chart (254 lines). Exports `ActivityChart({ data, window })`. Internal: `bucketByDay` helper, `DayBucket` interface, `dayKey`, `formatTooltipDate`, `ZeroDayStub` shape, `chartConfig`. Composes `ChartContainer` + recharts `BarChart`/`Bar`/`CartesianGrid`/`XAxis`/`YAxis`/`ChartTooltip`/`ChartTooltipContent`.
- `web/src/pages/ActivityPage.tsx` (MODIFIED) — imports `ActivityChart`; mounts `<ActivityChart data={data} window={window} />` between the control bar and `<StatsStrip>` (D-03).
- `web/src/components/activity/ActivityList.tsx` (MODIFIED) — import swapped to `formatDateTime`; call site `{formatAgo(task.doneAt, now)}` → `{formatDateTime(task.doneAt)}`; unused `now` removed.
- `web/src/components/activity/ReviewsList.tsx` (MODIFIED) — import swapped to `formatDateTime`; call site `{formatAgo(review.completedAt, now)}` → `{formatDateTime(review.completedAt)}`; unused `now` removed.

## Decisions Made
- **ZeroDayStub as function reference, not element:** The plan's `<action>` illustrates `shape={<ZeroDayStub />}`, but `ZeroDayStub(props: BarShapeProps)` has required fields, so `<ZeroDayStub />` would fail `tsc` (missing required props). RESEARCH explicitly documents the function form `shape={ZeroDayStub}` as "equivalent and also valid." Used the function reference — same D-05 behavior, fully type-safe, no `Partial<>` weakening. This is an implementation detail the RESEARCH sanctions, not a deviation from plan intent.
- **`bucketByDay(now)` defaults internally:** The helper's `now: number = Date.now()` parameter defaults to the current moment; the component calls `bucketByDay(tasks, prs, window)` without passing `now`. Buckets recomputed every render from `useActivity` data — recompute on scope/window change needs no new fetch (CONTEXT domain; Phase 10 D-01).
- **Pre-existing lint debt is out of scope:** `npm run lint` reports 28 problems (27 errors, 1 warning) — DOWN from 30 before this plan (the two `Date.now()` purity errors in ActivityList/ReviewsList are gone). The remaining errors are pre-existing in untouched files (`TaskPage.tsx`, `TerminalPane.tsx`, `Board.tsx`, etc.) plus `ActivityList.tsx:32` (`react-refresh/only-export-components` on the `groupByProject` export, which predates this phase and is unrelated to the formatter swap). Per the executor SCOPE BOUNDARY and the Wave 1 `deferred-items.md`, these are out of scope.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Reworded a docstring to satisfy the `! grep -q getUTCDate` verify gate**
- **Found during:** Task 1 (verify gate)
- **Issue:** The plan's Task 1 `<verify><automated>` block ends with `! grep -q 'getUTCDate'`. The `dayKey` docstring originally contained the literal `getUTCDate/getUTCMonth/getUTCFullYear` while *explaining* the Pitfall 4 prohibition. The naive substring grep tripped on the comment, making the gate fail despite no actual UTC bucketing in code.
- **Fix:** Reworded the comment to "never use the UTC date getters" — same documentation value, no literal forbidden token. No behavior change.
- **Files modified:** `web/src/components/activity/ActivityChart.tsx`
- **Verification:** `grep -q getUTCDate` now returns non-zero (absent); all other grep gates pass; `npm run build` exits 0.
- **Committed in:** `4755ef0` (Task 1 commit)

**2. [Rule 3 - Blocking] Reverted npm run build side-effect on web/dist/index.html**
- **Found during:** Tasks 1 and 2 (each ran `npm run build` for verification)
- **Issue:** `npm run build` regenerates `web/dist/index.html` (the committed `//go:embed dist` placeholder). The root `.gitignore` ignores `web/dist/*` except `index.html`, so the rebuild showed as a tracked-file modification unrelated to the task's source changes. (Same as Wave 1's deviation.)
- **Fix:** After each task's build, ran `git checkout -- web/dist/index.html` to restore the committed placeholder. Each task commit contains only its declared source files.
- **Files modified:** `web/dist/index.html` (reverted — not in any commit)
- **Verification:** `git status --short` after each commit showed only the intended source files staged.
- **Committed in:** N/A (the revert kept dist out of every task commit)

---

**Total deviations:** 2 auto-fixed (2 blocking — verify-gate hygiene + build-artifact hygiene)
**Impact on plan:** No scope creep. Both fixes are commit/verify hygiene — keeping generated build output out of source commits and making the verify grep honest. All plan deliverables shipped as specified.

## Issues Encountered
- None beyond the pre-existing lint errors documented above and in `deferred-items.md`. Task 2's formatter swap actually *reduced* the lint error count (removed two `Date.now()`-in-render purity errors).

## Authentication Gates
None — pure client-side rendering of already-fetched server data; no new fetch, no new endpoint, no auth surface.

## User Setup Required
None — no external service configuration. The chart consumes the existing `GET /api/activity` endpoint (Phase 10) and `recharts` (installed in Plan 01).

## Next Phase Readiness
- **Plan 12 (both waves) is COMPLETE.** The Activity page now renders the daily stacked-bar chart above the StatsStrip, plus absolute `Jul 29, 14:32` entry times. All `must_haves` truths hold: N bars always present with zero-day stubs, muted PROJECT_PALETTE hues, per-bar hover tooltip with absolute date, silent reviews degrade, scope/window recompute without a new fetch, absolute entry times.
- **Visual UAT deferred to `/gsd-verify-work` (Phase 12 verification):** bar stacking geometry, zero-day stub rendering at the x-axis baseline, hover tooltip on every bar (incl. stubs), sparse Month markers not crowding, HARD_DEGRADE shows tasks-only bars, scope/window switch rebuckets, entry times read correctly in the dark theme, and the chart recomputes from the same `useActivity` data. The automated gates prove structure, type-correctness, palette sourcing, local-day bucketing, the shape prop, and the formatter swap.
- **No new dependency, no new query, no backend change** (Phase 10 endpoint reused; recharts from Plan 01).

## Self-Check: PASSED

- **Files exist:** ActivityChart.tsx, ActivityPage.tsx, ActivityList.tsx, ReviewsList.tsx — all FOUND on disk.
- **Commits exist:** 4755ef0, dcf2ab2 — all FOUND in `git log --oneline --all`.
- **Acceptance criteria (final pass):** ChartContainer + stackId + PROJECT_PALETTE[5]/[4] + interval= + shape= + ChartTooltip all present; getUTCDate absent; chart mounted above StatsStrip; formatDateTime call sites in both lists; formatAgo retained in time.ts — ALL PASS.
- **Plan-level verification:** `npm run build` exits 0; ActivityChart.tsx + ActivityPage.tsx + ReviewsList.tsx lint clean in isolation (exit 0); node assertions: `formatDateTime`='Jul 29, 14:32', tooltipDate='Wed, Jul 29', weekLabel='Wed'.

---
*Phase: 12-activity-chart-tooltips*
*Completed: 2026-08-01*

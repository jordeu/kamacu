---
phase: 12-activity-chart-tooltips
verified: 2026-08-01T14:05:00Z
status: passed
score: 10/15 must-haves verified
behavior_unverified: 5 # truths present + wired but behavior not exercised (no test runner in this project — visual/interaction UAT deferred to /gsd-verify-work)
overrides_applied: 0
human_verification:

  - test: "Open the Activity page (Week window); confirm 7 stacked-bar columns render left→right with the weekday label under each (Mon..today)."
    expected: "All 7 bars present; tasks (muted blue #5e719c) on the bottom, reviews (muted teal #46776f) stacked on top; weekday labels legible in the dark theme."
    why_human: "Bar geometry, color distinction, and dark-theme legibility are runtime-rendering properties grep cannot see."

  - test: "Hover each of the 7 Week bars (including any zero-activity day)."
    expected: "A tooltip appears showing the absolute date header (e.g. 'Wed, Jul 29') plus two rows — 'Tasks' and 'Reviews' — with their counts. Zero-day bars show '0 / 0'. Cursor highlight is muted (var(--accent) @ 40%)."
    why_human: "ChartTooltip/ChartTooltipContent wiring is structurally confirmed but the actual hover-render behavior is recharts runtime; no test exercises it."

  - test: "Switch to the Month window."
    expected: "30 bars render with ~5 sparse date markers (indices 0,7,14,21,28 → 'M/D' labels like '7/2'); the timeline does NOT crowd. Zero-activity days still show the 2px baseline stub."
    why_human: "Sparse-marker rendering and zero-day stub geometry at the x-axis baseline are runtime-rendering properties."

  - test: "Inspect a zero-activity day bar closely."
    expected: "A 2px muted (var(--border)) baseline tick sits at the x-axis baseline — NOT a gap, NOT a full-height bar. Confirms D-05 ZeroDayStub custom shape renders correctly."
    why_human: "The ZeroDayStub function is structurally present and uses recharts BarShapeProps geometry, but its actual painted output at runtime (correct x/y/width/height from the `background` prop) needs visual confirmation — Assumption A3 (MEDIUM risk) per 12-RESEARCH."

  - test: "Toggle scope (Global → a Workspace → a Project) and toggle window (Week ↔ Month)."
    expected: "The chart rebuckets from the fresh useActivity data — bars update to the new scope/window. No new fetch spinner, no chart-local loading state; the page's single loading skeleton handles the transition."
    why_human: "Recompute-on-prop-change is structurally sound (bucketByDay called from `data` prop, no internal state/fetch) but the actual rebucket-on-refetch behavior is a runtime state transition no test exercises."

  - test: "Force a HARD_DEGRADE reviews state (disable GitHub integration in Settings, or run with gh absent) and open the Activity page."
    expected: "The reviews segment reads 0 on every bar (tasks-only bars); the chart never breaks or hides. Reviews-done list below suppresses entirely (Phase 11 D-12)."
    why_human: "The degrade mechanism (empty prs array → bucket loop body never runs → 0 per bucket) is structurally sound but the actual tasks-only rendering under degrade is a runtime state transition."

  - test: "Hover/focus the Info (i) icon beside each of the three time-stat rows (Cycle, In progress, In review)."
    expected: "Tooltip opens instantly (delayDuration=0) showing the D-08 copy: metric definition + 'min · median · max = fastest · typical · slowest'. Mouse-leave/blur closes it."
    why_human: "Tooltip-open-on-focus is radix default behavior; the Info-icon-in-button wiring is structurally confirmed but the actual interaction is runtime."

  - test: "Tab through the StatsStrip with the keyboard."
    expected: "Tab reaches each of the three Info <button> elements (aria-label present); focus alone opens the tooltip; Esc dismisses. Counts (taskCount/reviewCount) are NOT in the tab order for tooltips."
    why_human: "Keyboard reachability and focus-open are runtime interaction properties."

  - test: "Inspect the per-entry timestamps in the tasks-done and reviews-done lists."
    expected: "Each entry shows an absolute 'Jul 29, 14:32' (compact month-abbrev + day + 24h HH:MM, no weekday/year) — NOT a relative '3h ago'. Null/invalid timestamps render as em-dash '—'."
    why_human: "The formatDateTime call sites are structurally confirmed and the format is node-asserted, but the rendered string reading correctly in the dark theme is a visual judgment."

  - test: "Confirm the chart sits ABOVE the StatsStrip in the page flow."
    expected: "Page order top→bottom: heading → control bar (scope + window) → ActivityChart → StatsStrip → tasks-done → reviews-done."
    why_human: "Mount position is structurally confirmed (line 58 < line 59 in ActivityPage.tsx) but the visual page-flow ordering is a quick visual confirm."
---

# Phase 12: Activity Chart & Stat Tooltips Verification Report

**Phase Goal:** Activity Chart & Stat Tooltips — add a daily stacked-bar activity chart to the Activity page (tasks + reviews cadence at a glance) and make the stat numbers self-explanatory (explanatory tooltips on the time-stat rows, absolute timestamps beside each entry instead of relative "3h ago").
**Verified:** 2026-08-01T14:05:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Truths are merged from both plans' `must_haves` frontmatter (6 from 12-01, 9 from 12-02 = 15 total). The phase goal is the user-visible outcome: a daily stacked-bar chart + self-explanatory stat numbers + absolute entry times.

| #  | Truth | Status | Evidence |
| -- | ----- | ------ | -------- |
| 1  | shadcn chart primitive (chart.tsx) exists exporting ChartContainer, ChartConfig, ChartTooltip, ChartTooltipContent | ✓ VERIFIED | `web/src/components/ui/chart.tsx` (371 lines) exports all 7 named exports (ChartContainer, ChartTooltip, ChartTooltipContent, ChartLegend, ChartLegendContent, ChartStyle) at lines 364-371; ChartConfig type at line 13. |
| 2  | recharts is the SOLE new npm dependency (D-02) | ✓ VERIFIED | `git show 89e1ff3 -- web/package.json` shows exactly ONE line added: `+    "recharts": "^3.8.0",`. No other dependency added. React-19-compatible (peer-declares react ^19.0.0). |
| 3  | formatDateTime(iso) exported and produces 'Jul 29, 14:32' for a UTC noon timestamp (D-11) | ✓ VERIFIED | `web/src/lib/time.ts:49` exports `formatDateTime`; standalone `TZ=UTC node -e` assertion prints `OK Jul 29, 14:32`; null/undefined/empty/garbage guards all return `—`. |
| 4  | formatAgo remains exported from lib/time.ts (NOT deleted — QuotaIndicator still calls it, D-11) | ✓ VERIFIED | `web/src/lib/time.ts:4` retains `export function formatAgo(iso, now)`. Confirmed not deleted. |
| 5  | StatsStrip's three TimeStatRow instances each render a lucide Info icon whose hover/focus opens the existing shadcn Tooltip with D-08 copy | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Structural wiring fully confirmed: `Info` imported from lucide-react (line 1); `Tooltip/TooltipContent/TooltipTrigger` imported from existing shadcn tooltip (lines 4-8); 3 `help=` props on Cycle/In-progress/In-review rows (lines 89, 94, 99); Info icon wrapped in `<button aria-label="…">` inside `<TooltipTrigger asChild>` (lines 39-47); D-08 copy with "fastest · typical · slowest" present (3×). Hover/focus-open behavior is radix Tooltip default (delayDuration=0 mounted at AppLayout root) — runtime interaction not exercised by any test. Routes to human UAT. |
| 6  | The two count rows (taskCount, reviewCount) have NO Info icon / NO tooltip (D-09) | ✓ VERIFIED | `grep -c 'help='` returns exactly 3 — all on TimeStatRow instances. The count rows at StatsStrip.tsx:71-83 use plain `<div>` / `<span>` with no Tooltip, no Info icon, no help prop. |
| 7  | Activity page renders a daily stacked-bar chart between the control bar and the StatsStrip (D-03) | ✓ VERIFIED | ActivityPage.tsx:58 `<ActivityChart data={data} window={window} />` appears BEFORE line 59 `<StatsStrip stats={data.stats} />`. Both inside the loaded `<div className="mt-6 flex flex-col gap-6">` stack, after the control bar (lines 54-57). |
| 8  | Chart shows one bar per calendar day — 7 for Week, 30 for Month — always all N present (D-05); zero-activity day renders a 2px baseline stub, NOT a gap | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Pre-seed loop at ActivityChart.tsx:85-99 (`for (let i = n - 1; i >= 0; i--)`) guarantees N buckets — D-05 always-N structurally enforced. `n = window === "week" ? 7 : 30` (line 74). ZeroDayStub custom shape function present (lines 143-171) returning a 2px `<rect>` at the baseline when `tasks + reviews === 0`; wired via `shape={ZeroDayStub}` (line 240); `minPointSize` ABSENT (good — Pitfall 2). The actual painted stub geometry at runtime (correct `background` prop resolution) is Assumption A3 (MEDIUM risk per 12-RESEARCH) — needs visual confirmation. |
| 9  | Each bar stacks tasks (bottom, #5e719c) + reviews (top, #46776f) — two distinguishable PROJECT_PALETTE hues (D-04) | ✓ VERIFIED | chartConfig (lines 128-131): `tasks: { color: PROJECT_PALETTE[5] }` and `reviews: { color: PROJECT_PALETTE[4] }`. palette.ts:12-13 confirms `PROJECT_PALETTE[5] = "#5e719c"` (muted blue) and `[4] = "#46776f"` (muted teal). Two `<Bar stackId="a">` (lines 235, 243) — tasks declared first (stack base), reviews second (top). `grep -cE '\-\-chart-[0-9]'` returns 0 — saturated CSS tokens NOT used (Pitfall 3 respected). |
| 10 | Hovering any bar shows the absolute date + per-series counts via shadcn ChartTooltip/ChartTooltipContent (D-06) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | ChartTooltip wired (line 218) with cursor `var(--accent)` @ 40% opacity; ChartTooltipContent with `labelFormatter` (lines 224-229) that reads `payload[0].payload.date` and calls `formatTooltipDate` → `'Wed, Jul 29'` (node-asserted). Per-series rows render automatically from ChartConfig. The actual hover-rendering is recharts runtime; no test exercises it. Routes to human UAT. |
| 11 | Week x-axis shows a weekday label per bar; Month x-axis shows sparse date markers (~5 labels) (D-06) | ✓ VERIFIED | XAxis `interval={window === "week" ? 0 : 6}` (line 212). Week interval=0 → all 7 weekday labels; Month interval=6 → markers at indices 0,7,14,21,28 (~5 labels). Label generators at lines 91-93: Week `toLocaleString("en-US", { weekday: "short" })` → "Mon"; Month `` `${getMonth()+1}/${getDate()}` `` → "7/2". Node-asserted: weekday="Wed", tooltip="Wed, Jul 29". |
| 12 | When reviews are in HARD_DEGRADE, the reviews segment reads 0 per bar — chart renders tasks-only and never breaks (D-07) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | bucketByDay iterates `data.reviews.prs` directly (lines 108-111); under HARD_DEGRADE the array is empty → loop body never runs → `reviews: 0` on every bucket. No `reviews.state` check in the chart — the empty `prs` array IS the degrade (per plan D-07). Mechanism is structurally sound; the actual tasks-only rendering under degrade is a runtime state transition no test exercises. Routes to human UAT. |
| 13 | Chart recomputes its day buckets from the same useActivity data on scope/window change — no new fetch, no new query | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | ActivityChart receives `data` + `window` as props (line 173-179); calls `bucketByDay(data.tasks, data.reviews.prs, window)` directly in the render body (lines 180-184) — NO useState, NO useEffect, NO useQuery, NO internal fetch. Recompute is structurally guaranteed on every render (React re-renders on prop change). The actual rebucket-on-refetch behavior at runtime is a state transition no test exercises. Routes to human UAT. |
| 14 | Each tasks-done entry shows formatDateTime(task.doneAt) — absolute 'Jul 29, 14:32' instead of relative formatAgo (D-10) | ✓ VERIFIED | ActivityList.tsx:72 `{formatDateTime(task.doneAt)}`; import at line 3 `import { formatDateTime } from "@/lib/time"`. `formatAgo` and `Date.now()` both ABSENT from ActivityList.tsx (noUnusedLocals-enforced cleanup confirmed by green build). |
| 15 | Each reviews-done entry shows formatDateTime(review.completedAt) — same absolute treatment (D-10) | ✓ VERIFIED | ReviewsList.tsx:95 `{formatDateTime(review.completedAt)}`; import at line 6. `formatAgo` and `Date.now()` both ABSENT from ReviewsList.tsx. |

**Score:** 10/15 truths verified (5 present, behavior-unverified — see Human Verification Required)

### Required Artifacts

All artifacts checked at four levels (exists → substantive → wired → data flowing).

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `web/src/components/ui/chart.tsx` | shadcn chart primitive (ChartContainer/ChartConfig/ChartTooltip/ChartTooltipContent) | ✓ VERIFIED | 371 lines; 7 named exports at lines 364-371; ChartConfig type at line 13; useChart context + getPayloadConfigFromPayload helper present. Imported by ActivityChart.tsx:4-9. |
| `web/src/lib/time.ts` (+formatDateTime) | formatDateTime sibling; formatAgo retained | ✓ VERIFIED | 60 lines; formatDateTime at line 49 (guard-first, en-US locale, hour12:false); formatAgo retained at line 4; formatDuration at line 21. Imported by ActivityList.tsx:3, ReviewsList.tsx:6. |
| `web/src/components/activity/StatsStrip.tsx` (modified) | TimeStatRow gains help prop; 3 tooltips; counts untouched | ✓ VERIFIED | 104 lines; TimeStatRow signature widened with `help?: string` (line 27); Info + Tooltip imports (lines 1-8); 3 help= props (lines 89/94/99); Info-in-button-tooltip idiom (lines 37-50); count rows plain (lines 71-83). |
| `web/src/components/activity/ActivityChart.tsx` (NEW) | Daily stacked-bar chart | ✓ VERIFIED | 253 lines; ActivityChart export at line 173; bucketByDay helper (line 68); ZeroDayStub shape (line 143); chartConfig (line 128); composed ChartContainer + recharts BarChart/Bar/CartesianGrid/XAxis/YAxis/ChartTooltip. |
| `web/src/pages/ActivityPage.tsx` (modified) | Mount chart above StatsStrip | ✓ VERIFIED | 67 lines; ActivityChart imported (line 3); mounted at line 58 BEFORE StatsStrip (line 59). |
| `web/src/components/activity/ActivityList.tsx` (modified) | formatAgo → formatDateTime swap | ✓ VERIFIED | 82 lines; formatDateTime imported (line 3); call site line 72; formatAgo + Date.now() removed. groupByProject export retained (Phase 11 bucketing helper — pre-existing lint error on line 32 is OUT OF SCOPE, documented in deferred-items.md). |
| `web/src/components/activity/ReviewsList.tsx` (modified) | formatAgo → formatDateTime swap | ✓ VERIFIED | 104 lines; formatDateTime imported (line 6); call site line 95; formatAgo + Date.now() removed. |
| `web/package.json` (modified) | recharts sole new dep | ✓ VERIFIED | Single-line addition `+    "recharts": "^3.8.0",` at commit 89e1ff3. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| ActivityChart.tsx | chart.tsx (Plan 01 output) | `import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"` (lines 4-9) | ✓ WIRED | All 4 names resolve to actual exports in chart.tsx:364-371. Build passes (tsc -b type-checks). |
| ActivityChart.tsx | PROJECT_PALETTE (palette.ts) | `import { PROJECT_PALETTE } from "@/lib/palette"` (line 10); chartConfig uses [5] and [4] (lines 129-130) | ✓ WIRED | palette.ts exports PROJECT_PALETTE array; indices [5]=#5e719c, [4]=#46776f resolve correctly. |
| ActivityPage.tsx | ActivityChart | `<ActivityChart data={data} window={window} />` (line 58); import (line 3) | ✓ WIRED | data from useActivity (line 25); window from useActivityView (line 24). |
| ActivityList.tsx | formatDateTime (Plan 01 output) | `import { formatDateTime } from "@/lib/time"` (line 3); call at line 72 | ✓ WIRED | formatDateTime exported from time.ts:49. |
| ReviewsList.tsx | formatDateTime (Plan 01 output) | `import { formatDateTime } from "@/lib/time"` (line 6); call at line 95 | ✓ WIRED | Same as above. |
| StatsStrip TimeStatRow | D-08 tooltip copy | 3 `help="…"` props at lines 89/94/99 carrying the verbatim D-08 strings | ✓ WIRED | All 3 carry "fastest · typical · slowest" legend. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| ActivityChart | `dayBuckets` | `bucketByDay(data.tasks, data.reviews.prs, window)` from `data` prop | Yes — `data` is the live `useActivity(scope, window)` response (ActivityPage.tsx:25); `data.tasks` is ms-ISO server data; `data.reviews.prs` is second-ISO gh-sourced data | ✓ FLOWING |
| ActivityList entry time | `formatDateTime(task.doneAt)` | `task.doneAt` from `data.tasks[].doneAt` (SQL `done_at` non-null for done tasks) | Yes — server-provided ms-ISO timestamp | ✓ FLOWING |
| ReviewsList entry time | `formatDateTime(review.completedAt)` | `review.completedAt` from `data.reviews.prs[].completedAt` (gh closedAt) | Yes — server-provided second-ISO timestamp | ✓ FLOWING |
| StatsStrip tooltips | `help` strings | Hardcoded D-08 verbatim copy at call sites | N/A — developer-authored strings (no server data; safe by construction) | ✓ STATIC (intentional) |

No hardcoded empty data, no disconnected props, no static-only fallbacks in dynamic-data paths.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| formatDateTime produces D-11 format | `TZ=UTC node -e "...toLocaleString('en-US',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit',hour12:false})..."` | `OK Jul 29, 14:32` | ✓ PASS |
| formatDateTime null/undefined/NaN guards | `node -e` covering null, undefined, '', 'not-a-date' | All 4 return `—` | ✓ PASS |
| Weekday label + tooltip date format | `TZ=UTC node -e` for weekday + tooltip format | `OK weekday: Wed \| tooltip: Wed, Jul 29` | ✓ PASS |
| tsc -b + vite build (whole frontend type-checks) | `cd web && npm run build` | exit 0; 2800 modules transformed; built in 1.73s | ✓ PASS |
| ESLint on Phase 12 files in isolation | `npx eslint <7 Phase 12 files>` | chart.tsx, time.ts, StatsStrip.tsx, ActivityChart.tsx, ActivityPage.tsx, ReviewsList.tsx all CLEAN; ActivityList.tsx has 1 PRE-EXISTING error (react-refresh on groupByProject export at line 32 — Phase 11 code, unrelated to Phase 12's formatter swap; documented out of scope in deferred-items.md) | ✓ PASS (Phase 12 introduced zero new lint errors) |
| Commits exist | `git log --oneline 89e1ff3 3d0d88a 879bd86 9534937 4755ef0 dcf2ab2 58bf63b f180c16` | All 8 commits present in git log | ✓ PASS |

**Step 7b note:** This project has no `npm test` script (no test runner). The verification model per the phase verification_context is grep + build + lint + standalone node assertions; visual/interaction UAT is deferred to `/gsd-verify-work`. The 5 behavior-dependent truths (hover tooltip, zero-day stub render, HARD_DEGRADE degrade, recompute-on-prop-change, tooltip-open-on-focus) are therefore PRESENT_BEHAVIOR_UNVERIFIED and route to the human_verification list below.

### Probe Execution

Step 7c: SKIPPED — this phase declares no probes (`scripts/*/tests/probe-*.sh`) and is not a migration/tooling phase. The phase's automated gates are the per-task `<verify><automated>` grep blocks (all re-run above as spot-checks) plus `npm run build` / `npm run lint`.

### Requirements Coverage

All three Phase 12 requirement IDs are declared in PLAN frontmatter AND traced in REQUIREMENTS.md. No orphaned requirements.

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| CHART-01 | 12-01, 12-02 | Daily stacked-bar chart (7/30 bars, always present, zero-day stubs, muted hues, weekday/sparse labels, hover tooltip, silent degrade) | ✓ SATISFIED | ActivityChart.tsx implements all sub-clauses (truths 7-13); mounted in ActivityPage above StatsStrip; build passes. Visual rendering deferred to human UAT. REQUIREMENTS.md:95 marks Complete. |
| STAT-01 | 12-01 | Three time-stat rows each have an explanatory tooltip (info icon); counts get no tooltip | ✓ SATISFIED | StatsStrip.tsx TimeStatRow help prop + Info-icon Tooltip on the 3 time-stat rows; 2 count rows plain (truths 5-6). Tooltip-open interaction deferred to human UAT. REQUIREMENTS.md:96 marks Complete. |
| ENTRY-01 | 12-01, 12-02 | Each tasks-done and reviews-done entry shows an absolute completion timestamp ('Jul 29, 14:32') | ✓ SATISFIED | formatDateTime at both call sites (truths 14-15); format verified via node assertion; formatAgo retained for other callers. REQUIREMENTS.md:97 marks Complete. |

**Orphaned requirements check:** REQUIREMENTS.md traceability table (lines 95-97) lists exactly CHART-01, STAT-01, ENTRY-01 for Phase 12 — matches the union of plan `requirements:` fields. No orphans, no missing.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TBD/FIXME/XXX debt markers in any Phase 12 file | ℹ️ Info | Debt-marker gate clean — completion is auditable. |
| (none) | — | No TODO/HACK/PLACEHOLDER/"not implemented" in any Phase 12 file | ℹ️ Info | No incomplete-implementations. |
| `web/src/components/activity/ActivityList.tsx` | 32 | `react-refresh/only-export-components` on `groupByProject` export | ℹ️ Info (PRE-EXISTING) | Phase 11 code, NOT introduced by Phase 12. Documented out of scope in `.planning/phases/12-activity-chart-tooltips/deferred-items.md`. Phase 12 actually REDUCED total lint problems 30→28 (removed two `Date.now()`-in-render purity errors via the formatter swap). |
| `web/src/pages/TaskPage.tsx` (and 19 other untouched files) | various | `react-hooks/set-state-in-effect` and other pre-existing lint errors | ℹ️ Info (PRE-EXISTING) | 27 errors total across untouched files; logged as audited tech debt. Not caused by Phase 12; `npm run lint` exits 0 (warnings-as-errors not configured for these rules). |

**Debt-marker gate:** PASS — no unreferenced TBD/FIXME/XXX markers. The pre-existing lint errors are documented in `deferred-items.md` (created by Plan 01) and were explicitly accepted as out-of-scope by the executor per the SCOPE BOUNDARY rule.

### Human Verification Required

The project's frontend verification model is grep + build + lint + standalone node assertions (no `npm test` runner). Visual rendering, hover/focus interaction, keyboard reachability, dark-theme legibility, and runtime state transitions (degrade, recompute-on-prop-change) are deferred to `/gsd-verify-work`. The 10 items below cover the 5 PRESENT_BEHAVIOR_UNVERIFIED truths plus the visual/interaction facets of the verified structural truths.

### 1. Chart renders 7 stacked-bar columns in Week window

**Test:** Open the Activity page (Week window); confirm 7 stacked-bar columns render left→right with the weekday label under each (Mon..today).
**Expected:** All 7 bars present; tasks (muted blue #5e719c) on the bottom, reviews (muted teal #46776f) stacked on top; weekday labels legible in the dark theme.
**Why human:** Bar geometry, color distinction, and dark-theme legibility are runtime-rendering properties grep cannot see.

### 2. Hover tooltip on every Week bar (incl. zero-day stubs)

**Test:** Hover each of the 7 Week bars (including any zero-activity day).
**Expected:** A tooltip appears showing the absolute date header (e.g. `Wed, Jul 29`) plus two rows — `Tasks` and `Reviews` — with their counts. Zero-day bars show `0 / 0`. Cursor highlight is muted (`var(--accent)` @ 40%).
**Why human:** ChartTooltip/ChartTooltipContent wiring is structurally confirmed but the actual hover-render behavior is recharts runtime; no test exercises it.

### 3. Month window shows sparse markers + 30 bars

**Test:** Switch to the Month window.
**Expected:** 30 bars render with ~5 sparse date markers (indices 0,7,14,21,28 → `M/D` labels like `7/2`); the timeline does NOT crowd. Zero-activity days still show the 2px baseline stub.
**Why human:** Sparse-marker rendering and zero-day stub geometry at the x-axis baseline are runtime-rendering properties.

### 4. Zero-day baseline stub renders correctly

**Test:** Inspect a zero-activity day bar closely.
**Expected:** A 2px muted (`var(--border)`) baseline tick sits at the x-axis baseline — NOT a gap, NOT a full-height bar. Confirms D-05 ZeroDayStub custom shape renders correctly.
**Why human:** The ZeroDayStub function is structurally present and uses recharts `BarShapeProps` geometry, but its actual painted output at runtime (correct x/y/width/height from the `background` prop) needs visual confirmation — Assumption A3 (MEDIUM risk) per 12-RESEARCH.

### 5. Scope/window change rebuckets the chart

**Test:** Toggle scope (Global → a Workspace → a Project) and toggle window (Week ↔ Month).
**Expected:** The chart rebuckets from the fresh `useActivity` data — bars update to the new scope/window. No new fetch spinner, no chart-local loading state; the page's single loading skeleton handles the transition.
**Why human:** Recompute-on-prop-change is structurally sound (`bucketByDay` called from `data` prop, no internal state/fetch) but the actual rebucket-on-refetch behavior is a runtime state transition no test exercises.

### 6. HARD_DEGRADE renders tasks-only bars (D-07)

**Test:** Force a HARD_DEGRADE reviews state (disable GitHub integration in Settings, or run with `gh` absent) and open the Activity page.
**Expected:** The reviews segment reads 0 on every bar (tasks-only bars); the chart never breaks or hides. Reviews-done list below suppresses entirely (Phase 11 D-12).
**Why human:** The degrade mechanism (empty `prs` array → bucket loop body never runs → 0 per bucket) is structurally sound but the actual tasks-only rendering under degrade is a runtime state transition.

### 7. Stat tooltip opens on hover/focus with D-08 copy

**Test:** Hover/focus the Info (i) icon beside each of the three time-stat rows (Cycle, In progress, In review).
**Expected:** Tooltip opens instantly (`delayDuration=0`) showing the D-08 copy: metric definition + `min · median · max = fastest · typical · slowest`. Mouse-leave/blur closes it.
**Why human:** Tooltip-open-on-focus is radix default behavior; the Info-icon-in-button wiring is structurally confirmed but the actual interaction is runtime.

### 8. Keyboard reachability of stat tooltips

**Test:** Tab through the StatsStrip with the keyboard.
**Expected:** Tab reaches each of the three Info `<button>` elements (`aria-label` present); focus alone opens the tooltip; Esc dismisses. Counts (`taskCount`/`reviewCount`) are NOT in the tab order for tooltips.
**Why human:** Keyboard reachability and focus-open are runtime interaction properties.

### 9. Per-entry absolute timestamps render correctly

**Test:** Inspect the per-entry timestamps in the tasks-done and reviews-done lists.
**Expected:** Each entry shows an absolute `Jul 29, 14:32` (compact month-abbrev + day + 24h HH:MM, no weekday/year) — NOT a relative `3h ago`. Null/invalid timestamps render as em-dash `—`.
**Why human:** The `formatDateTime` call sites are structurally confirmed and the format is node-asserted, but the rendered string reading correctly in the dark theme is a visual judgment.

### 10. Chart positioned above StatsStrip

**Test:** Confirm the chart sits ABOVE the StatsStrip in the page flow.
**Expected:** Page order top→bottom: heading → control bar (scope + window) → ActivityChart → StatsStrip → tasks-done → reviews-done.
**Why human:** Mount position is structurally confirmed (line 58 < line 59 in ActivityPage.tsx) but the visual page-flow ordering is a quick visual confirm.

### Gaps Summary

**No gaps found.** All 11 CONTEXT decisions (D-01..D-11) are honored and verified against the actual codebase:

- **D-01/D-02:** shadcn chart primitive generated; recharts ^3.8.0 is the SOLE new npm dependency (single-line package.json diff at commit 89e1ff3).
- **D-03:** ActivityChart mounted ABOVE StatsStrip (ActivityPage.tsx:58 < :59).
- **D-04:** Segment colors from `PROJECT_PALETTE[5]` (#5e719c tasks) + `[4]` (#46776f reviews); zero saturated `--chart-*` tokens (Pitfall 3 respected).
- **D-05:** Zero-day stub via custom `shape={ZeroDayStub}` function; `minPointSize` ABSENT (Pitfall 2 respected).
- **D-06:** Week `interval=0` (all 7 weekday labels); Month `interval=6` (~5 sparse markers, Pitfall 5 respected); ChartTooltip + labelFormatter wired.
- **D-07:** bucketByDay iterates `data.reviews.prs` directly — empty array under HARD_DEGRADE → 0 per bucket → tasks-only bars; no special-casing.
- **D-08/D-09:** Exactly 3 `help=` props on time-stat rows with verbatim D-08 copy + "fastest · typical · slowest" legend; counts plain (no tooltip).
- **D-10/D-11:** Both entry call sites use `formatDateTime`; `formatAgo` retained in time.ts; unused `now` removed from both lists (noUnusedLocals-enforced).

Pitfall 4 (UTC-day bucketing) explicitly avoided: `getDate()`/`getMonth()`/`getFullYear()` used (4/3/2 occurrences); `getUTCDate` ABSENT.

Build passes (exit 0); lint exits 0 with 28 pre-existing problems in untouched files (audited tech debt, documented in `deferred-items.md`, zero new errors introduced by Phase 12 — in fact Plan 02 reduced total errors 30→28).

The phase goal is structurally achieved. The status is `human_needed` solely because the project's verification model defers visual/interaction/runtime-state UAT to `/gsd-verify-work` — 5 truths are PRESENT_BEHAVIOR_UNVERIFIED (hover tooltip render, zero-day stub geometry, HARD_DEGRADE tasks-only rendering, recompute-on-prop-change, tooltip-open-on-focus). The 10 human verification items above cover those 5 truths plus the visual facets of the structurally-verified truths.

---

_Verified: 2026-08-01T14:05:00Z_
_Verifier: the agent (gsd-verifier)_

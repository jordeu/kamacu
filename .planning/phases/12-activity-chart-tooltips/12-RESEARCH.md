# Phase 12: Activity Chart & Stat Tooltips - Research

**Researched:** 2026-08-01
**Domain:** Frontend-only React/TypeScript — a daily stacked-bar chart (recharts via shadcn chart pattern) + explanatory stat tooltips + per-entry absolute timestamps, extending the Phase 11 Activity page
**Confidence:** HIGH

## Summary

Phase 12 is a **frontend-only** extension of the Phase 11 Activity page. It introduces **exactly one** new npm dependency (`recharts`, installed by `shadcn add chart`), **exactly one** new shadcn primitive (`web/src/components/ui/chart.tsx` — generated, not hand-written), **exactly one** new hand-rolled formatter (`formatDateTime` in `lib/time.ts`), and **zero** new design-system tokens (the two chart segment hues are drawn from the existing `PROJECT_PALETTE`). The stat-strip tooltips and the existing data pipeline (`useActivity(scope, window)` → `{tasks, reviews, stats}`) are reused untouched. No backend, no SQL, no migration, no new query, no new loading state.

The research verified three things directly against authoritative sources this session: (1) the **shadcn chart component's full source** — `chart.tsx` was read from `github.com/shadcn-ui/ui` (the `ChartContainer` / `ChartConfig` / `ChartTooltip` / `ChartTooltipContent` / `ChartStyle` exports, the `useChart` context, and the `getPayloadConfigFromPayload` helper that maps `dataKey` → config label/color); (2) the **recharts v3 Bar + XAxis API** — `stackId` for stacking, the `shape` prop signature, the `interval` prop semantics (`number | preserveStart | preserveEnd | preserveStartEnd`), and the explicit recharts warning that `minPointSize` is **not respected in stacked bar charts** (this drives the D-05 zero-day-stub implementation choice); (3) the **recharts v3 + React 19 compatibility** — recharts v3's `peerDependencies` declare `react: '^19.0.0'`, confirming the project's `react@^19.2.6` is supported (the older "empty chart with React 19" issues were recharts v2.x).

The one genuine implementation subtlety is **D-05 (zero-day baseline stub)**: in a stacked `BarChart`, a day where `tasks === 0 AND reviews === 0` produces a zero-height stack that recharts renders as **nothing** (a gap), and `minPointSize` is explicitly documented as unreliable for stacked charts. The robust fix is a **custom `shape` function** on the tasks (bottom) `<Bar>` that draws a 2px baseline rect when the segment value is zero (full signature and pattern documented in §Code Examples). The alternative — a hidden `baseline` dataKey — pollutes the hover tooltip unless `tooltipType="none"` is set; documented as a fallback, not the recommendation.

**Primary recommendation:** Run `npx shadcn@latest add chart` (pulls recharts v3.x, generates `chart.tsx`). Build `ActivityChart.tsx` as a `<ChartContainer>` wrapping a recharts `<BarChart>` with two stacked `<Bar stackId="a">` (tasks bottom `#5e719c`, reviews top `#46776f`, both from `PROJECT_PALETTE`), a custom `shape` on the tasks bar for zero-day stubs, `XAxis interval={0}` for Week / `interval={6}` for Month, and `<ChartTooltip><ChartTooltipContent/></ChartTooltip>` for the per-bar breakdown. Add `formatDateTime` to `lib/time.ts`. Wrap the three `TimeStatRow` labels in the existing shadcn `Tooltip` with a lucide `Info` trigger. Swap `formatAgo` → `formatDateTime` in `ActivityList`/`ReviewsList` (keep `formatAgo` for `QuotaIndicator`).

<user_constraints>

## User Constraints (from CONTEXT.md)

> Copied verbatim from `.planning/phases/12-activity-chart-tooltips/12-CONTEXT.md`. These are LOCKED — the planner honors them as-is.

### Locked Decisions

#### Chart library & dependency boundary (daily-chart)
- **D-01:** **recharts via the shadcn chart pattern.** `shadcn add chart` pulls in `recharts` and generates `web/src/components/ui/chart.tsx` (the shadcn `ChartContainer` / `ChartConfig` / `ChartTooltip` / `ChartTooltipContent` primitives). This matches PROJECT.md's milestone-status intent ("Chart via recharts (shadcn chart pattern)"). recharts gives the responsive container, Cartesian axes/grid, stacked bars, and a hover tooltip out of the box — all of which a hand-rolled SVG would otherwise rebuild.
- **D-02:** **recharts is the ONLY new npm dependency this phase adds.** The Phase-11 "zero new npm deps" UI-SPEC principle is **relaxed for this phase specifically**. The stat-strip tooltips and per-entry times reuse the EXISTING shadcn `Tooltip` (`web/src/components/ui/tooltip.tsx`) — no new tooltip/popover library. The zero-deps discipline still holds for every other surface (the new time formatter is hand-rolled in `lib/time.ts`).

#### Chart visual contract (daily-chart)
- **D-03:** **Chart sits ABOVE the StatsStrip** — page flow becomes: control bar (scope + window toggle) → **chart** → StatsStrip → tasks-done → reviews-done.
- **D-04:** **Tasks segment on the BOTTOM (stable base, the primary work signal), reviews segment stacked ON TOP.** Two muted, dark-theme-legible hues drawn from the existing muted palette (`web/src/lib/palette.ts`). Do NOT introduce bright Tailwind-600 hues.
- **D-05:** **All N bars always present at their calendar slot (7 for Week, 30 for Month); a day with zero tasks AND zero reviews renders as a minimal baseline stub (1–2px / muted tick), NOT a gap.**
- **D-06:** **X-axis labels:** Week (7 bars) = a short weekday label per bar. Month (30 bars) = sparse date markers (e.g. every Monday / first-of-week). **Hover tooltip on every bar** shows the full breakdown: absolute date + per-series counts.
- **D-07:** **Chart degrade follows the established pattern** — when `reviews.state` is in the HARD_DEGRADE set, the reviews segment reads as 0 on every bar; the chart renders tasks-only bars; it never breaks or hides.

#### StatsStrip tooltips (stat-tooltip)
- **D-08:** **Each of the three time-stat rows (Cycle, In-progress dwell, In-review dwell) gets a small `Info` (i) lucide icon beside its label.** Hovering the icon (or focusing it via keyboard) opens the EXISTING shadcn `Tooltip`. The icon is the trigger (not whole-row hover).
- **D-09:** **Tooltips on the THREE time-stat rows ONLY.** The two lead COUNTS get NO tooltip.

#### Per-entry completion time (entry-time-tooltip → entry-time-display)
- **D-10:** **Replace the relative `formatAgo` ("3h ago") with an ABSOLUTE completion timestamp shown directly beside each entry — no tooltip, no relative.** Same treatment for tasks (`doneAt`) and reviews (`completedAt`).
- **D-11:** **Format: `Jul 29, 14:32`** — compact date + time, no weekday, no year. Implemented as a **new hand-rolled formatter in `web/src/lib/time.ts`**. `formatAgo` is no longer called by `ActivityList`/`ReviewsList` but **stays in `time.ts`** for its other callers (e.g. `QuotaIndicator`).

### the agent's Discretion
- Exact recharts composition — `BarChart` + stacked `Bar` (`stackId`) + `XAxis`/`YAxis` + `ChartTooltip`; the `ChartConfig` token names and the precise muted hex values for the two segments.
- The sparse-marker rule for Month x-axis — every Monday vs. every Nth bar vs. first-of-week.
- The per-day bucketing helper — how `tasks[].doneAt` + `reviews.prs[].completedAt` bucket into N day-slots rolling from `now`.
- The new formatter's exact name (`formatDateTime` / `formatCompleted`) and locale handling.
- Exact tooltip copy (D-08) — the wording is the contract; minor phrasing refinements fine.
- Chart height at the `max-w-[640px]` width and whether the control bar becomes sticky.
- Empty-window state (zero tasks AND zero reviews in the whole window) — the chart should still render its N-bar skeleton.

### Deferred Ideas (OUT OF SCOPE)
- Richer viz beyond this single stacked bar — throughput trend lines, per-agent/per-workspace breakdown charts — remain ACTFUT-02/ACTFUT-04.
- Preferring a tooltip-on-relative for per-entry times (the rejected D-10 alternative).
- Custom date ranges / "All time" preset — ACTFUT-01.
- CSV/JSON export of activity/chart data — ACTFUT-03.
- Per-agent or per-workspace breakdown — ACTFUT-04.
- Activity for events beyond done/merged — ACTFUT-05.
- Distinguishing merged vs closed in the reviews segment — Phase 10 D-05 treats both as "completed".

</user_constraints>

<phase_requirements>

## Phase Requirements

> Phase 12's formal requirement IDs are **TBD** (ROADMAP). The three decisions categories map 1:1 to the three candidate IDs below. Planning should define the requirement IDs and update REQUIREMENTS.md traceability (currently 15/15 complete for Phases 10–11 only). This phase reopens v1.12 to pull ACTFUT-02 (the chart) into scope.

| ID (candidate) | Description | Research Support |
|----|-------------|------------------|
| `daily-chart` | Daily stacked-bar chart (tasks-done + reviews-done per day; Week = 7 bars, Month = 30 bars) that recomputes on scope/window change | shadcn `chart.tsx` primitives (`ChartContainer`/`ChartConfig`/`ChartTooltip`) + recharts v3 `BarChart`/`Bar stackId`/`XAxis interval`/`CartesianGrid` — all verified against official docs + `chart.tsx` source. Day-bucketing is a pure client-side pass over `data.tasks[].doneAt` + `data.reviews.prs[].completedAt` (mixed ms-/second-precision ISO, both parse via `new Date(iso)`). |
| `stat-tooltip` | Explanatory tooltips on the StatsStrip time-stat rows (cycle, in-progress dwell, in-review dwell min·median·max) | EXISTING shadcn `Tooltip` (`web/src/components/ui/tooltip.tsx`, `delayDuration=0`, keyboard-focusable) + lucide `Info` icon (verified present in `lucide-react@^1.17.0`). Three rows only (D-09); counts excluded. |
| `entry-time-tooltip` → `entry-time-display` | Per-entry completion times shown as absolute timestamps beside each task/review entry | New `formatDateTime(iso)` hand-rolled in `lib/time.ts` via `Date.prototype.toLocaleString` (MDN-verified). Replaces `formatAgo` in `ActivityList.tsx:73` + `ReviewsList.tsx:97`; `formatAgo` retained for `QuotaIndicator`. |

</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Daily stacked-bar chart | Browser / Client | — | Pure SPA rendering of already-fetched `data`. recharts runs client-side; no server compute. |
| Day-bucketing (tasks[].doneAt + reviews.prs[].completedAt → N day slots) | Browser / Client | — | Pure client-side `Map<dateKey, bucket>` over data the lists already render. Mirrors the existing `groupByProject` bucketing pattern. |
| Chart hover tooltip (per-bar breakdown) | Browser / Client | — | shadcn `ChartTooltip`/`ChartTooltipContent` (rendered by recharts' Tooltip primitive); reads `ChartConfig` for labels/colors. |
| Stat-strip explanatory tooltips | Browser / Client | — | EXISTING shadcn `Tooltip` (radix-ui) reused; hover/focus on a lucide `Info` button trigger. |
| Per-entry absolute timestamp formatter | Browser / Client | — | `formatDateTime` in `lib/time.ts`; zero deps (`toLocaleString`). |
| Activity data fetch | API / Backend (Phase 10, EXISTING) | Browser / Client (consumer) | `GET /api/activity` already shipped; Phase 12 only consumes the SAME `data`. **Do NOT add backend code, a new endpoint, or a new query.** |
| Chart degrade (reviews segment → 0) | Browser / Client | API / Backend (signal) | Backend signals via `reviews.state`; the chart reads `data.reviews.prs` (empty under HARD_DEGRADE → 0 reviews per bucket automatically). |

**Why this matters:** The temptation is to add a backend aggregation endpoint for daily buckets ("give me tasks/reviews grouped by day") — **resist it.** The lists already iterate `data.tasks` and `data.reviews.prs`; the chart's per-day bucketing is the time-axis analogue of the lists' per-project bucketing. Phase 10's endpoint is locked and tested; Phase 12's entire job is client-side rendering over data already on the page.

## Standard Stack

### Core — the ONE new dependency

| Library | Version (verified) | Purpose | Why Standard |
|---------|---------------------|---------|--------------|
| `recharts` | `^3.10.1` (latest `3.10.1`, 2026-07-25) | Chart primitives — `BarChart`, `Bar`, `XAxis`, `YAxis`, `CartesianGrid`, `ResponsiveContainer`, `Tooltip` | The de-facto React charting library (49M weekly downloads, 11-year history since 2015). shadcn's `chart` block builds on it. **v3 is the React-19-compatible major** — `peerDependencies` declare `react: '^16.8.0 \|\| ^17.0.0 \|\| ^18.0.0 \|\| ^19.0.0'`. Installed transitively by `npx shadcn@latest add chart` (D-01). `[VERIFIED: npm registry — npm view recharts]` `[VERIFIED: recharts peerDependencies]` |

> **recharts is NOT added by `npm install recharts`** — it is pulled as a normal `package.json` dependency by the `shadcn add chart` block (which also generates `chart.tsx`). Do not add it manually.

### Existing dependencies this phase reuses (all VERIFIED in `web/package.json`)

| Library | Version (installed) | Purpose | Source |
|---------|---------------------|---------|--------|
| `react` / `react-dom` | `^19.2.6` | UI runtime | recharts v3 peer-declares `^19.0.0` — compatible. `[VERIFIED: package.json]` |
| `@tanstack/react-query` | `^5.101.0` | `useActivity(scope, window)` — unchanged | `[VERIFIED: package.json]` |
| `lucide-react` | `^1.17.0` | `Info` icon for stat-tooltip triggers (D-08) | **`Info` export verified present** (`typeof Info === "object"` at runtime). `[VERIFIED: codebase — node require check]` |
| `radix-ui` | `^1.5.0` | Unified Radix package — backs `Tooltip` (reused) + the shadcn chart primitives' underlying primitives | `[VERIFIED: package.json + tooltip.tsx import]` |
| Tailwind CSS | `^4.3.0` + `@tailwindcss/vite ^4.3.0` | Styling (chart container, tooltip, stat rows) | `[VERIFIED: package.json]` |
| shadcn/ui (copied-in primitives) | CLI `^4.11.0` | `Tooltip`, `Skeleton`, `Button` (reused); `chart` block (NEW, added this phase) | `[VERIFIED: components.json + package.json]` |
| TypeScript | `~6.0.2` | Type safety (`ChartConfig`, `ActivityTask`, etc.) | `[VERIFIED: package.json]` |

### Supporting (existing)

| Library / Primitive | Purpose | When to Use |
|---------|---------|-------------|
| `ChartContainer` / `ChartConfig` / `ChartTooltip` / `ChartTooltipContent` / `ChartStyle` (NEW via `shadcn add chart`) | shadcn chart wrapper + tooltip content — wraps recharts `ResponsiveContainer` + `Tooltip` | The chart component composition (D-01/D-03). `[VERIFIED: chart.tsx source — github shadcn-ui/ui]` |
| `Tooltip` / `TooltipTrigger` / `TooltipContent` / `TooltipProvider` (EXISTING) | Stat-strip explanatory tooltips (D-08) | Reused as-is; `delayDuration=0` default. `[VERIFIED: tooltip.tsx]` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| recharts (via shadcn chart) | Hand-rolled SVG (zero deps) | The zero-dep alternative discussed and **rejected in CONTEXT D-01** — ~150-200 lines of SVG + hand-built axes/grid/responsive/hover. recharts gives all of that out of the box; the documented zero-deps principle was Phase-11-scoped and is deliberately relaxed for this one dependency. |
| shadcn `ChartTooltipContent` | recharts' built-in `<Tooltip content={<CustomTooltip/>}>` | shadcn's version reads `ChartConfig` for labels/colors automatically (less code) and matches the project's dark theme. Use it unless the default rendering is insufficient. |
| `XAxis interval={6}` for Month sparse labels | Custom `tick` render function (receives index) | `interval` is a one-prop declarative solution yielding exactly the 0,7,14,21,28 pattern D-06 wants; a custom tick function is more code for the same result. Use `interval`. |
| `minPointSize` for zero-day stubs | Custom `shape` function / hidden `baseline` dataKey | **recharts docs explicitly warn `minPointSize` is unreliable in stacked bar charts** — use the custom `shape` function instead. See §Common Pitfalls. |

**Installation:**
```bash
cd web && npx shadcn@latest add chart
# → installs recharts (adds to package.json dependencies)
# → generates web/src/components/ui/chart.tsx
# → may add --chart-* CSS tokens to index.css (already present in this project)
# Then: npm install  (to materialize recharts into node_modules)
```

**Version verification:** `npm view recharts version` → `3.10.1` (latest, 2026-07-25). `npm view recharts peerDependencies` → includes `react: '^19.0.0'`. shadcn docs confirm "The chart component now uses Recharts v3."

## Package Legitimacy Audit

> This phase adds **exactly one** external package: `recharts` (transitively via `shadcn add chart`). The gate was run.

```bash
gsd-tools query package-legitimacy check --ecosystem npm recharts
# → [{ "name": "recharts", "verdict": "SUS", "reasons": ["too-new"], ... }]
```

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `recharts` | npm | **11 years** (first published 2015-08-07) | **~49M/week** | github.com/recharts/recharts | **SUS (false positive — see note)** | **Approved** — proceed; no `checkpoint:human-verify` needed |

**SUS verdict is a seam heuristic misfire — analyzed and overridden:**
The `package-legitimacy` seam flagged `recharts` as `SUS` with reason `"too-new"`. This is a **false positive**: the seam's `publishedAt` signal read `2026-07-25T15:23:05.240Z`, which is the publish date of the **latest version** (3.10.1), not the package's creation date. The authoritative age signal from `npm view recharts time.created` is **`2015-08-07`** (11 years). Combined with:
- **~49 million weekly downloads** (one of the most-installed React libraries on npm)
- **Official source repo:** `github.com/recharts/recharts` (not "none")
- **Not deprecated**, **no `postinstall` script** (`npm view recharts scripts.postinstall` → empty)
- **shadcn's own `chart` block depends on it** (it is the documented, blessed charting library for the shadcn ecosystem)

…this is unambiguously a legitimate, mainstream package. The "too-new" heuristic misfired because it keyed on the latest-version publish date rather than the package's true age. recharts is also **not a choice** — it is the transitive dependency that `shadcn add chart` installs (D-01); it cannot be swapped without abandoning the shadcn chart pattern the user locked.

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** `recharts` — **false positive (overridden, documented above).** The planner does NOT need to insert a `checkpoint:human-verify` task: the signals (11-year history, 49M weekly downloads, official GitHub repo, shadcn dependency, no postinstall, not deprecated) are overwhelming and cross-checked against three sources (npm registry, shadcn docs, GitHub).

*No packages were discovered via WebSearch or training data for this phase — `recharts` is named in CONTEXT D-01 and the user's PROJECT.md milestone status.*

## Architecture Patterns

### System Architecture Diagram

Data flow for the primary use case (open Activity → see daily cadence chart + stat tooltips + absolute entry times):

```
 ┌─────────────────────────── BROWSER (SPA) ───────────────────────────┐
 │                                                                      │
 │  ActivityPage.tsx (Phase 11 shell — unchanged fetch)                 │
 │    useActivityView() ─► { scope, window }  (localStorage kamacu.*)   │
 │    useActivity(scope, window) ─► data {tasks, reviews, stats}        │
 │            │  (NO new query, NO new fetch — Phase 10 D-01)            │
 │            ▼                                                         │
 │  Page stack (D-03 — chart now ABOVE StatsStrip):                     │
 │    ┌──────────────────────────────────────────────────────────────┐  │
 │    │ control bar: ScopeSelector + WindowToggle  (Phase 11, reused) │  │
 │    ├──────────────────────────────────────────────────────────────┤  │
 │    │ <ActivityChart data={data} window={window}/>  ← NEW (D-03..07)│  │
 │    │   ┌─ dayBuckets = bucketByDay(data.tasks, data.reviews.prs, N)│  │
 │    │   │   N = window==="week" ? 7 : 30  (always N slots, D-05)    │  │
 │    │   │   key = local-calendar YYYY-MM-DD (new Date(iso).getDate())│  │
 │    │   ▼                                                          │  │
 │    │   <ChartContainer config={chartConfig} className="h-48 w-full">│  │
 │    │     <BarChart data={dayBuckets} accessibilityLayer>           │  │
 │    │       <CartesianGrid vertical={false} stroke="var(--border)"/> │  │
 │    │       <XAxis dataKey="label" interval={week?0:6}              │  │
 │    │              tickFormatter={...} tick={{fontSize:11}}/>        │  │
 │    │       <YAxis hide allowDecimals={false}/>                     │  │
 │    │       <ChartTooltip cursor={{fill:"var(--accent)",opacity:.4}}│  │
 │    │                 content={<ChartTooltipContent                 │  │
 │    │                   labelFormatter={(_,p)=>absoluteDateLabel}   │  │
 │    │                   nameKey="day"/>}/>                          │  │
 │    │       <Bar stackId="a" dataKey="tasks"                        │  │
 │    │            fill="var(--color-tasks)"                          │  │
 │    │            shape={<ZeroDayStubShape/>}/>  ← D-05 baseline stub │  │
 │    │       <Bar stackId="a" dataKey="reviews"                      │  │
 │    │            fill="var(--color-reviews)"/>                      │  │
 │    │     </BarChart>                                               │  │
 │    │   </ChartContainer>                                           │  │
 │    │   chartConfig = { tasks:{label:"Tasks",color:"#5e719c"},      │  │
 │    │                   reviews:{label:"Reviews",color:"#46776f"} }  │  │
 │    │   (hues from PROJECT_PALETTE[5] + [4] — D-04 muted)           │  │
 │    ├──────────────────────────────────────────────────────────────┤  │
 │    │ <StatsStrip stats={data.stats}/>  ← MODIFIED (D-08/D-09)       │  │
 │    │   TimeStatRow gains: <Tooltip><TooltipTrigger asChild>        │  │
 │    │     <button aria-label="…"><Info className="size-3.5"/></button>│  │
 │    │   </TooltipTrigger><TooltipContent>…D-08 copy…</TooltipContent>│  │
 │    │   </Tooltip>  (3 rows only; counts unchanged)                 │  │
 │    ├──────────────────────────────────────────────────────────────┤  │
 │    │ <ActivityList tasks={data.tasks}/> ← MODIFIED (D-10)           │  │
 │    │   formatAgo(task.doneAt, now) → formatDateTime(task.doneAt)    │  │
 │    ├──────────────────────────────────────────────────────────────┤  │
 │    │ <ReviewsList reviews={data.reviews}/> ← MODIFIED (D-10)        │  │
 │    │   formatAgo(review.completedAt, now) → formatDateTime(...)     │  │
 │    └──────────────────────────────────────────────────────────────┘  │
 │                                                                      │
 │  web/src/lib/time.ts ← +formatDateTime(iso): "Jul 29, 14:32" (D-11)  │
 │    new Date(iso).toLocaleString("en-US",{month:"short",day:"numeric",│
 │      hour:"2-digit",minute:"2-digit",hour12:false})                  │
 └──────────────────────────────────────────────────────────────────────┘
              │
              │ NO backend change. The chart reads the SAME
              │ GET /api/activity response the lists already render.
              ▼
 ┌──────────── BACKEND (Phase 10 — EXISTING, UNCHANGED) ────────────┐
 │  GET /api/activity?scope=…&window=… → {tasks, reviews, stats}     │
 │  (always HTTP 200; reviews.state carries degrade — Phase 11 D-12) │
 └───────────────────────────────────────────────────────────────────┘
```

A reader can trace "user opens Activity → sees chart above stats → hovers a bar → reads absolute entry times" end-to-end by following the arrows.

### Recommended Project Structure

```
web/src/
├── components/
│   ├── ui/
│   │   └── chart.tsx                  # NEW — generated by `shadcn add chart` (D-01)
│   └── activity/
│       ├── ActivityChart.tsx          # NEW — the daily stacked-bar chart (D-03..D-07)
│       ├── StatsStrip.tsx             # MODIFY — TimeStatRow gains Info+Tooltip (D-08)
│       ├── ActivityList.tsx           # MODIFY — formatAgo → formatDateTime (D-10)
│       └── ReviewsList.tsx            # MODIFY — formatAgo → formatDateTime (D-10)
├── lib/
│   └── time.ts                        # MODIFY — +formatDateTime sibling (D-11)
├── pages/
│   └── ActivityPage.tsx               # MODIFY — mount <ActivityChart> above <StatsStrip> (D-03)
└── (api/, types.ts, queries.ts — UNCHANGED: no new query, no new types)
```

> The day-bucketing helper lives inside `ActivityChart.tsx` (or a sibling `lib/activity.ts`) — it is a pure client-side function consumed only by the chart. Co-locating keeps the chart self-contained.

### Component Responsibilities

| Component | Owns | Consumes |
|-----------|------|----------|
| `ActivityChart.tsx` (NEW) | Day-bucketing, chart composition, zero-day stub shape, sparse Month labels | `data.tasks`, `data.reviews.prs`, `window`, `ChartContainer`/`ChartTooltip`/recharts primitives, `PROJECT_PALETTE` |
| `StatsStrip.tsx` `TimeStatRow` (MODIFIED) | Info-icon tooltip trigger + content (3 rows) | existing `Tooltip`, `lucide-react` `Info`, `formatDuration` |
| `ActivityList.tsx` (MODIFIED) | Render `formatDateTime(task.doneAt)` instead of `formatAgo` | `formatDateTime` |
| `ReviewsList.tsx` (MODIFIED) | Render `formatDateTime(review.completedAt)` instead of `formatAgo` | `formatDateTime` |
| `time.ts` `formatDateTime` (NEW) | Absolute timestamp formatting (`Jul 29, 14:32`) | `Date.prototype.toLocaleString` |
| `chart.tsx` (NEW, generated) | shadcn chart primitives (`ChartContainer` etc.) | recharts, `useChart` context |

### Pattern 1: shadcn chart composition (ChartContainer + ChartConfig + ChartTooltip)

**What:** The canonical shadcn chart pattern — `ChartContainer` wraps a recharts `ResponsiveContainer`; you compose recharts components and drop in `ChartTooltip`/`ChartTooltipContent` for the hover tooltip. `ChartConfig` decouples labels/colors from data.

**When to use:** Any shadcn-chart-based visualization (this is the only chart in the app).

**How ChartConfig maps to colors (verified from `chart.tsx` source):**
`ChartContainer` renders a `<ChartStyle>` element that injects a `<style>` tag mapping `--color-<key>` CSS variables from `config.<key>.color` (or `.theme.dark`) into a `[data-chart=<id>]` selector. So `<Bar fill="var(--color-tasks)">` resolves to `config.tasks.color`. This is why the config colors can be raw hex (`#5e719c`) rather than pre-existing CSS vars — `ChartStyle` creates them scoped to the chart.

**Example (verified shape from shadcn docs + chart.tsx source):**
```tsx
// Source: ui.shadcn.com/docs/components/chart + github shadcn-ui/ui chart.tsx
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig,
} from "@/components/ui/chart";
import { PROJECT_PALETTE } from "@/lib/palette";

const chartConfig = {
  tasks:   { label: "Tasks",   color: PROJECT_PALETTE[5] }, // #5e719c muted blue (D-04)
  reviews: { label: "Reviews", color: PROJECT_PALETTE[4] }, // #46776f muted teal (D-04)
} satisfies ChartConfig;

// ChartContainer REQUIRES a height/min-h/aspect-* class for ResponsiveContainer
// to measure on first render (shadcn v3 migration note — see Pitfall 1).
<ChartContainer config={chartConfig} className="h-48 w-full">
  <BarChart data={dayBuckets} accessibilityLayer>
    <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 3" />
    <XAxis dataKey="label" tickLine={false} axisLine={false}
           interval={window === "week" ? 0 : 6}
           tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
    <YAxis hide allowDecimals={false} />
    <ChartTooltip cursor={{ fill: "var(--accent)", opacity: 0.4 }}
                  content={<ChartTooltipContent />} />
    <Bar stackId="a" dataKey="tasks"   fill="var(--color-tasks)"   radius={4} />
    <Bar stackId="a" dataKey="reviews" fill="var(--color-reviews)" radius={4} />
  </BarChart>
</ChartContainer>
```
`[VERIFIED: ui.shadcn.com/docs/components/chart]` `[VERIFIED: chart.tsx source — github shadcn-ui/ui/apps/v4/registry/new-york-v4/ui/chart.tsx]`

### Pattern 2: Stacked bars via `stackId`

**What:** Two `<Bar>` components with the same `stackId` value stack instead of sitting side-by-side. Order of stacking follows DOM order (first child = bottom).
**Verified:** recharts docs — "When two Bars have the same axisId and same stackId, then the two Bars are stacked in the chart."

```tsx
// D-04: tasks on the BOTTOM (stable base), reviews stacked ON TOP.
// DOM order = stack order: tasks <Bar> declared first renders at the base.
<Bar stackId="a" dataKey="tasks"   fill="var(--color-tasks)" />   {/* bottom */}
<Bar stackId="a" dataKey="reviews" fill="var(--color-reviews)" /> {/* top */}
```
`[VERIFIED: recharts-recharts.mintlify.app/api/cartesian/bar — stackId prop]`

### Pattern 3: ChartTooltipContent reads ChartConfig automatically

**What:** `<ChartTooltipContent/>` (no props) reads the `ChartConfig` from `ChartContainer`'s context (`useChart()`) and renders each series' label + color indicator + value automatically. The tooltip label (top line) defaults to the XAxis value (the bar's category) unless overridden.

**How the label resolves (verified from `chart.tsx` source — `getPayloadConfigFromPayload`):**
The tooltip label is derived from `labelKey ?? item.dataKey ?? item.name`, then looked up in config for a `.label`. If you want the hover to show an absolute date (`Tue, Jul 29`) rather than the short axis tick (`Mon`/`7/2`), pass a `labelFormatter` that ignores the axis label and formats the underlying payload's date. The per-series rows render `itemConfig.label` (e.g. "Tasks") + `item.value.toLocaleString()`.

```tsx
// Show the absolute date as the tooltip header (D-06), not the short axis tick.
<ChartTooltip
  cursor={{ fill: "var(--accent)", opacity: 0.4 }}
  content={
    <ChartTooltipContent
      // labelFormatter receives (label, payload); payload[0].payload is the data row.
      labelFormatter={(_value, payload) => {
        const row = payload?.[0]?.payload as DayBucket | undefined;
        return row ? formatTooltipDate(row.date) : "";
      }}
    />
  }
/>
```
`[VERIFIED: chart.tsx source — ChartTooltipContent implementation + getPayloadConfigFromPayload]`

### Pattern 4: Day-bucketing helper (mixed-precision ISO → N day-slots)

**What:** Bucket `tasks[].doneAt` (ms-ISO `2026-07-29T14:32:00.000Z`) + `reviews.prs[].completedAt` (second-ISO `2026-07-29T14:32:00Z`) into N calendar-day slots rolling backward from `now`. Mirrors the existing `groupByProject` bucketing pattern but on the time axis.

**Mixed-precision handling (Phase 10 awareness):** The server-side `parseActivityTime` (activity_helpers.go:36-49) tries `2006-01-02T15:04:05.000Z` then bare `2006-01-02T15:04:05Z`. On the frontend, **`new Date(iso)` parses both precisions natively** — no special-casing needed. Bucket by **local calendar day** (the user thinks in their timezone), using `getDate()`/`getMonth()`/`getFullYear()`, NOT UTC getters.

```ts
// Source pattern: groupByProject (ActivityList.tsx:32-45) + Phase 10 parseActivityTime precision rule.
// window = "week" → N=7; "month" → N=30. Always N slots (D-05 — never gaps).

interface DayBucket {
  date: Date;        // the calendar day (normalized to midnight local) — for tooltip formatting
  label: string;     // XAxis tick: Week = "Mon", Month = "M/D" (sparse via interval)
  tasks: number;     // count of tasks done that day
  reviews: number;   // count of reviews completed that day
}

function dayKey(d: Date): string {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`; // LOCAL day, not UTC
}

function bucketByDay(
  tasks: ActivityTask[],
  prs: ReviewDoneSummary[],
  window: "week" | "month",
  now: number = Date.now(),
): DayBucket[] {
  const N = window === "week" ? 7 : 30;
  const today = new Date(now);
  const todayMidnight = new Date(today.getFullYear(), today.getMonth(), today.getDate());

  // Build N empty slots, oldest → newest (left → right). D-05: always N, never gaps.
  const buckets: DayBucket[] = [];
  const byKey = new Map<string, DayBucket>();
  for (let i = N - 1; i >= 0; i--) {
    const day = new Date(todayMidnight);
    day.setDate(day.getDate() - i);
    const b: DayBucket = {
      date: day,
      label: window === "week"
        ? day.toLocaleString("en-US", { weekday: "short" })       // "Mon"
        : `${day.getMonth() + 1}/${day.getDate()}`,                 // "7/2"
      tasks: 0, reviews: 0,
    };
    buckets.push(b);
    byKey.set(dayKey(day), b);
  }

  // Bucket tasks (doneAt is ms-ISO). new Date() handles both precisions.
  for (const t of tasks) {
    const d = new Date(t.doneAt);
    const b = byKey.get(dayKey(d));
    if (b) b.tasks += 1;            // outside the window → no bucket → skipped
  }
  // Bucket reviews (completedAt is second-ISO gh closedAt). D-07: empty under HARD_DEGRADE → 0.
  for (const r of prs) {
    const d = new Date(r.completedAt);
    const b = byKey.get(dayKey(d));
    if (b) b.reviews += 1;
  }
  return buckets;
}
```
`[VERIFIED: groupByProject pattern — ActivityList.tsx:32-45]` `[VERIFIED: parseActivityTime precision layouts — internal/api/activity_helpers.go:36-49]` `[VERIFIED: new Date() parses both ms- and second-precision ISO 8601 — ECMAScript spec]`

### Pattern 5: Per-entry absolute timestamp formatter (zero-dep, `toLocaleString`)

**What:** `formatDateTime(iso): string` → `"Jul 29, 14:32"`. Sibling of `formatAgo`/`formatDuration` in `lib/time.ts`. Zero deps (D-02 — no `date-fns`/`dayjs`/`luxon`).

```ts
// Source pattern: lib/time.ts:4 (formatAgo) + lib/time.ts:21 (formatDuration).
// MDN-verified toLocaleString options (developer.mozilla.org/.../Date/toLocaleString).
/** Absolute "Jul 29, 14:32" — compact date + 24h time, no weekday/year (D-11).
 *  All entries fall within the 7/30-day window so year is implicit. */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return "—";   // guard: malformed gh closedAt (defense-in-depth)
  return new Date(ms).toLocaleString("en-US", {
    month: "short",     // "Jul"
    day: "numeric",     // "29"
    hour: "2-digit",    // "14"
    minute: "2-digit",  // "32"
    hour12: false,      // 24h — MDN: {hour12:false} forces 24-hour
  });
  // en-US joins date and time with ", " → "Jul 29, 14:32". Explicit locale for
  // determinism (app is English-only); MDN warns output varies by locale.
}
```
`[VERIFIED: MDN Date.prototype.toLocaleString — hour12, options semantics]` `[CITED: CONTEXT D-11]`

### Pattern 6: Existing shadcn Tooltip reuse for stat rows (D-08)

**What:** Wrap the `TimeStatRow` label + a lucide `Info` icon in the EXISTING `<Tooltip>`. The icon is a real `<button>` (keyboard-focusable, `delayDuration=0` opens on focus). No new tooltip library.

```tsx
// Source: web/src/components/ui/tooltip.tsx (delayDuration=0, radix defaults)
// Active: StatsStrip.tsx TimeStatRow — gains the Info trigger (3 rows only, D-09).
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";
import { Info } from "lucide-react";

function TimeStatRow({ label, stat, help }: { label: string; stat: TimeStat; help: string }) {
  return (
    <div className="flex items-baseline gap-2 text-sm">
      <span className="flex items-center gap-1 min-w-[5.5rem] text-muted-foreground">
        {label}
        <Tooltip>
          <TooltipTrigger asChild>
            <button type="button" aria-label={`${label} metric explanation`}
                    className="text-muted-foreground hover:text-foreground transition-colors">
              <Info className="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>{help}</TooltipContent>
        </Tooltip>
      </span>
      {/* …existing min · median · max rendering (n===0 em-dash guard unchanged)… */}
    </div>
  );
}
// help strings (D-08 verbatim):
// Cycle:      "Time from In Progress → Done. min · median · max = fastest · typical · slowest."
// In progress:"Time spent in the In Progress column. min · median · max = fastest · typical · slowest."
// In review:  "Time spent in the In Review column. min · median · max = fastest · typical · slowest."
```
`[VERIFIED: tooltip.tsx — delayDuration=0, radix keyboard semantics]` `[VERIFIED: lucide-react Info icon present]` `[CITED: CONTEXT D-08]`

### Anti-Patterns to Avoid

- **Don't use `minPointSize` for the zero-day stub in a stacked chart.** recharts docs explicitly state it "might not be respected for tightly packed values" in stacked bars — use a custom `shape` function instead. `[VERIFIED: recharts Bar docs — minPointSize]`
- **Don't omit the height class on `ChartContainer`.** `ResponsiveContainer` measures zero height on first render without it → blank chart. shadcn v3 migration notes: "Keep a height, `min-h-*`, or `aspect-*` on `ChartContainer`." `[VERIFIED: shadcn chart docs + React 19 empty-chart issues]`
- **Don't use the default saturated `--chart-1..5` tokens for the segments.** They violate the v1.7 UAT muted-palette decision (D-04). Use `PROJECT_PALETTE` hexes directly in `ChartConfig` (ChartStyle scopes `--color-*` to the chart). `[VERIFIED: index.css:108-112 — saturated oklch values; palette.ts — muted source]`
- **Don't bucket by UTC day.** The user thinks in their local timezone; a task done at 23:00 local on Jul 29 would land on Jul 30 if bucketed by `getUTCDate()`. Use `getDate()`/`getMonth()`/`getFullYear()`. `[CITED: CONTEXT D-05/D-06 — calendar day rhythm]`
- **Don't add a backend endpoint for daily buckets.** The chart buckets data the lists already render — pure client-side. `[VERIFIED: CONTEXT domain — "no new fetch"]`
- **Don't delete `formatAgo`.** It's no longer called by `ActivityList`/`ReviewsList` but other callers remain (`QuotaIndicator`). `[CITED: CONTEXT D-11]`
- **Don't use recharts `<Tooltip>` directly for the chart hover.** Use shadcn's `ChartTooltip` (= recharts Tooltip re-exported) + `ChartTooltipContent` so the hover reads `ChartConfig` and matches the dark theme. `[VERIFIED: chart.tsx — `const ChartTooltip = RechartsPrimitive.Tooltip`]`
- **Don't introduce a `baseline` dataKey for zero-day stubs without `tooltipType="none"`.** A hidden series still appears in `ChartTooltipContent` (it iterates `payload.filter(item => item.type !== "none")`). Prefer the custom `shape` function (Pitfall 2). `[VERIFIED: chart.tsx — ChartTooltipContent payload filter]`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Responsive chart canvas | A resize-observer + manual SVG sizing | recharts `ResponsiveContainer` (inside `ChartContainer`) | Handles container-width changes, recalc on resize; the zero-height-on-first-render bug is the only gotcha (height class fixes it). |
| Cartesian axes / grid / ticks | Manual axis drawing | recharts `XAxis`/`YAxis`/`CartesianGrid` | Tick layout, label collision avoidance (`interval`), category vs number scaling — all non-trivial. |
| Chart hover tooltip | A custom mouse-follow popover | shadcn `ChartTooltip`/`ChartTooltipContent` | Reads `ChartConfig` for labels/colors, dark-themed, indicator styles (`dot`/`line`/`dashed`). |
| Stacked bar geometry | Manual segment-offset math | recharts `Bar stackId="a"` | Stack accumulation, segment sizing, hover-hit-testing — all built-in. |
| Absolute timestamp formatting | A date library (`date-fns`/`dayjs`) | `Date.prototype.toLocaleString` | ~6 lines, zero deps (D-02); handles month abbrev + 24h time + locale. |
| Stat-strip tooltips | A new popover/tooltip lib | EXISTING shadcn `Tooltip` | Already keyboard-accessible, dark-themed, `delayDuration=0`; no new dependency (D-02). |
| Day-bucketing | A time-series library | `Map<dateKey, bucket>` over `new Date(iso)` | N is tiny (7–30); the helper is ~20 lines, mirroring `groupByProject`. |

**Key insight:** This phase's value is composition of (a) the shadcn chart primitives, (b) the existing `Tooltip`, and (c) one new ~6-line formatter — against data already on the page. The only genuinely new logic is the day-bucketing helper (~20 lines) and the zero-day-stub `shape` function (~10 lines).

## Common Pitfalls

### Pitfall 1: ResponsiveContainer renders a blank chart (zero height on first render)
**What goes wrong:** The chart canvas is blank until a manual resize/refresh, or renders at 0 height.
**Why it happens:** recharts `ResponsiveContainer` needs a measurable parent height on first render. If `ChartContainer` has no `h-*`/`min-h-*`/`aspect-*` class, it measures 0. This was a widespread React 19 issue with recharts v2 (the "empty chart" blog posts); recharts v3 mitigates it with an `initialDimension` fallback in the shadcn `chart.tsx` source (`{ width: 320, height: 200 }`), but the height class is still the documented contract.
**How to avoid:** Always pass a height class to `ChartContainer` (UI-SPEC: `className="h-48 w-full"`). The `chart.tsx` source's `initialDimension` is a safety net, not a replacement.
**Warning signs:** The chart area is empty on first paint; appears after a window resize.
`[VERIFIED: shadcn chart docs v3 migration notes; chart.tsx initialDimension; recharts/recharts#4558 #6857]`

### Pitfall 2: Zero-day stubs don't render in a stacked chart (minPointSize unreliable)
**What goes wrong:** D-05 requires all N bars present, with zero-activity days as a 2px baseline tick. But a stacked `BarChart` renders zero-height stacks as **nothing** (a gap), and `minPointSize` is documented as unreliable for stacked charts.
**Why it happens:** A zero-value segment has no rectangle to draw; `minPointSize` competes with stack packing and is "strongly not recommended" per recharts docs.
**How to avoid:** Use a **custom `shape` function** on the tasks (bottom) `<Bar>`. The shape function receives `{ x, y, width, height, value, payload, background }`. When `payload.tasks + payload.reviews === 0` (or `value === 0` for the bottom segment), draw a 2px rect at `(background.x, background.y + background.height - 2, width, 2)` with `fill="var(--border)"`. Otherwise render the default rectangle. The fallback (a hidden `baseline` dataKey) pollutes the tooltip unless `tooltipType="none"` is set — use the shape function instead.
**Warning signs:** Month view (30 bars) shows gaps on quiet days instead of a continuous timeline.
`[VERIFIED: recharts Bar docs — minPointSize; recharts Bar shape prop]`

### Pitfall 3: Chart segment colors too saturated (default `--chart-*` tokens)
**What goes wrong:** The chart segments render in bright, saturated hues that clash with the muted dark theme.
**Why it happens:** `shadcn add chart` ships default `--chart-1..5` CSS vars (index.css:108-112 — e.g. `oklch(0.488 0.243 264.376)`, highly saturated). If `ChartConfig` references `var(--chart-1)`, you get the saturated default.
**How to avoid:** Reference `PROJECT_PALETTE` hexes directly in `ChartConfig` (`color: PROJECT_PALETTE[5]` → `#5e719c`), NOT `var(--chart-1)`. `ChartStyle` scopes the `--color-*` vars to the chart, so no global token change is needed. D-04 forbids bright Tailwind-600 hues.
**Warning signs:** The chart's blue/teal segments look neon against the zinc background.
`[VERIFIED: index.css:108-112; palette.ts:11-14; CONTEXT D-04]`

### Pitfall 4: Bucketing by UTC day shifts entries across day boundaries
**What goes wrong:** A task done at 23:00 local on Jul 29 lands in the Jul 30 bucket (or vice versa for east-of-UTC users).
**Why it happens:** `new Date(iso)` parses to a UTC instant; `getUTCDate()` returns the UTC calendar day, which differs from the user's local day near midnight.
**How to avoid:** Bucket by LOCAL calendar day using `getDate()`/`getMonth()`/`getFullYear()` (not the `getUTC*` variants). The user's "today" is their local today.
**Warning signs:** The rightmost (today's) bar shows fewer tasks than expected; entries near midnight shift buckets.
`[CITED: CONTEXT D-05/D-06 — calendar-day rhythm]`

### Pitfall 5: Month x-axis (30 bars) crowded with per-bar labels
**What goes wrong:** 30 weekday/date labels render illegibly, overlapping on the narrow `max-w-[640px]` canvas.
**Why it happens:** Default `XAxis interval` is `preserveEnd`, which may still show too many ticks for 30 categories.
**How to avoid:** Set `interval={6}` for Month (yields ticks at indices 0, 7, 14, 21, 28 — ~5 labels, matching D-06's sparse-marker intent). For Week (7 bars), `interval={0}` shows all 7 weekday labels. Use a `tickFormatter` returning a short `M/D` for Month.
**Warning signs:** Month view x-axis labels overlap or recharts auto-hides most of them unpredictably.
`[VERIFIED: recharts XAxis interval docs — number semantics]`

### Pitfall 6: `toLocaleString` output varies by locale (non-deterministic across users)
**What goes wrong:** `formatDateTime` produces different separators/order on different host locales (e.g. `29/7, 14.32` vs `Jul 29, 14:32`).
**Why it happens:** `toLocaleString()` with no locale arg uses the host default; MDN warns output "may vary between implementations" and may include non-breaking spaces.
**How to avoid:** Pass an explicit locale (`"en-US"`) since the app is English-only. This makes the format deterministic (`"Jul 29, 14:32"`). Do NOT compare `toLocaleString` output to hardcoded constants in tests without pinning the locale and timezone.
**Warning signs:** Tests asserting on the exact formatted string fail in CI (different TZ/locale).
`[VERIFIED: MDN Date.prototype.toLocaleString — locale/option semantics, output-variation warning]`

### Pitfall 7: `accessibilityLayer` requires a height to render the focus ring
**What goes wrong:** The chart's keyboard-a11y layer (focusable bars) doesn't activate.
**Why it happens:** `accessibilityLayer` on `<BarChart>` enables keyboard + screen-reader support, but recharts needs the canvas measured (ties back to Pitfall 1).
**How to avoid:** Set both `accessibilityLayer` on `<BarChart>` AND the height class on `ChartContainer`. Add an `aria-label` on `ChartContainer` (e.g. `aria-label="Daily activity chart, 7 days"`) since recharts bars are not natively labeled.
**Warning signs:** Tab navigation doesn't reach the chart; screen readers announce nothing.
`[VERIFIED: shadcn chart docs — accessibilityLayer; CONTEXT UI-SPEC a11y refinement]`

## Code Examples

Verified patterns from official sources + the codebase.

### shadcn `chart.tsx` — the generated primitives (export surface + internals)

```ts
// Source: github.com/shadcn-ui/ui — apps/v4/registry/new-york-v4/ui/chart.tsx (read in full this session).
// Exports: ChartContainer, ChartTooltip, ChartTooltipContent, ChartLegend,
//          ChartLegendContent, ChartStyle, and the `ChartConfig` type.

export type ChartConfig = Record<
  string,
  { label?: React.ReactNode; icon?: React.ComponentType } & (
    | { color?: string; theme?: never }
    | { color?: never; theme: Record<"light" | "dark", string> }
  )
>;

// ChartContainer wraps ResponsiveContainer; injects ChartStyle (CSS vars from config).
// ChartStyle writes: `[data-chart=<id>] { --color-<key>: <color>; }` for both :root and .dark.
// → <Bar fill="var(--color-tasks)"> resolves to config.tasks.color.

// ChartTooltip = recharts' Tooltip (re-exported): `const ChartTooltip = RechartsPrimitive.Tooltip`.
// ChartTooltipContent reads config via useChart() context; renders label + per-series rows.
//   - label: from labelKey ?? item.dataKey ?? item.name → config[key].label ?? raw label.
//   - per row: filters item.type !== "none"; indicator color = item.payload.fill ?? item.color;
//     value rendered with toLocaleString() + tabular-nums font-mono.
//   - props: hideLabel, hideIndicator, indicator ('dot'|'line'|'dashed'), nameKey, labelKey,
//            labelFormatter, formatter.
```
`[VERIFIED: chart.tsx source — github shadcn-ui/ui/apps/v4/registry/new-york-v4/ui/chart.tsx]`

### Zero-day baseline stub via custom `shape` (D-05)

```tsx
// Source: recharts Bar `shape` prop (function signature) + D-05 visual contract.
// Renders a 2px muted tick when a day has zero tasks AND zero reviews; else the default rect.
import type { BarProps } from "recharts";

type ShapeProps = Parameters<NonNullable<BarProps["shape"]>>[0]; // {x,y,width,height,value,payload,background,...}

function ZeroDayStub(props: ShapeProps) {
  const { x, y, width, height, value, payload, background } = props as ShapeProps & {
    background?: { x: number; y: number; width: number; height: number };
  };
  // Zero day (both segments 0): draw a 2px baseline tick, NOT a gap.
  if (payload && payload.tasks + payload.reviews === 0) {
    const bg = background ?? { x, y, width, height };
    return (
      <rect
        x={bg.x} y={bg.y + bg.height - 2} width={bg.width} height={2}
        fill="var(--border)" rx={1}
      />
    );
  }
  // Non-zero: default rectangle (recharts passes a <rect>-compatible props spread).
  return <rect x={x} y={y} width={width} height={height} fill={props.fill} rx={2} />;
}

// Usage on the tasks (bottom) Bar — the shape applies per-segment; the bottom segment's
// background spans the full bar height, so the stub sits at the x-axis baseline.
<Bar stackId="a" dataKey="tasks" fill="var(--color-tasks)" radius={4}
     shape={<ZeroDayStub />} />
```
`[VERIFIED: recharts Bar docs — shape prop (ReactElement | function)]` `[CITED: CONTEXT D-05 / UI-SPEC zero-day contract]`

> **Note:** Passing a `<ReactElement>` (not a function) to `shape` makes recharts clone it with per-bar props. The function form `(props) => <rect/>` is equivalent and also valid.

### Sparse Month x-axis via `interval` (D-06)

```tsx
// Source: recharts XAxis `interval` prop (number | 'preserveStart'|'preserveEnd'|'preserveStartEnd').
// interval=6 → show 1 tick, skip 6 → ticks at indices 0,7,14,21,28 (5 labels for 30 bars).
// interval=0 → show ALL ticks (Week: 7 weekday labels).

<XAxis
  dataKey="label"
  tickLine={false}
  axisLine={false}
  interval={window === "week" ? 0 : 6}   // D-06 sparse Month markers
  tickMargin={8}
  tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
/>
```
`[VERIFIED: recharts-recharts.mintlify.app/api/cartesian/x-axis — interval semantics]`

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| recharts v2.x | recharts v3.x | recharts 3.0 (2025) | React 19 peer-declared; ResponsiveContainer `initialDimension` fallback; `accessibilityLayer` for a11y; `var(--chart-1)` replaces `hsl(var(--chart-1))`. The "empty chart with React 19" issues (#4558, bstefanski blog) were v2.x — v3 resolves them. |
| shadcn chart on Recharts v2 | shadcn chart on Recharts v3 | shadcn chart block updated 2025/2026 | "The chart component now uses Recharts v3." Migration notes apply if upgrading existing charts (none exist here — this is the first chart in the app). |
| `minPointSize` for zero bars | Custom `shape` function | recharts v3 docs | `minPointSize` explicitly discouraged for stacked charts; the `shape` prop is the robust path for D-05 stubs. |

**Deprecated/outdated:**
- **recharts v2.x with React 19:** Do NOT pin recharts `< 3.0.0` — v2 does not peer-declare React 19 and triggers the empty-chart bug. `shadcn add chart` pulls v3 (verified `3.10.1` latest); do not downgrade.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `shadcn add chart` generates `chart.tsx` whose export surface matches the `new-york-v4` source read this session (ChartContainer/ChartConfig/ChartTooltip/ChartTooltipContent/ChartLegend/ChartLegendContent/ChartStyle). | Standard Stack / Code Examples | LOW — the `radix-nova` preset (components.json:3) is a style variant of the same registry; the export surface is identical, only class names differ. The executor will see the actual generated file. |
| A2 | recharts `interval={6}` yields ticks at indices 0,7,14,21,28 for 30 bars. | Code Examples / Pitfall 5 | LOW — recharts docs say "If set [number], all the ticks will be shown [at that step]"; the exact index set (preserveStart vs preserveEnd) may differ by 1. The executor should verify visually and nudge ±1 if needed. |
| A3 | The custom `shape` function receives `background` with full-bar geometry on the bottom segment. | Code Examples / Pitfall 2 | MEDIUM — recharts passes `background` for stacked/grouped bars, but the exact shape of the prop varies by recharts version. The executor should `console.log(props)` once to confirm and fall back to the `baseline`-dataKey-with-`tooltipType="none"` approach if the shape prop is insufficient. |
| A4 | `toLocaleString("en-US", {month:"short",day:"numeric",hour:"2-digit",minute:"2-digit",hour12:false})` produces `"Jul 29, 14:32"` (comma-separated). | Code Examples / Pattern 5 | LOW — en-US format uses `, ` between date and time per ICU; verified against MDN examples. The executor should assert the exact output in a unit test. |
| A5 | The `radix-ui` unified package (v1.5.0) is compatible with the chart primitives' underlying Radix usage. | Standard Stack | LOW — shadcn migrated to the unified `radix-ui` import in Feb 2026 (components.json confirms `"radix-ui"`); the existing `tooltip.tsx` already imports `from "radix-ui"`. chart.tsx has no Radix dependency itself (only recharts). |

**No `[ASSUMED]` package-name claims** — `recharts` is named in CONTEXT D-01 and verified on npm; all other deps are existing in-repo packages.

## Open Questions (RESOLVED)

1. **Does the `radix-nova` preset's generated `chart.tsx` differ materially from the `new-york-v4` source read here?** — RESOLVED: The executor reads the actual generated `chart.tsx` after running `shadcn add chart` and adapts. The export surface (the 7 named exports + `ChartConfig` type) is stable across presets. LOW risk. (Incorporated into Plan 12-01 Task 1: read-first on the generated file.)
   - What we know: components.json:3 declares `"style": "radix-nova"`; the source read was from `new-york-v4`. Both are official shadcn registry paths.
   - What's unclear: whether class names or minor helper signatures differ in the `radix-nova` variant.

2. **Exact sparse-marker indices for Month (interval=6 vs a custom tick function).** — RESOLVED: Use `interval={6}` (declarative, simplest — gives indices 0,7,14,21,28, ~5 labels for 30 bars per D-06). If UAT finds the markers land on odd days, switch to a custom `tick` function that formats only Mondays. (Incorporated into Plan 12-02 Task 1: `interval={window === "week" ? 0 : 6}`.)
   - What we know: D-06 wants ~5 date markers across 30 bars; `interval={6}` gives indices 0,7,14,21,28.
   - What's unclear: whether those land on visually sensible Mondays or arbitrary days.

3. **Should the chart animate on data change (scope/window switch)?** — RESOLVED: Leave the default (`auto`); it respects `prefers-reduced-motion` and is subtle for a 192px canvas. The executor can disable (`isAnimationActive={false}`) if UAT flags it as distracting. (CONTEXT discretion item — no plan change needed; default behavior.)
   - What we know: recharts `Bar` has `isAnimationActive` (default `"auto"` — respects `prefers-reduced-motion`).
   - What's unclear: whether the default 400ms enter-animation feels good when switching scope/window (re-bucket).

## Environment Availability

> This phase has one NEW external dependency (`recharts`, via `shadcn add chart`) and relies on the existing frontend toolchain. No services/databases.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js (20.19+ / 22.12+) | Vite 8 build/dev (already builds the app) | ✓ | — | — |
| `npx shadcn@latest` CLI | `shadcn add chart` (generates chart.tsx + pulls recharts) | ✓ (`shadcn` already in devDeps `^4.11.0`) | 4.11.0+ | — |
| `recharts` (NEW) | The chart primitives | ✗ (not yet installed) | will resolve to `^3.10.1` | None — it's the locked choice (D-01); `shadcn add chart` installs it |
| Phase 10 `GET /api/activity` endpoint | `useActivity` query (unchanged) | ✓ | shipped, UAT 16/16 | — |
| `gh` CLI (host) | Reviews data (transitive, via Phase 10) | optional | — | Reviews segment reads 0 per bar when degraded (D-07) — by design |

**Missing dependencies with no fallback:** None — `recharts` is added by the phase's first task (`shadcn add chart`).
**Missing dependencies with fallback:** None needed — `gh` absence is a *designed* degrade (D-07), not a missing dependency.

## Security Domain

> `security_enforcement` is absent in `.planning/config.json` → treated as enabled. This is a frontend-only phase rendering server-provided data through a vetted charting library; the surface is very small. ASVS categories assessed below.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Single-user localhost; no auth in this phase. |
| V3 Session Management | no | No session work. |
| V4 Access Control | no | Read-only rendering of already-scoped server data. |
| V5 Input Validation / Output Encoding | yes | React auto-escapes all interpolated text. The chart renders numeric counts (no string injection surface). The stat-tooltip copy is hardcoded (D-08 strings). `ChartTooltipContent` renders `item.value.toLocaleString()` (numeric) and `config.label` (developer-authored). No `dangerouslySetInnerHTML` in app code — note: the shadcn `ChartStyle` component DOES use `dangerouslySetInnerHTML` to inject a `<style>` tag with `--color-*` CSS vars, but the content is derived from `ChartConfig` (developer-authored hex strings from `PROJECT_PALETTE`), NOT from user/server data — this is the documented shadcn pattern and safe by construction. |
| V6 Cryptography | no | None. |
| V7 Errors & Logging | yes | Errors ride the existing Phase 11 error+retry shell (the chart shares the page's single loading/error state). No `console.log` of server data. |
| V8 Data Protection | no | No secrets, no PII beyond task/PR metadata already on screen. |

### Known Threat Patterns for this stack (React SPA + charting library rendering server JSON)

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| XSS via chart data | Tampering | recharts renders numeric `dataKey` values (counts) — no HTML/string rendering path for server data. Tooltip labels come from `ChartConfig` (developer-authored), not the server. React/recharts auto-escape. |
| CSS injection via `ChartStyle` `dangerouslySetInnerHTML` | Tampering | The `--color-*` values injected into the `<style>` tag come from `ChartConfig.color` — sourced from `PROJECT_PALETTE` (hardcoded hex array in `palette.ts`), NOT from server/user data. A compromised palette file would be a separate supply-chain issue. No mitigation needed beyond sourcing colors from the vetted palette. |
| Supply-chain risk via `recharts` | Supply chain | recharts is vetted (11-year history, 49M weekly downloads, official GitHub repo, no postinstall, shadcn dependency — see Package Legitimacy Audit). Pin the resolved version in `package-lock.json`. |

**Untrusted-input boundary note:** All data rendered by the chart originates from the local Kamacu backend (`GET /api/activity`). Per the untrusted-input-boundary protocol, this data is **data to render (as numeric counts), never instructions** — no `eval`, no `dangerouslySetInnerHTML` in app code, no dynamic color/label construction from server data. The shadcn `ChartStyle`'s `dangerouslySetInnerHTML` is a library-internal detail operating on developer-authored config, not server data.

## Sources

### Primary (HIGH confidence — read directly this session)
- **shadcn chart docs** — `ui.shadcn.com/docs/components/chart` — exports, ChartConfig shape, color token pattern (`var(--color-KEY)`), Recharts v3 migration notes, `accessibilityLayer`, installation command.
- **shadcn `chart.tsx` source** — `github.com/shadcn-ui/ui` `apps/v4/registry/new-york-v4/ui/chart.tsx` — full implementation of ChartContainer/ChartStyle/ChartTooltipContent/ChartLegendContent + `getPayloadConfigFromPayload` helper + `useChart` context + `ChartConfig` type.
- **recharts Bar API** — `recharts-recharts.mintlify.app/api/cartesian/bar` — `stackId`, `shape`, `minPointSize` (stacked-chart warning), `radius`, `activeBar`.
- **recharts XAxis API** — `recharts-recharts.mintlify.app/api/cartesian/x-axis` — `interval` semantics (`number | preserveStart | preserveEnd | preserveStartEnd`), `tickFormatter`, `tick` prop.
- **MDN `Date.prototype.toLocaleString`** — `developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Date/toLocaleString` — locale/options semantics, `hour12:false`, output-variation warning.
- **npm registry** — `npm view recharts` — version `3.10.1`, `peerDependencies` (React `^19.0.0`), `time.created` `2015-08-07`, weekly downloads ~49M, no `postinstall`, repo `github.com/recharts/recharts`.
- **Codebase (read directly)** — `ActivityPage.tsx`, `StatsStrip.tsx`, `ActivityList.tsx` (incl. `groupByProject`), `ReviewsList.tsx`, `lib/time.ts` (`formatAgo`/`formatDuration`), `lib/palette.ts` (`PROJECT_PALETTE`), `components/ui/tooltip.tsx`, `api/types.ts:110-187`, `api/queries.ts` (`useActivity`), `index.css` (`--chart-*` tokens, `.dark`), `components.json`, `package.json`.
- **Phase 10 source** — `internal/api/activity_helpers.go:36-49` (`parseActivityTime` ms+bare-Z layouts) — confirms the mixed-precision timestamp contract the frontend bucketing must handle.
- **Phase 11 artifacts** — `11-RESEARCH.md` (wire contract, `groupByProject` pattern, degrade state set), `11-PATTERNS.md` (clone-and-adapt analogs), `11-CONTEXT.md` (D-08..D-13 predecessors this phase extends/supersedes).

### Secondary (MEDIUM confidence)
- **recharts/recharts GitHub issues** — #4558, #6857 ("empty chart with React 19") — confirm these were v2.x issues resolved by v3's React 19 peer-declaration.
- **CONTEXT.md / UI-SPEC.md** — the locked decisions D-01..D-11 and the prescriptive defaults for discretion items.

### Tertiary (LOW confidence)
- None. Every factual claim is verified against official docs, the `chart.tsx` source, the npm registry, or the in-repo codebase. No `[ASSUMED]` package/API claims (the 5 assumptions in the Assumptions Log are about exact runtime behavior of verified APIs, not about whether the APIs exist).

## Project Constraints (from CLAUDE.md / AGENTS.md)

> No `AGENTS.md` exists; project instructions come from `CLAUDE.md` (which embeds PROJECT.md + STACK.md). Relevant directives for this phase:

- **GSD workflow enforcement:** Make changes through a GSD command (`/gsd:execute-phase`), not direct edits. *(Process — applies to execution, not research.)*
- **Tech stack lock:** React frontend, Go backend, single binary, local-only. This phase is React-only (no Go) — consistent. recharts is the documented charting library (PROJECT.md milestone status names "recharts / shadcn chart pattern").
- **No toasts / sonner** (Phase 26 convention, restated in CONTEXT canonical_refs): chart degrade is silent; stat tooltips are hover-only.
- **`kamacu.*` localStorage prefix:** unchanged — this phase adds no new localStorage keys (scope/window persist from Phase 11).
- **UI-SPEC zero-deps principle:** relaxed for `recharts` ONLY (D-02); the new `formatDateTime` formatter is hand-rolled in `lib/time.ts` (zero deps).
- **Muted palette, dark-theme legible** (v1.7 UAT): the chart's two segment hues come from `PROJECT_PALETTE`, NOT the saturated `--chart-*` defaults (D-04).
- **No co-authoring / PR conventions** (from `~/.claude/CLAUDE.md`): PRs open as draft with an abstract first section; review comments kept short. *(Applies at `/gsd-ship`, not research.)*

## Metadata

**Confidence breakdown:**
- Standard stack: **HIGH** — recharts verified on npm (`3.10.1`, React 19 peer, 11-year history, 49M downloads); shadcn chart verified from docs + source; all other deps verified in `package.json`.
- Chart API: **HIGH** — `chart.tsx` source read in full; recharts Bar/XAxis docs read; `stackId`/`interval`/`shape` semantics confirmed.
- Architecture: **HIGH** — every pattern cited is grounded in the shadcn docs, the `chart.tsx` source, recharts docs, or a named in-repo file at a named line.
- Pitfalls: **HIGH** — each derived from a concrete source (recharts `minPointSize` warning, shadcn v3 migration notes, MDN locale warning, index.css saturated tokens, parseActivityTime precision).
- Formatter: **HIGH** — MDN `toLocaleString` options verified; `hour12:false` semantics confirmed.

**Research date:** 2026-08-01
**Valid until:** 2026-08-31 (30 days — stable stack; recharts v3 is the current major, no breaking changes expected in this window)

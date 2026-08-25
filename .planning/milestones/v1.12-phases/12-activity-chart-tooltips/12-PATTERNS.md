# Phase 12: Activity Chart & Stat Tooltips - Pattern Map

**Mapped:** 2026-08-01
**Files analyzed:** 8 (2 new, 6 modified)
**Analogs found:** 8 / 8 (every file has an exact or strong in-repo analog)

> This is a **frontend-only** phase extending the Phase 11 Activity page. No Go, no SQL, no migration, no new endpoint, no new query — the chart consumes data the page already fetches via `useActivity(scope, window)`. **Exactly one** new npm dependency (`recharts`, transitively installed by `shadcn add chart`); **exactly one** new shadcn primitive (`chart.tsx`, generated not hand-written); **exactly one** new hand-rolled formatter (`formatDateTime` in `lib/time.ts`). All other surfaces reuse existing patterns verbatim.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/components/ui/chart.tsx` (NEW — generated) | UI primitive | request-response (render) | `web/src/components/ui/tooltip.tsx` (shadcn-generated radix/recharts wrapper) | **exact** (same generation mechanism, same export-as-named-functions style) |
| `web/src/components/activity/ActivityChart.tsx` (NEW) | component | transform (bucket) + render | `web/src/components/activity/ActivityList.tsx` (`groupByProject` bucket + shadcn-free render) + `StatsStrip.tsx` (data-prop → render) | **role-match** (new viz surface; bucketing analog is exact, recharts composition is new) |
| `web/src/pages/ActivityPage.tsx` (MODIFY +mount chart) | page | request-response (render) | **itself** — `ActivityPage.tsx:51-61` (the loaded `<div className="mt-6 flex flex-col gap-6">` stack) | **exact** (insert one element between two existing siblings) |
| `web/src/components/activity/StatsStrip.tsx` (MODIFY +Info+Tooltip) | component | event-driven (hover/focus) | `web/src/components/StatusDot.tsx:56-70` (Tooltip + `TooltipTrigger asChild` + child + `TooltipContent`) + **itself** `StatsStrip.tsx:18-35` (`TimeStatRow`) | **exact** (the Tooltip idiom has 14 in-repo call sites) |
| `web/src/components/activity/ActivityList.tsx` (MODIFY swap formatAgo) | component | transform | **itself** — `ActivityList.tsx:72-74` (the `formatAgo(task.doneAt, now)` call site) | **exact** (1-token swap on one line) |
| `web/src/components/activity/ReviewsList.tsx` (MODIFY swap formatAgo) | component | transform | **itself** — `ReviewsList.tsx:96-98` (the `formatAgo(review.completedAt, now)` call site) | **exact** (1-token swap on one line) |
| `web/src/lib/time.ts` (MODIFY +formatDateTime) | utility | transform | **itself** — `time.ts:1-11` (`formatAgo`) + `time.ts:21-33` (`formatDuration`) | **exact** (third sibling formatter, same shape) |
| `web/package.json` (MODIFY +recharts) | config | — | **itself** + `web/components.json` (shadcn registry config the CLI reads) | **exact** (one entry added by `shadcn add chart`, not by hand) |

> **Note on `chart.tsx`:** This file is **generated** by `npx shadcn@latest add chart` (D-01). The planner/executor should NOT hand-write it — the analog below describes the **shape** to expect (so the planner can spot a botched generation), and `12-RESEARCH.md §Code Examples` documents the full export surface (`ChartContainer` / `ChartConfig` / `ChartTooltip` / `ChartTooltipContent` / `ChartLegend` / `ChartLegendContent` / `ChartStyle` + the `useChart` context). After generation, treat the file as a vendored primitive and adapt `ActivityChart.tsx` to its actual exports.

---

## Pattern Assignments

### `web/src/components/ui/chart.tsx` (NEW — shadcn-generated)

**Analog:** `web/src/components/ui/tooltip.tsx` (the closest existing shadcn-generated primitive in this repo)

> The `radix-nova` preset (`components.json:3`) is a style variant of the same shadcn registry path the RESEARCH source was read from (`new-york-v4`); export surface is identical, only Tailwind class names differ. The executor reads the actually-generated file after `shadcn add chart`.

**shadcn primitive anatomy to expect** (mirror of `tooltip.tsx:1-5, 55`):
```tsx
// Source: web/src/components/ui/tooltip.tsx:1-5,55 — every shadcn-generated primitive in
// this repo follows this skeleton (verified against button.tsx, skeleton.tsx, select.tsx).
import * as React from "react"
// chart.tsx instead imports: import * as React from "react"; import * as RechartsPrimitive from "recharts"
import { cn } from "@/lib/utils"
// ...one named function declaration per exported primitive, each thin-wrapping the underlying lib
//   function ChartContainer(...) { return <RechartsPrimitive.ResponsiveContainer ... data-slot="chart-container" ... /> }
// bottom of file:
export { ChartContainer, ChartConfig, ChartTooltip, ChartTooltipContent, ChartLegend, ChartLegendContent, ChartStyle }
```

**`data-slot` convention** — every shadcn primitive in this repo sets `data-slot="<name>"` on its root (tooltip.tsx:12/21/28/40, button.tsx:58-60, skeleton.tsx:6). `chart.tsx` does the same (`data-slot="chart-container"` etc.). Do not strip these — they are how Tailwind v4 `@slot` selectors and the parent `Sidebar`/`Tabs` slots work.

**ChartConfig color-token mechanism** (from `12-RESEARCH.md §Pattern 1`):
`ChartContainer` renders a `<ChartStyle>` element that injects `[data-chart=<id>] { --color-<key>: <color>; }` from `config.<key>.color`. So `<Bar fill="var(--color-tasks)">` resolves to `config.tasks.color` — the config colors can be raw hex (e.g. `#5e719c`), they do NOT need to be pre-existing CSS vars.

---

### `web/src/components/activity/ActivityChart.tsx` (NEW — the stacked-bar chart)

**Analogs (composite — two patterns combined):**
1. **Bucketing pattern** — `web/src/components/activity/ActivityList.tsx:32-45` (`groupByProject`) — the time-axis analogue
2. **Data-prop → render pattern** — `web/src/components/activity/StatsStrip.tsx:18-35, 37-62` (a small section component with `flex flex-col gap-3` + an `uppercase tracking-wide` heading + a body)
3. **recharts composition** — no in-repo analog (first chart in the app); use `12-RESEARCH.md §Pattern 1-3` + `§Code Examples` as the source of truth

**Bucketing helper — clone-and-adapt from `groupByProject`:**
```tsx
// Source: web/src/components/activity/ActivityList.tsx:32-45 — the established
// in-repo bucketing idiom. The chart's bucketByDay is the time-axis analogue:
// same shape (Map<key, bucket>, push-only mutation, return [...map.values()]).
export function groupByProject<
  T extends { projectId: number; projectName: string },
>(rows: T[] | null | undefined): ProjectGroup<T>[] {
  const map = new Map<number, ProjectGroup<T>>();
  for (const r of rows ?? []) {              // ← null-safe iteration (Go nil → JSON null)
    let g = map.get(r.projectId);
    if (!g) {
      g = { projectId: r.projectId, projectName: r.projectName, entries: [] };
      map.set(r.projectId, g);
    }
    g.entries.push(r);
  }
  return [...map.values()];
}
```
**Adaptation for `bucketByDay`:** same skeleton, but (a) **pre-seed** N empty slots rolling backward from `now` so D-05 (always N bars, never gaps) holds; (b) key by **local-calendar** `${y}-${m}-${d}` via `getDate()`/`getMonth()`/`getFullYear()` — NOT `getUTCDate()` (Pitfall 4); (c) parse both ms- and second-precision ISO via `new Date(iso)` (native — Phase 10 `parseActivityTime` awareness). Full reference impl in `12-RESEARCH.md §Pattern 4`.

**Component anatomy — clone from `StatsStrip`:**
```tsx
// Source: web/src/components/activity/StatsStrip.tsx:37-44 — the in-repo "section"
// shape (exported function, destructured typed props, <section className="flex
// flex-col gap-3"> wrapper, uppercase tracking-wide muted heading). ActivityChart
// follows the SAME shell — only the body differs (ChartContainer instead of rows).
export function StatsStrip({ stats }: { stats: StatsBlock }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Statistics`}</h2>
      {/* …body… */}
    </section>
  );
}
```

**recharts composition** (no in-repo analog — the chart is the first in the app). Source of truth is `12-RESEARCH.md §Pattern 1` (verbatim, verified against shadcn docs + `chart.tsx` source). Skeleton:
```tsx
// Source: 12-RESEARCH.md §Pattern 1 (verified against ui.shadcn.com/docs/components/chart
// + github shadcn-ui/ui chart.tsx). NOT in-repo — the executor composes this from
// the generated chart.tsx exports + PROJECT_PALETTE.
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart";
import { PROJECT_PALETTE } from "@/lib/palette";

const chartConfig = {
  tasks:   { label: "Tasks",   color: PROJECT_PALETTE[5] }, // #5e719c muted blue (D-04)
  reviews: { label: "Reviews", color: PROJECT_PALETTE[4] }, // #46776f muted teal (D-04)
} satisfies ChartConfig;

<ChartContainer config={chartConfig} className="h-48 w-full">  {/* height class is MANDATORY (Pitfall 1) */}
  <BarChart data={dayBuckets} accessibilityLayer>
    <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 3" />
    <XAxis dataKey="label" tickLine={false} axisLine={false}
           interval={window === "week" ? 0 : 6}              /* D-06 sparse Month markers */
           tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
    <YAxis hide allowDecimals={false} />
    <ChartTooltip cursor={{ fill: "var(--accent)", opacity: 0.4 }}
                  content={<ChartTooltipContent />} />
    <Bar stackId="a" dataKey="tasks"   fill="var(--color-tasks)"   radius={4}
         shape={<ZeroDayStub />} />                            {/* D-05 zero-day baseline stub */}
    <Bar stackId="a" dataKey="reviews" fill="var(--color-reviews)" radius={4} />
  </BarChart>
</ChartContainer>
```
**Zero-day stub `shape` function** + **sparse Month `interval` semantics** — full reference impls in `12-RESEARCH.md §Code Examples` (verified against recharts Bar docs). Do NOT use `minPointSize` for the stub — recharts docs explicitly warn it is unreliable in stacked charts (`12-RESEARCH.md §Pitfall 2`).

**Color source — clone from `palette.ts`:**
```ts
// Source: web/src/lib/palette.ts:11-14 — the ONLY legal muted-hue source (v1.7 UAT).
// Index [5]=blue, [4]=teal — both dark-theme-legible, both clear WCAG AA. Do NOT
// use the saturated --chart-1..5 oklch tokens (index.css:73-77, 108-112) — Pitfall 3.
export const PROJECT_PALETTE = [
  "#9e5757", "#9c6b4b", "#8a7345", "#4e7a54", "#46776f",
  "#5e719c", "#6c6699", "#84689e", "#9c6188",
] as const;
```

**Wire types the chart reads** (`web/src/api/types.ts:122-144`):
```ts
// Source: web/src/api/types.ts:122-128 (ActivityTask) + :136-144 (ReviewDoneSummary).
// doneAt is ms-ISO; completedAt is second-ISO — both parse via new Date(iso) natively.
export interface ActivityTask { id: number; title: string; doneAt: string; projectId: number; projectName: string; }
export interface ReviewDoneSummary { number: number; title: string; completedAt: string; url: string; projectName: string; projectId: number; repo: string; }
```

---

### `web/src/pages/ActivityPage.tsx` (MODIFY — mount chart above StatsStrip)

**Analog:** itself — `ActivityPage.tsx:51-61`

**Modification site — the loaded `<div className="mt-6 flex flex-col gap-6">` stack:**
```tsx
// Source: web/src/pages/ActivityPage.tsx:52-60 (current). The chart slots in BETWEEN
// the control bar (line 53-56) and <StatsStrip> (line 57) per D-03.
// The gap-6 wrapper already gives the chart vertical rhythm for free — no extra margin.
<div className="mt-6 flex flex-col gap-6">
  <div className="flex items-center gap-2">                  {/* line 53-56 — control bar */}
    <ScopeSelector scope={scope} onScopeChange={setScope} />
    <WindowToggle window={window} onWindowChange={setWindow} />
  </div>
  {/* ↓ INSERT <ActivityChart data={data} window={window}/> HERE (D-03) ↓ */}
  <StatsStrip stats={data.stats} />                          {/* line 57 */}
  <ActivityList tasks={data.tasks ?? []} />                  {/* line 58 */}
  <ReviewsList reviews={{ ...data.reviews, prs: data.reviews.prs ?? [] }} />  {/* line 59 */}
</div>
```

**Import to add** (mirror existing imports `ActivityPage.tsx:3-7`):
```tsx
// Source: web/src/pages/ActivityPage.tsx:3-7 — existing pattern: named import from
// "@/components/activity/<Name>". Add ActivityChart alongside ActivityList/ReviewsList.
import { ActivityChart } from "@/components/activity/ActivityChart";
```

**`data` prop shape** — `ActivityPage.tsx:24` already has `const { data, isLoading, isError, refetch } = useActivity(scope, window);`. Pass `data` + `window` straight through; the chart does its own bucketing over `data.tasks` + `data.reviews.prs` (no new fetch — Phase 10 D-01).

---

### `web/src/components/activity/StatsStrip.tsx` (MODIFY — add Info icon + Tooltip on TimeStatRow)

**Analogs:**
1. **Tooltip + icon-trigger idiom** — `web/src/components/StatusDot.tsx:56-70` (the cleanest in-repo example; 13 other call sites exist — see Grep results)
2. **The row to modify** — itself, `StatsStrip.tsx:18-35` (`TimeStatRow`)

**Tooltip + trigger pattern — clone verbatim from `StatusDot`:**
```tsx
// Source: web/src/components/StatusDot.tsx:56-70 — the established in-repo Tooltip idiom
// (verified against 14 call sites: PRCard, TaskPage, sidebar, settings, task tabs, etc.).
// Shape: <Tooltip><TooltipTrigger asChild>{triggerEl}</TooltipTrigger><TooltipContent>{text}</TooltipContent></Tooltip>
return (
  <Tooltip>
    <TooltipTrigger asChild>
      {/* trigger MUST be a single element; asChild merges props into it.
          StatusDot uses a <span>; for D-08 use a <button> so the icon is
          keyboard-focusable (Tab + hover both open; delayDuration=0). */}
      <span className={cn("inline-flex shrink-0", className)}>
        <span role="img" aria-label={`Agent status: ${tooltip.toLowerCase()}`} className={cn("size-2 rounded-full shrink-0", dotClassName)} />
      </span>
    </TooltipTrigger>
    <TooltipContent>{tooltip}</TooltipContent>
  </Tooltip>
);
```

**lucide-react icon import** — verified present (`web/package.json:22` `"lucide-react": "^1.17.0"`), `Info` confirmed in RESEARCH. Import shape mirrors any of the 29 in-repo call sites (e.g. `WorktreeCleanupSection.tsx:2`):
```tsx
// Source: web/src/components/settings/WorktreeCleanupSection.tsx:2 (representative of 29 sites).
import { RefreshCw } from "lucide-react";
// For D-08:
import { Info } from "lucide-react";
```

**Tooltip imports** — `web/src/components/StatusDot.tsx:2-6`:
```tsx
// Source: web/src/components/StatusDot.tsx:2-6 — the established import shape.
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
// (TooltipProvider is NOT needed here — it's mounted ONCE at the app root:
//  web/src/components/layout/AppLayout.tsx:4,25 — <TooltipProvider delayDuration={0}>.)
```

**Modification to `TimeStatRow`** (`StatsStrip.tsx:18-35`) — add an optional `help?: string` prop and mount the `Info`+`Tooltip` beside the label. The `n === 0` em-dash guard (lines 22-29) stays UNCHANGED. Three call sites at `StatsStrip.tsx:56-58` gain a `help="…"` arg (D-08 copy verbatim from CONTEXT); the two COUNT rows at lines 42-53 are NOT touched (D-09).

**Existing `TimeStatRow` to extend** (`StatsStrip.tsx:18-35`):
```tsx
// Source: web/src/components/activity/StatsStrip.tsx:18-35 — current. Add a `help?: string`
// prop; when present, render the <Info>+<Tooltip> beside {label} inside the existing
// <span className="min-w-[5.5rem] text-muted-foreground">. The min-w may need a nudge
// (icon adds ~16px) — agent's discretion per CONTEXT.
function TimeStatRow({ label, stat }: { label: string; stat: TimeStat }) {
  return (
    <div className="flex items-baseline gap-2 text-sm">
      <span className="min-w-[5.5rem] text-muted-foreground">{label}</span>
      {stat.n === 0 ? (
        <span className="font-mono text-muted-foreground">—</span>
      ) : (
        <span className="font-mono">
          {formatDuration(stat.min)} · {formatDuration(stat.median)} ·{" "}
          {formatDuration(stat.max)}
        </span>
      )}
      <span className="text-xs text-muted-foreground">{`over ${stat.n} tasks`}</span>
    </div>
  );
}
```

---

### `web/src/components/activity/ActivityList.tsx` (MODIFY — swap formatAgo → formatDateTime)

**Analog:** itself — `ActivityList.tsx:3, 72-74`

**Import swap** (`ActivityList.tsx:3`):
```tsx
// Source: web/src/components/activity/ActivityList.tsx:3 — current.
import { formatAgo } from "@/lib/time";
// Phase 12: ALSO import formatDateTime (formatAgo stays — other callers: QuotaIndicator).
import { formatAgo, formatDateTime } from "@/lib/time";
```

**Render-site swap** (`ActivityList.tsx:72-74`):
```tsx
// Source: web/src/components/activity/ActivityList.tsx:72-74 — current call site.
// D-10: replace relative ("3h ago") with absolute ("Jul 29, 14:32").
// formatDateTime takes ONLY the iso (no `now`) — the `now = Date.now()` on line 48
// becomes unused by this call site (ReviewsList has its own `now` on line 72; check
// both before removing — `now` may still be wanted elsewhere or just delete it).
<span className="shrink-0 text-xs text-muted-foreground">
  {formatAgo(task.doneAt, now)}
</span>
// ↓ becomes ↓
<span className="shrink-0 text-xs text-muted-foreground">
  {formatDateTime(task.doneAt)}
</span>
```

**`groupByProject` stays exported and unchanged** — the chart's day-bucketing mirrors it but is a separate helper (see `ActivityChart.tsx` assignment).

---

### `web/src/components/activity/ReviewsList.tsx` (MODIFY — swap formatAgo → formatDateTime)

**Analog:** itself — `ReviewsList.tsx:6, 72, 96-98`

**Import swap** (`ReviewsList.tsx:6`):
```tsx
// Source: web/src/components/activity/ReviewsList.tsx:6 — current.
import { formatAgo } from "@/lib/time";
// Phase 12:
import { formatAgo, formatDateTime } from "@/lib/time";
```

**Render-site swap** (`ReviewsList.tsx:96-98`):
```tsx
// Source: web/src/components/activity/ReviewsList.tsx:96-98 — current call site.
// `completedAt` is second-precision gh closedAt; formatDateTime parses both precisions
// via Date.parse (see time.ts assignment).
<span className="shrink-0 text-xs text-muted-foreground">
  {formatAgo(review.completedAt, now)}
</span>
// ↓ becomes ↓
<span className="shrink-0 text-xs text-muted-foreground">
  {formatDateTime(review.completedAt)}
</span>
```

**The `now = Date.now()` on `ReviewsList.tsx:72`** becomes unused after this swap — remove it (lint will flag it). Same caveat as ActivityList: confirm no other reference first.

**D-12 HARD_DEGRADE branch (`ReviewsList.tsx:34-58`) is UNCHANGED** by this phase — the chart's degrade (D-07) is the time-axis analogue and lives in `ActivityChart.tsx`, not here.

---

### `web/src/lib/time.ts` (MODIFY — add formatDateTime sibling)

**Analog:** itself — `time.ts:1-11` (`formatAgo`) + `time.ts:21-33` (`formatDuration`)

**Sibling formatter pattern — clone the shape verbatim:**
```ts
// Source: web/src/lib/time.ts:1-11 (formatAgo) + :21-33 (formatDuration) — the established
// in-repo formatter idiom. EVERY formatter in this file:
//   1. Has a JSDoc header explaining the contract + zero-deps rationale
//      (UI-SPEC zero-deps principle — relaxed ONLY for recharts per D-02, NEVER for date libs).
//   2. Takes a primitive input (string iso | number seconds).
//   3. Guards the degenerate input FIRST (null → sentinel, NaN → sentinel).
//   4. Returns string.
/** Age of an ISO timestamp relative to `now` (ms): "12s", "7m", "1h 5m".
 *  Lifted from QuotaIndicator so the quota footer and PR cards share one
 *  implementation (no date library — UI-SPEC: zero new npm deps). */
export function formatAgo(iso: string | null, now: number): string {
  if (iso === null) return "0s";
  const secs = Math.max(0, Math.floor((now - Date.parse(iso)) / 1000));
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
}
```

**New `formatDateTime` to add** (sibling — same file, same idiom). Reference impl from `12-RESEARCH.md §Pattern 5` (MDN-verified):
```ts
// Source: 12-RESEARCH.md §Pattern 5 (verified against MDN Date.prototype.toLocaleString).
// Sibling of formatAgo/formatDuration. Zero deps (D-02 — no date-fns/dayjs/luxon).
/** Absolute "Jul 29, 14:32" — compact date + 24h time, no weekday/year (D-11).
 *  All entries fall within the 7/30-day window so year is implicit. */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return "—";   // guard: malformed gh closedAt (defense-in-depth)
  return new Date(ms).toLocaleString("en-US", {
    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false,
  });
  // en-US joins date and time with ", " → "Jul 29, 14:32". Explicit locale for
  // determinism (app is English-only); MDN warns output varies by locale (Pitfall 6).
}
```

**`formatAgo` is RETAINED** — CONTEXT D-11: still called by `QuotaIndicator` (`web/src/components/quota/QuotaIndicator.tsx`). Do NOT delete it.

---

### `web/package.json` (MODIFY — add recharts)

**Analog:** itself + `web/components.json`

> This file is **modified by the `shadcn add chart` CLI, not by hand**. The planner's action is `cd web && npx shadcn@latest add chart && npm install` (D-01). The expected diff is documented below so the planner can verify the CLI did the right thing.

**Expected diff** (one entry added to `dependencies`, alphabetical-ish — current block at `package.json:12-32`):
```json
// Source: web/package.json:12-32 (current dependencies block). shadcn add chart inserts
// "recharts" alphabetically between "react-router" and "shadcn" (or wherever the CLI
// places it — order is not load-bearing). The resolved version will be ^3.10.1
// (verified — 12-RESEARCH.md §Standard Stack).
"dependencies": {
  // …existing…
  "react-router": "^7.17.0",
  "recharts": "^3.10.1",          // ← NEW (sole new dep this phase — D-02)
  "remark-gfm": "^4.0.1",
  // …existing…
}
```

**shadcn registry config (read-only reference)** — `web/components.json:1-25`:
- `"style": "radix-nova"` (line 3) — the preset the CLI generates against
- `"aliases.ui": "@/components/ui"` (line 18) — confirms `chart.tsx` lands at `web/src/components/ui/chart.tsx`
- `"tailwind.css": "src/index.css"` (line 8) — the `--chart-1..5` tokens the CLI may touch (already present at `index.css:73-77, 108-112`; Pitfall 3 says DO NOT use them for the segments — use `PROJECT_PALETTE` hexes directly in `ChartConfig`)

**React 19 compatibility verified** — `react@^19.2.6` (`package.json:24`); recharts v3 peer-declares `react: '^19.0.0'` (`12-RESEARCH.md §Standard Stack`).

---

## Shared Patterns

### Tooltip (the project's only tooltip library — reused, not added)

**Source:** `web/src/components/ui/tooltip.tsx` (the existing shadcn Tooltip; `delayDuration=0` default at line 7)
**Apply to:** `StatsStrip.tsx` `TimeStatRow` (D-08, the only NEW tooltip site this phase)

**Provider is already mounted at the app root** — `web/src/components/layout/AppLayout.tsx:4,25`:
```tsx
// Source: web/src/components/layout/AppLayout.tsx:4,25 — single mount, wraps the whole app.
import { TooltipProvider } from "@/components/ui/tooltip";
// …
<TooltipProvider delayDuration={0}>  {/* every tooltip in the app inherits delayDuration=0 */}
```
**Implication:** `StatsStrip.tsx` does NOT need its own `<TooltipProvider>` — just `<Tooltip><TooltipTrigger asChild>…</TooltipTrigger><TooltipContent>…</TooltipContent></Tooltip>`. (Same as `StatusDot.tsx`, `PRCard.tsx`, `TaskPage.tsx`, etc.)

**Exports** (`tooltip.tsx:55`):
```tsx
export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger }
```

### Formatter discipline (zero-deps, hand-rolled in `lib/time.ts`)

**Source:** `web/src/lib/time.ts` (the project's ONLY time/format utility)
**Apply to:** the new `formatDateTime` (D-11)

The UI-SPEC zero-deps principle (Phase 11) is relaxed for `recharts` ONLY (D-02). The new formatter stays hand-rolled — NO `date-fns`/`dayjs`/`luxon`. Every formatter in `time.ts` follows the same idiom (see assignment above): JSDoc header, primitive input, degenerate-input guard first, `string` return.

### Muted palette (single source of truth)

**Source:** `web/src/lib/palette.ts:11-14` (`PROJECT_PALETTE`, mirrors Go `internal/api/icons.go`)
**Apply to:** the two chart segment colors in `ChartConfig` (D-04)

```ts
export const PROJECT_PALETTE = [
  "#9e5757", "#9c6b4b", "#8a7345", "#4e7a54", "#46776f",  // [4] = muted teal → reviews
  "#5e719c", "#6c6699", "#84689e", "#9c6188",              // [5] = muted blue → tasks
] as const;
```
**Never** use the saturated `--chart-1..5` oklch tokens (`index.css:73-77, 108-112`) for the chart segments — Pitfall 3. The v1.7 UAT-muted palette decision (CONTEXT D-04) stands.

### Degrade-don't-break (carried from Phase 11 D-12)

**Source:** `web/src/components/activity/ReviewsList.tsx:34-58` (the established HARD_DEGRADE pattern)
**Apply to:** `ActivityChart.tsx` reviews segment (D-07)

The chart's reviews segment reads `data.reviews.prs`. Under HARD_DEGRADE (`disabled`/`no_gh`/`auth_required`/`error`), `prs` is empty → the reviews bucket stays 0 on every bar automatically. **No special-casing needed in the chart** — the existing data shape carries the degrade signal. The chart NEVER breaks or hides; it renders tasks-only bars (D-07).

### Section heading treatment

**Source:** `StatsStrip.tsx:40` + `ActivityList.tsx:53` + `ReviewsList.tsx:76` (consistent across all three Phase-11 activity sections)
**Apply to:** `ActivityChart.tsx` heading (if it has one — agent's discretion)

```tsx
<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Section name`}</h2>
```
This is the **top-level section heading** treatment. Project-name subheadings (`ActivityList.tsx:60-62`) are NOT uppercase — the uppercase tracking-wide is reserved for section headings only (Phase 11 UI-SPEC).

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `web/src/components/ui/chart.tsx` (recharts composition internals) | UI primitive | render | **No charting primitive exists in the repo** — this is the first chart. The shadcn-generated `chart.tsx` is the analog for the *file anatomy* (matches `tooltip.tsx`), but the recharts composition *inside* it has no in-repo precedent. **Source of truth:** `12-RESEARCH.md §Code Examples` (read from `github.com/shadcn-ui/ui chart.tsx` this session). |
| `ActivityChart.tsx` recharts composition | component | render | **No chart component exists in the repo.** The bucketing helper has an exact analog (`groupByProject`); the shadcn-section shell has an exact analog (`StatsStrip`); but the recharts `<BarChart>`/`<Bar stackId>`/`<ChartTooltip>` composition is net-new. **Source of truth:** `12-RESEARCH.md §Pattern 1-3 + §Code Examples` (verified against shadcn docs + recharts docs + `chart.tsx` source). |

> Both "no analog" items are covered exhaustively by `12-RESEARCH.md` with verified code excerpts. The planner should reference RESEARCH §Code Examples directly in the plan actions for these two surfaces rather than searching the codebase further.

---

## Metadata

**Analog search scope:**
- `web/src/components/ui/*.tsx` (20 shadcn primitives — `tooltip.tsx` selected as closest size/style match for `chart.tsx`)
- `web/src/components/activity/*.tsx` (4 Phase-11 components — all read; `StatsStrip`/`ActivityList` are the analogs for `ActivityChart`)
- `web/src/components/StatusDot.tsx` (cleanest Tooltip+trigger example; 13 other call sites confirmed via Grep)
- `web/src/lib/time.ts`, `web/src/lib/palette.ts` (the formatters + palette sources)
- `web/src/pages/ActivityPage.tsx` (the page shell being modified)
- `web/src/api/types.ts:100-188` (the wire contract — `ActivityTask`, `ReviewDoneSummary`, `StatsBlock`, `TimeStat`, `ActivityResponse`)
- `web/src/api/queries.ts:85-108` (`useActivity` — confirms no new query needed)
- `web/src/components/layout/AppLayout.tsx` (confirms `TooltipProvider` is already mounted app-wide)
- `web/package.json`, `web/components.json`, `web/src/index.css` (dependency + shadcn config + chart-token sources)

**Files scanned:** 14 source files + 4 planning/config artifacts (CONTEXT, RESEARCH, components.json, Phase 11 PATTERNS for style reference)

**Pattern extraction date:** 2026-08-01

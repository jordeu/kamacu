# Phase 12: Activity Chart & Stat Tooltips - Context

**Gathered:** 2026-08-01
**Status:** Ready for planning

<domain>
## Phase Boundary

The **frontend-only** chart + tooltip extension to the Phase 11 Activity page. No backend, no SQL, no migration — the chart buckets the **existing client-side** `data.tasks[].doneAt` and `data.reviews.prs[].completedAt` already fetched by the one `useActivity(scope, window)` query (Phase 10 D-01 / Phase 11 D-08). Nothing new is fetched.

Delivers:
1. A **daily stacked-bar chart** (tasks-done + reviews-done per day; one bar per calendar day; Week window = 7 bars, Month window = 30 bars) that recomputes on scope/window change — communicates "daily cadence at a glance."
2. **Explanatory tooltips on the StatsStrip rows** — the three time-stat rows (Cycle, In-progress dwell, In-review dwell) each get an info affordance explaining the metric + the min·median·max legend.
3. **Per-entry completion times** — each tasks-done and reviews-done entry shows an **absolute** completion timestamp beside it (replacing today's relative `formatAgo`).

**Built on existing foundations (no new API):**
- Phase 11's Activity page shell (`ActivityPage.tsx`), `StatsStrip`, `ActivityList`, `ReviewsList`, `ScopeSelector`, `WindowToggle`, and the `useActivityView` localStorage hook (scope/window persist).
- Phase 10's combined `GET /api/activity` → `{tasks, reviews:{state,…,prs}, stats}`. The chart's per-day buckets are a **client-side bucketing pass** over `tasks[].doneAt` + `reviews.prs[].completedAt`, exactly the data the lists already render. `reviews.state` carries degrade (Phase 11 D-12); the reviews segment reads as 0 per bar when degraded (carry-forward, see Decisions D-09).
- The existing shadcn `Tooltip` (`web/src/components/ui/tooltip.tsx`) is reused for the stat-strip tooltips — no new tooltip/popover library.

**Out of this phase (per REQUIREMENTS.md Out of Scope, unchanged):** custom date ranges / "All time" (ACTFUT-01), CSV/JSON export (ACTFUT-03), per-agent/workspace breakdown (ACTFUT-04), events beyond done/merged (ACTFUT-05), time stats for reviews (STATS-04 — permanently out), multi-user/team stats. The chart is the *one* previously-deferred viz (ACTFUT-02) now pulled into scope by reopening v1.12 for Phase 12; richer viz beyond this single stacked bar remains deferred.

**Requirements note:** ROADMAP lists Phase 12's requirements as **TBD** (candidates: `daily-chart`, `stat-tooltip`, `entry-time-tooltip`). The v1.12 REQUIREMENTS.md traceability was complete at 15/15 for Phases 10–11; Phase 12 adds scope beyond those. Formal requirement IDs are a planning artifact — the three Decisions categories below map 1:1 to the three candidate capabilities.

</domain>

<decisions>
## Implementation Decisions

### Chart library & dependency boundary (daily-chart)

- **D-01:** **recharts via the shadcn chart pattern.** `shadcn add chart` pulls in `recharts` and generates `web/src/components/ui/chart.tsx` (the shadcn `ChartContainer` / `ChartConfig` / `ChartTooltip` / `ChartTooltipContent` primitives). This matches PROJECT.md's milestone-status intent ("Chart via recharts (shadcn chart pattern)"). recharts gives the responsive container, Cartesian axes/grid, stacked bars, and a hover tooltip out of the box — all of which a hand-rolled SVG would otherwise rebuild.
- **D-02:** **recharts is the ONLY new npm dependency this phase adds.** The Phase-11 "zero new npm deps" UI-SPEC principle (`web/src/lib/time.ts` comment; `11-UI-SPEC.md` §14/§197) is **relaxed for this phase specifically** — it was Phase-11-scoped to numeric readouts at a time when charts were explicitly deferred (ACTFUT-02). Charts are now in scope, so recharts is the documented, deliberate exception. **The stat-strip tooltips and per-entry times reuse the EXISTING shadcn `Tooltip`** (`web/src/components/ui/tooltip.tsx`) — no new tooltip/popover library. The zero-deps discipline still holds for every other surface in this phase (the new time formatter is hand-rolled in `lib/time.ts`, sibling of `formatAgo`/`formatDuration`).

### Chart visual contract (daily-chart)

- **D-03:** **Chart sits ABOVE the StatsStrip** — page flow becomes: control bar (scope + window toggle) → **chart** → StatsStrip → tasks-done → reviews-done. "Daily cadence at a glance" is the first thing seen after the controls, before the numeric stats. (Supersedes Phase 11 D-08's "stats strip on top" ordering for this page; the lists-below stacking is otherwise unchanged.)
- **D-04:** **Tasks segment on the BOTTOM (stable base, the primary work signal), reviews segment stacked ON TOP.** Two **muted, dark-theme-legible hues** that distinguish the two series at a glance (e.g. tasks = a foreground-ish muted slate/zinc; reviews = a distinct muted accent). Exact tokens are the agent's call — draw from the existing muted palette (`web/src/lib/palette.ts`) and the shadcn chart `ChartConfig` color tokens; do NOT introduce bright Tailwind-600 hues (the v1.7 UAT muted-palette decision stands). The Activity page was monochrome in Phase 11 (per `11-UI-REVIEW.md`); the chart is the one surface introducing two segment colors, kept muted.
- **D-05:** **All N bars are always present at their calendar slot (7 for Week, 30 for Month); a day with zero tasks AND zero reviews renders as a minimal baseline stub (1–2px / muted tick), NOT a gap.** The rhythm must read as a rhythm, including quiet days — critical for Month (30 slots) so the x-axis stays a stable calendar timeline rather than a scatter of present days.
- **D-06:** **X-axis labels:** Week (7 bars) = a short weekday label per bar (Mon, Tue, …). Month (30 bars) = sparse date markers (e.g. every Monday / first-of-week) so the timeline is orientable without crowding — 30 per-bar labels are too dense. **Hover tooltip on every bar** shows the full breakdown: absolute date + per-series counts, e.g. `Tue Jul 29: 3 tasks · 1 review` (uses the shadcn `ChartTooltip`/`ChartTooltipContent` from D-01).
- **D-07:** **Chart degrade follows the established pattern (NOT a new decision — carrying forward Phase 11 D-12 + degrade-don't-break).** When `reviews.state` is in the HARD_DEGRADE set (`disabled`/`no_gh`/`auth_required`/`error`), the reviews segment reads as 0 on every bar — the chart renders tasks-only bars; it never breaks or hides. `partial` renders the successful repos' reviews normally. No special-casing beyond reading the already-fetched `reviews.prs`.

### StatsStrip tooltips (stat-tooltip)

- **D-08:** **Each of the three time-stat rows (Cycle, In-progress dwell, In-review dwell) gets a small `Info` (i) lucide icon beside its label.** Hovering the icon (or focusing it via keyboard) opens the EXISTING shadcn `Tooltip`. The tooltip content explains BOTH the metric definition AND the min·median·max legend. Exact copy (agent may refine wording):
  - **Cycle:** `Time from In Progress → Done. min · median · max = fastest · typical · slowest.`
  - **In progress:** `Time spent in the In Progress column. min · median · max = fastest · typical · slowest.`
  - **In review:** `Time spent in the In Review column. min · median · max = fastest · typical · slowest.`
  The icon is the trigger (not whole-row hover) so the row doesn't read as hoverable/link-like — the icon signals "there's an explanation here."
- **D-09:** **Tooltips on the THREE time-stat rows ONLY.** The two lead COUNTS (`stats.taskCount`, `stats.reviewCount`) are self-evident and get NO tooltip. Matches the phase intent ("make every number self-explanatory") — the counts already are; the cryptic ones are the cycle/dwell readouts.

### Per-entry completion time (entry-time-tooltip → entry-time-display)

- **D-10:** **Replace the relative `formatAgo` ("3h ago") with an ABSOLUTE completion timestamp shown directly beside each entry — no tooltip, no relative.** This supersedes the literal ROADMAP phrasing ("tooltips … and the per-entry completion times") for entries: showing the absolute time directly is MORE self-explanatory than a tooltip on a relative time, which is the deeper intent. (The StatsStrip half of the phase keeps its tooltips per D-08.) **Same treatment for tasks (`doneAt`) and reviews (`completedAt`).**
- **D-11:** **Format: `Jul 29, 14:32`** — compact date + time, no weekday, no year (all entries fall within the 7/30-day rolling window, so the year is implicit and the weekday is recoverable from the chart). Implemented as a **new hand-rolled formatter in `web/src/lib/time.ts`** (sibling of `formatAgo`/`formatDuration`; zero deps — D-02). `formatAgo` is no longer called by `ActivityList`/`ReviewsList` but **stays in `time.ts`** for its other callers (e.g. `QuotaIndicator`).

### the agent's Discretion

- **Exact recharts composition** — `BarChart` + stacked `Bar` (`stackId`) + `XAxis`/`YAxis` + `ChartTooltip`; the `ChartConfig` token names and the precise muted hex values for the two segments (draw from `web/src/lib/palette.ts` + shadcn chart tokens per D-04).
- **The sparse-marker rule for Month x-axis** — every Monday vs. every Nth bar vs. first-of-week; pick what stays legible at the `max-w-[640px]` column width.
- **The per-day bucketing helper** — how `tasks[].doneAt` + `reviews.prs[].completedAt` (mixed ms- and second-precision ISO) bucket into N day-slots rolling from `now`; reuse the precision-mismatch awareness from `parseActivityTime` (Phase 10) on the client side. Bucketing is pure client-side over already-fetched data.
- **The new formatter's exact name** (`formatDateTime` / `formatCompleted` / similar) and locale handling — `Date.prototype.toLocaleString` with a fixed format is fine; keep it deterministic and dependency-free.
- **Exact tooltip copy** (D-08) — the wording above is the contract; minor phrasing refinements are fine.
- **Chart height** at the `max-w-[640px]` width and whether the control bar becomes sticky — follow existing visual language (Phase 11 left the control bar non-sticky).
- **Empty-window state** (zero tasks AND zero reviews in the whole window) — the chart should still render its N-bar skeleton (all stubs) rather than collapse; pick the cleanest empty rendering consistent with the lists' muted empty states.

### Folded Todos

None — `todo.match-phase` returned no matches for Phase 12.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 12: Activity Chart & Stat Tooltips" — the goal (daily stacked-bar + stat tooltips + per-entry times) and the explicit "no backend change (reuses client-side task/review data)" constraint. Note Requirements = TBD (candidates `daily-chart`, `stat-tooltip`, `entry-time-tooltip`).
- `.planning/REQUIREMENTS.md` § "v1.12 Requirements" + **Out of Scope** table + **ACTFUT-01..05** — the chart is the one ACTFUT-02 item now in scope; everything else deferred there stays deferred. STATS-04 (no time stats for reviews) stands.

### Phase 11 — the page this phase EXTENDS (READ FIRST)
- `.planning/phases/11-activity-page-controls/11-CONTEXT.md` — the direct predecessor. Especially **D-08** (stacked column order — now superseded by D-03 above), **D-09/D-10/D-11** (the `min · median · max` rows + `formatDuration` + em-dash-on-empty the chart sits beside), **D-12** (degrade-don't-break → chart degrade D-07), **D-13** (`groupByProject` bucketing pattern the chart's day-bucketing mirrors).
- `.planning/phases/11-activity-page-controls/11-UI-SPEC.md` §14/§197 — the **"zero new npm deps"** principle D-02 relaxes for recharts (and only recharts). Read to understand what's being deliberately excepted.

### The Activity page source (the files this phase modifies)
- `web/src/pages/ActivityPage.tsx` — the page shell; the chart slots in above `<StatsStrip>` (D-03). The `data.tasks`/`data.reviews`/`data.stats` already in scope here feed the chart with zero new fetches.
- `web/src/components/activity/StatsStrip.tsx` — `TimeStatRow` is where the `Info` icon + `Tooltip` mount (D-08); the `n === 0` em-dash guard (D-11 of Phase 11) stays.
- `web/src/components/activity/ActivityList.tsx` — the tasks-done list; `formatAgo(task.doneAt, now)` → the new absolute formatter (D-10/D-11). Also exports `groupByProject` (the bucketing pattern).
- `web/src/components/activity/ReviewsList.tsx` — the reviews-done list; `formatAgo(review.completedAt, now)` → the new absolute formatter. Note `completedAt` is second-precision gh `closedAt`.
- `web/src/lib/time.ts` — `formatAgo` + `formatDuration`; the new absolute formatter lives here as a sibling (D-11).

### Frontend data layer + UI primitives (reused, not new)
- `web/src/api/types.ts:122-187` — `ActivityTask` (`doneAt`), `ReviewDoneSummary` (`completedAt`), `StatsBlock`/`TimeStat` — the exact wire shapes the chart buckets and the tooltips annotate.
- `web/src/api/queries.ts` — `useActivity(scope, window)` (`queryKey: ["activity", scope, window]`); the chart reads the same `data`, no new query.
- `web/src/components/ui/tooltip.tsx` — the EXISTING shadcn Tooltip reused for D-08 (no new tooltip lib per D-02).
- `web/src/components/ui/` — shadcn primitives; `chart.tsx` will be ADDED here by `shadcn add chart` (D-01).
- `web/src/lib/palette.ts` — the muted desaturated palette source for the two segment colors (D-04); mirrored from Go `internal/api/icons.go`.

### Dependency surface
- `web/package.json` — current deps (NO charting lib present today). `recharts` is added by `shadcn add chart` (D-01/D-02); it is the sole new dependency.

### Established project patterns (constraints, not files)
- **One endpoint → one TanStack query, one loading state** (Phase 10 D-01) — the chart rides the existing `useActivity` data; no new query, no new loading state beyond the page's existing skeleton.
- **Degrade rides in `reviews.state`, always HTTP 200** — chart reviews segment reads 0 when degraded (D-07).
- **Muted palette, dark-theme legible** — the v1.7 UAT-muted palette decision; no bright Tailwind-600 hues (D-04).
- **No toasts** — inline/muted/icon feedback only.
- **Zero-deps discipline for everything except recharts** (D-02) — hand-roll formatters/bucketing; no date library.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`ActivityPage.tsx`** — the page shell the chart slots into; already holds `data` (tasks/reviews/stats) from `useActivity`.
- **`StatsStrip.tsx` `TimeStatRow`** — the exact row component that gains the `Info` icon + `Tooltip` (D-08); its `n === 0` em-dash guard is preserved.
- **`ActivityList.tsx` / `ReviewsList.tsx`** — the two entry lists whose `formatAgo(...)` calls become the new absolute formatter (D-10). `groupByProject` (exported from `ActivityList`) is the bucketing-pattern template for the chart's day buckets.
- **`web/src/lib/time.ts`** — `formatAgo`/`formatDuration` siblings; the new absolute formatter (`formatDateTime`-style) lives here, zero deps.
- **`web/src/components/ui/tooltip.tsx`** — the existing shadcn Tooltip, reused as-is for D-08.
- **`web/src/lib/palette.ts`** — muted palette for the two segment colors (D-04).
- **`useActivity` + the activity wire types** — the data source; no new fetch.

### Established Patterns
- **Client-side bucketing over server-annotated data** — `groupByProject` already buckets the lists by project; the chart's per-day bucketing is the time-axis analogue over `doneAt`/`completedAt`.
- **Mixed-precision ISO awareness** — `doneAt` is ms-precision (Kamacu `strftime`), `completedAt` is second-precision (gh `closedAt`); bucket by parsed `Date`, never lexical-compare across precisions (Phase 10 `parseActivityTime` / STATE decisions).
- **shadcn primitive reuse over new libs** — `Tooltip` reused; `chart.tsx` added via the canonical shadcn command (D-01).
- **Muted, monochrome-leaning aesthetic** — the chart's two hues stay muted (D-04).
- **`location.pathname` active state / localStorage view state** — unchanged by this phase.

### Integration Points
- **New `web/src/components/ui/chart.tsx`** — generated by `shadcn add chart` (pulls `recharts`).
- **New chart component** (e.g. `web/src/components/activity/ActivityChart.tsx`) — mounted in `ActivityPage.tsx` between the control bar and `<StatsStrip>`.
- **`StatsStrip.tsx` `TimeStatRow`** — gains the `Info` icon + `Tooltip` wrapper (D-08).
- **`ActivityList.tsx` + `ReviewsList.tsx`** — swap `formatAgo` → the new absolute formatter (D-10).
- **`web/src/lib/time.ts`** — new formatter sibling.
- **`web/package.json`** — `recharts` added (sole new dep).

</code_context>

<specifics>
## Specific Ideas

- **The load-bearing decision was the library.** PROJECT.md's milestone status named "recharts (shadcn chart pattern)," but the codebase carries a documented "zero new npm deps" UI-SPEC principle (Phase 11). The user confirmed recharts is the deliberate, sole exception — charts were deferred in Phase 11 (ACTFUT-02) and are now in scope, so the Phase-11-scoped zero-deps rule is relaxed for this one dependency and nothing else (D-01/D-02). A hand-rolled SVG was the credible zero-dep alternative and was consciously not chosen.
- **"Cadence at a glance" drives every chart decision.** All N bars always present (D-05), chart above the stats (D-03), weekday labels in Week (D-06) — the chart exists to show the rhythm of work, including quiet days, as the first thing you see.
- **D-10 deliberately supersedes the literal ROADMAP wording for entries.** "Per-entry completion times" is implemented as a directly-shown absolute timestamp (replacing relative), not a tooltip — the user explicitly rejected "keep relative + absolute-on-hover" and "show both." This better serves the phase's "make every number self-explanatory" intent. The StatsStrip half keeps its tooltips (D-08). Flag at UAT that the entry implementation is direct-display, not tooltip, in case the wording matters to verification.
- **The chart reuses the existing query's data — emphasize zero new fetches.** Per-day buckets are pure client-side over `data.tasks`/`data.reviews.prs` already on the page.
- **Phase 12's formal requirements are TBD** (ROADMAP). The three decisions categories (daily-chart / stat-tooltip / entry-time) map to the three candidates; planning should define the requirement IDs and update REQUIREMENTS.md traceability (currently 15/15 complete for Phases 10–11 only).

</specifics>

<deferred>
## Deferred Ideas

- **Richer viz beyond this single stacked bar** — throughput trend lines, per-agent/per-workspace breakdown charts, etc. — remain ACTFUT-02/ACTFUT-04. Phase 12 ships exactly one daily stacked-bar.
- **Preferring a tooltip-on-relative for per-entry times** (the rejected D-10 alternative) — not wanted; the user chose direct absolute display.
- **Custom date ranges / "All time" preset** — ACTFUT-01. Week/Month presets drive the chart like the rest of the page.
- **CSV/JSON export of activity/chart data** — ACTFUT-03.
- **Per-agent or per-workspace breakdown** (chart segments by agent/workspace, or separate charts) — ACTFUT-04. The chart's two segments are tasks vs reviews only.
- **Activity for events beyond done/merged** (e.g. tasks moved to In Review) — ACTFUT-05. The chart counts done + merged/closed only.
- **Distinguishing merged vs closed in the reviews segment** — Phase 10 D-05 treats both as "completed"; the chart's reviews segment follows (one color, no merge/closed split).

None were pulled in beyond the phase boundary — discussion stayed within Activity Chart & Stat Tooltips.

</deferred>

---

*Phase: 12-activity-chart-tooltips*
*Context gathered: 2026-08-01*

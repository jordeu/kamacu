# Phase 12: Activity Chart & Stat Tooltips - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-01
**Phase:** 12-activity-chart-tooltips
**Areas discussed:** Chart library, Chart visual contract, Stats-strip tooltip content & trigger, Per-entry completion time

---

## Chart library

| Option | Description | Selected |
|--------|-------------|----------|
| recharts via shadcn chart | `shadcn add chart` pulls recharts + creates web/src/components/ui/chart.tsx (ChartContainer, ChartConfig, ChartTooltip, ChartTooltipContent). Responsive container, axes, grid, hover tooltip, stacked bars out of the box. Matches PROJECT.md's stated intent. Cost: a real new dependency, breaking Phase-11's zero-deps discipline. | ✓ |
| Hand-rolled SVG (zero deps) | Honor the documented zero-deps principle. ~150-200 lines of inline SVG + a small bucketing util; reuse existing Tooltip for hover. Full control, no dependency. Cost: axes/grid/responsive/hover all hand-built and maintained. | |
| You decide | Let the agent/planner weigh recharts-vs-SVG during research; lock only the constraint (daily stacked bar, within the Activity column). | |

**User's choice:** recharts via shadcn chart
**Notes:** PROJECT.md's milestone status (updated at v1.12 reopen) named recharts; the documented "zero new npm deps" UI-SPEC principle was Phase-11-scoped to numeric readouts when charts were deferred (ACTFUT-02). Charts now in scope → recharts is the deliberate exception.

---

**Dependency boundary follow-up:**

| Option | Description | Selected |
|--------|-------------|----------|
| recharts only; tooltips reuse existing | recharts is the ONLY new npm dependency; stat-strip + per-entry tooltips reuse the EXISTING shadcn Tooltip (web/src/components/ui/tooltip.tsx). Phase-11 zero-deps principle holds for everything else. | ✓ |
| You decide | Don't pre-constrain the dependency surface. | |

**User's choice:** recharts only; tooltips reuse existing
**Notes:** Locks D-02 — recharts is the sole new dep; the new time formatter stays hand-rolled in lib/time.ts.

---

## Chart visual contract

| Option | Description | Selected |
|--------|-------------|----------|
| Above the StatsStrip | Chart between control bar and StatsStrip — "daily cadence at a glance" first, then numeric stats, then lists. | ✓ |
| Below the StatsStrip | StatsStrip stays on top (Phase 11 D-08); chart between stats and tasks-done list. | |
| You decide | Agent picks placement. | |

**User's choice:** Above the StatsStrip
**Notes:** Supersedes Phase 11 D-08's "stats strip on top" ordering for this page. New flow: control bar → chart → stats → tasks → reviews.

---

**Stacking order + segment colors:**

| Option | Description | Selected |
|--------|-------------|----------|
| Tasks bottom, reviews top; two muted hues | Tasks = stable base (primary work signal), reviews stacked on top. Two muted dark-theme-legible hues (tasks foreground-ish muted slate/zinc, reviews a distinct muted accent). Keeps monochrome discipline with enough contrast to separate at a glance. | ✓ |
| Reviews bottom, tasks top | Same two-hue treatment, reversed stack. | |
| You decide | Agent picks order + colors from palette.ts + shadcn chart tokens; lock "two distinguishable segments, dark-theme legible." | |

**User's choice:** Tasks bottom, reviews top; two muted hues
**Notes:** Exact tokens are agent's discretion; draw from web/src/lib/palette.ts (v1.7 muted palette) — no bright Tailwind-600 hues.

---

**Empty-day rendering:**

| Option | Description | Selected |
|--------|-------------|----------|
| Zero-height stub, no gaps | All N bars always present (7 Week / 30 Month) at calendar slot; zero-activity days = minimal baseline stub (1-2px muted tick), not a gap. Rhythm reads as a rhythm, including quiet days. | ✓ |
| Gaps for empty days | Days with no activity render as a visible gap. Emphasizes active days but breaks calendar-grid reading in Month. | |
| You decide | Agent picks the empty-day treatment. | |

**User's choice:** Zero-height stub, no gaps
**Notes:** Critical for Month (30 slots) so the x-axis stays a stable timeline.

---

**X-axis labels + hover tooltip:**

| Option | Description | Selected |
|--------|-------------|----------|
| Week=weekdays, Month=sparse markers, hover=full breakdown | Week (7 bars) = short weekday label per bar. Month (30 bars) = sparse date markers (every Monday / first-of-week). Hover tooltip on every bar = absolute date + per-series counts ("Tue Jul 29: 3 tasks · 1 review"). | ✓ |
| No axis labels; hover only | No x-axis labels (Week or Month); rely entirely on hover tooltip. Cleanest but disorienting in Month. | |
| You decide | Agent picks label density + tooltip format; lock "hover shows per-day task + review counts." | |

**User's choice:** Week=weekdays, Month=sparse markers, hover=full breakdown
**Notes:** Uses shadcn ChartTooltip/ChartTooltipContent from D-01.

**Carry-forward (not a question):** Chart degrade when reviews.state is in HARD_DEGRADE → reviews segment reads 0 per bar; tasks-only bars still render. Follows Phase 11 D-12 + degrade-don't-break.

---

## Stats-strip tooltip content & trigger

| Option | Description | Selected |
|--------|-------------|----------|
| Info icon + metric definition + min·med·max legend | Small `Info` (i) lucide icon beside each row label; hover/focus opens existing shadcn Tooltip. Content explains BOTH the metric (e.g. Cycle = "Time from In Progress → Done") AND the min·median·max legend ("fastest · typical · slowest"). Discoverable via the icon. | ✓ |
| Hover-on-label, no icon; same content | Hover anywhere on the row's label opens the tooltip (no icon). Less clutter but less discoverable. | |
| Info icon, metric definition only | Info icon, but tooltip explains ONLY the metric definition, not the min·median·max legend. | |

**User's choice:** Info icon + metric definition + min·med·max legend
**Notes:** Trigger is the icon (not whole-row hover) so the row doesn't read as hoverable/link-like.

---

**Tooltip scope (which rows):**

| Option | Description | Selected |
|--------|-------------|----------|
| Time-stat rows only | Tooltips on the three time-stat rows (Cycle, In progress, In review) — the cryptic ones. The two lead COUNTS get NO tooltip (self-evident). | ✓ |
| All rows incl. counts | Tooltips on all five rows, including counts (e.g. reviews count notes degrade/best-effort when reviews.state degraded). | |
| You decide | Agent decides which rows get tooltips. | |

**User's choice:** Time-stat rows only
**Notes:** Matches "make every number self-explanatory" — the counts already are.

---

## Per-entry completion time

| Option | Description | Selected |
|--------|-------------|----------|
| Keep relative; absolute on hover | Keep existing relative time visible ("3h ago"); add hover/focus tooltip showing absolute timestamp ("Mon Jul 29, 14:32"). Same for tasks + reviews. | |
| Replace relative with absolute | Replace relative entirely with an absolute timestamp beside each entry ("Jul 29, 14:32"). No tooltip. Loses quick "how recent" signal. | ✓ |
| Show both, no tooltip | Both side-by-side ("3h ago · Jul 29 14:32"). No tooltip. Most info-dense but noisier. | |
| You decide | Agent picks display strategy + format. | |

**User's choice:** Replace relative with absolute
**Notes:** Deliberately supersedes the literal ROADMAP "tooltip … per-entry completion times" wording for entries — direct absolute display is more self-explanatory than a tooltip on a relative time. The StatsStrip half keeps its tooltips. Flag at UAT that entries are direct-display, not tooltip.

---

**Timestamp format:**

| Option | Description | Selected |
|--------|-------------|----------|
| Date + time: "Jul 29, 14:32" | Compact date + time, no weekday, no year (all entries within 7/30-day window). New hand-rolled formatter in web/src/lib/time.ts (zero deps). | ✓ |
| Weekday + date + time | "Mon Jul 29, 14:32" — adds weekday prefix. Longer (truncation risk in narrow column). | |
| Date only: "Jul 29" | Date only, no time. Cleanest but loses time-of-day signal. | |
| You decide | Agent picks the exact format string. | |

**User's choice:** Date + time: "Jul 29, 14:32"
**Notes:** New formatter in lib/time.ts (sibling of formatAgo/formatDuration). formatAgo no longer used by ActivityList/ReviewsList but stays for QuotaIndicator.

---

## the agent's Discretion

- Exact recharts composition (BarChart + stacked Bar + axes + ChartTooltip) and ChartConfig token names + precise muted hex values (draw from palette.ts).
- The sparse-marker rule for Month x-axis (every Monday vs. every Nth bar vs. first-of-week).
- The per-day bucketing helper (mixed ms-/second-precision ISO → N day-slots rolling from now).
- The new formatter's exact name and locale handling (toLocaleString fixed format; deterministic, dependency-free).
- Exact stat-tooltip copy (the D-08 wording is the contract; minor refinements fine).
- Chart height at max-w-[640px]; whether the control bar becomes sticky.
- Empty-window state (chart renders N-bar skeleton of stubs vs. collapses).

## Deferred Ideas

- Richer viz beyond this single stacked bar (throughput trend lines, etc.) — ACTFUT-02.
- Tooltip-on-relative for per-entry times (the rejected alternative) — not wanted.
- Custom date ranges / "All time" — ACTFUT-01.
- CSV/JSON export — ACTFUT-03.
- Per-agent/per-workspace breakdown (chart segments by agent/workspace) — ACTFUT-04.
- Activity for events beyond done/merged — ACTFUT-05.
- Distinguishing merged vs closed in the reviews segment — Phase 10 D-05 treats both as "completed."

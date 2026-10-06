import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import type { BarShapeProps } from "recharts";

import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { PROJECT_PALETTE } from "@/lib/palette";
import type { ActivityResponse } from "@/api/types";
import { bucketByDay, type DayBucket } from "./bucketByDay";

/**
 * Phase 12 — the daily stacked-bar chart (CONTEXT D-03..D-07). Renders one bar
 * per calendar day (at least 7 for Week, 30 for Month), stacking tasks (bottom, muted
 * blue) + reviews (top, muted teal). A day with zero activity renders a 2px
 * baseline stub (D-05) — NEVER a gap. Hovering a bar shows the absolute date +
 * per-series counts (D-06). Reviews degrade silently to 0 under HARD_DEGRADE
 * (D-07): `data.reviews.prs` is empty → 0 per bucket → tasks-only bars; the
 * chart never breaks or hides.
 *
 * Partial boundary days are included so every API completion appears.
 * The chart recomputes its day buckets from the SAME `data` the page's single
 * `useActivity(scope, window)` query returns — NO new fetch, NO new query. It
 * slots ABOVE `<StatsStrip>` per D-03.
 *
 * Source of truth for the recharts composition: 12-RESEARCH §Pattern 1-3 +
 * §Code Examples (verified against shadcn docs + recharts docs + chart.tsx
 * source). Bucketing lives in bucketByDay.ts so its count invariant can be
 * tested independently of the chart renderer.
 */

/** Tooltip header: `{weekday}, {Mon D}` — e.g. `Tue, Jul 29` (D-06). */
function formatTooltipDate(date: Date): string {
  return date.toLocaleString("en-US", {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
}

/** The two segment series — colors sourced from PROJECT_PALETTE (NOT the
 *  saturated `--chart-*` tokens — Pitfall 3). ChartStyle scopes
 *  `--color-tasks` / `--color-reviews` from these hexes; `<Bar fill="var(...)">`
 *  resolves to config.<key>.color. */
const chartConfig = {
  tasks: { label: "Tasks", color: PROJECT_PALETTE[5] }, // #5e719c muted blue (D-04 bottom)
  reviews: { label: "Reviews", color: PROJECT_PALETTE[4] }, // #46776f muted teal (D-04 top)
} satisfies ChartConfig;

/**
 * Custom recharts Bar `shape`: draws a 2px muted baseline tick when a day has
 * zero tasks AND zero reviews (D-05), otherwise the default rect. Applied to
 * the tasks (bottom) <Bar> so the stub sits at the x-axis baseline.
 *
 * `minPointSize` is UNRELIABLE in stacked charts (Pitfall 2 — recharts v3 docs
 * explicitly discourage it); the `shape` prop is the robust path. Passed as a
 * function reference (RESEARCH documents this as equivalent to the element
 * form); recharts calls it per-bar with full BarShapeProps geometry.
 */
function ZeroDayStub(props: BarShapeProps): React.ReactElement {
  const { x, y, width, height, payload, fill } = props;
  // background spans the full bar height for the bottom segment of a stack;
  // coalesce nulls (BarRectangleType.x/y are `number | null`) to the bar geom.
  const bg = props.background;
  const bgX = bg?.x ?? x;
  const bgY = bg?.y ?? y;
  const bgWidth = bg?.width ?? width;
  const bgHeight = bg?.height ?? height;

  const row = payload as DayBucket | undefined;
  // Zero day (both segments 0): 2px baseline tick at the x-axis baseline.
  if (row && row.tasks + row.reviews === 0) {
    return (
      <rect
        x={bgX}
        y={bgY + bgHeight - 2}
        width={bgWidth}
        height={2}
        fill="var(--border)"
        rx={1}
      />
    );
  }
  // Non-zero: default rectangle.
  return (
    <rect x={x} y={y} width={width} height={height} fill={fill} rx={2} />
  );
}

export function ActivityChart({
  data,
  window,
}: {
  data: ActivityResponse;
  window: "week" | "month";
}) {
  const dayBuckets = bucketByDay(
    data.tasks,
    data.reviews.prs,
    window,
  );

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Daily activity`}</h2>
      <ChartContainer
        config={chartConfig}
        className="h-48 w-full"
        aria-label={`Daily activity chart, ${dayBuckets.length} days`}
      >
        <BarChart
          data={dayBuckets}
          accessibilityLayer
          barCategoryGap="30%"
          barGap={2}
        >
          <CartesianGrid
            vertical={false}
            stroke="var(--border)"
            strokeDasharray="3 3"
          />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            // Week: interval=0 shows every weekday label. Month: interval=6
            // yields sparse markers at indices 0,7,14,21,28 (~5 labels — D-06
            // forbids crowding; Pitfall 5).
            interval={window === "week" ? 0 : 6}
            tickMargin={8}
            tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
          />
          {/* Y axis hidden — counts are evident from bar height + hover. */}
          <YAxis hide allowDecimals={false} />
          <ChartTooltip
            cursor={{ fill: "var(--accent)", opacity: 0.4 }}
            content={
              <ChartTooltipContent
                // Ignore the short axis tick ("Mon"/"7/2") and show the
                // underlying payload's absolute date as the header (D-06).
                labelFormatter={(_label, payload) => {
                  const row = payload?.[0]?.payload as
                    | DayBucket
                    | undefined;
                  return row ? formatTooltipDate(row.date) : "";
                }}
              />
            }
          />
          {/* Tasks segment — BOTTOM (declared first → stack base, D-04). The
              custom shape draws the zero-day stub (D-05). */}
          <Bar
            stackId="a"
            dataKey="tasks"
            fill="var(--color-tasks)"
            radius={4}
            shape={ZeroDayStub}
          />
          {/* Reviews segment — TOP (stacked on tasks via stackId="a", D-04). */}
          <Bar
            stackId="a"
            dataKey="reviews"
            fill="var(--color-reviews)"
            radius={4}
          />
        </BarChart>
      </ChartContainer>
    </section>
  );
}

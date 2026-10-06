import type { ActivityTask, ReviewDoneSummary } from "@/api/types";

export interface DayBucket {
  /** Calendar day normalized to local midnight — used for the hover tooltip. */
  date: Date;
  /** XAxis tick: Week = "Mon", Month = "M/D" (sparse via `interval`). */
  label: string;
  /** Count of tasks completed that day. */
  tasks: number;
  /** Count of reviews completed that day (0 under HARD_DEGRADE). */
  reviews: number;
}

/**
 * LOCAL calendar-day key. The user thinks in their timezone; a task done at
 * 23:00 local on Jul 29 must land on Jul 29, not Jul 30 (Pitfall 4 — never use
 * the UTC date getters). Using the local getters keeps bucketing in the host
 * timezone.
 */
function dayKey(d: Date): string {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

/**
 * Group the API's already-filtered completion records by local calendar day.
 * Its rolling 168/720-hour window can span more than 7/30 calendar dates,
 * especially at DST. Keep the usual empty slots, but extend them to cover
 * every returned completion; the browser clock must not filter the payload
 * again (including when a cached response survives midnight).
 * Ordered oldest→newest, with null-safe arrays for older servers.
 */
export function bucketByDay(
  tasks: ActivityTask[] | null | undefined,
  prs: ReviewDoneSummary[] | null | undefined,
  window: "week" | "month",
  now: number = Date.now(),
): DayBucket[] {
  const n = window === "week" ? 7 : 30;
  const today = new Date(now);
  const todayMidnight = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate(),
  );

  const firstDay = new Date(todayMidnight);
  firstDay.setDate(firstDay.getDate() - (n - 1));
  const lastDay = new Date(todayMidnight);
  const taskDates = (tasks ?? []).map((t) => new Date(t.doneAt));
  const reviewDates = (prs ?? []).map((r) => new Date(r.completedAt));
  for (const date of [...taskDates, ...reviewDates]) {
    if (Number.isNaN(date.getTime())) continue;
    const midnight = new Date(date.getFullYear(), date.getMonth(), date.getDate());
    if (midnight < firstDay) firstDay.setTime(midnight.getTime());
    if (midnight > lastDay) lastDay.setTime(midnight.getTime());
  }

  const buckets: DayBucket[] = [];
  const byKey = new Map<string, DayBucket>();
  // Calendar increments preserve local midnights across 23/25-hour DST days.
  for (const day = new Date(firstDay); day <= lastDay; day.setDate(day.getDate() + 1)) {
    const b: DayBucket = {
      date: new Date(day),
      label:
        window === "week"
          ? day.toLocaleString("en-US", { weekday: "short" }) // "Mon"
          : `${day.getMonth() + 1}/${day.getDate()}`, // "7/2"
      tasks: 0,
      reviews: 0,
    };
    buckets.push(b);
    byKey.set(dayKey(day), b);
  }

  // Bucket tasks (doneAt is ms-ISO). new Date() handles both precisions.
  for (const date of taskDates) {
    const b = byKey.get(dayKey(date));
    if (b) b.tasks += 1;
  }
  // Bucket reviews (completedAt is second-ISO gh closedAt). D-07: empty under
  // HARD_DEGRADE → the loop body never runs → 0 per bucket.
  for (const date of reviewDates) {
    const b = byKey.get(dayKey(date));
    if (b) b.reviews += 1;
  }
  return buckets;
}


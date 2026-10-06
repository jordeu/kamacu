import { afterEach, describe, expect, it, vi } from "vitest";
import type { ActivityTask, ReviewDoneSummary } from "@/api/types";
import { bucketByDay } from "./bucketByDay";

afterEach(() => vi.unstubAllEnvs());

function task(doneAt: string, id = 1): ActivityTask {
  return { id, title: "Done", doneAt, projectId: 1, projectName: "Project" };
}
function review(completedAt: string, number = 1): ReviewDoneSummary {
  return {
    number, title: "Closed", completedAt, url: "https://example.com/pr",
    projectId: 1, projectName: "Project", repo: "owner/repo",
  };
}

function expectCounts(
  buckets: ReturnType<typeof bucketByDay>,
  tasks: number,
  reviews: number,
) {
  expect(buckets.reduce((sum, b) => sum + b.tasks, 0)).toBe(tasks);
  expect(buckets.reduce((sum, b) => sum + b.reviews, 0)).toBe(reviews);
}

describe("bucketByDay agrees with API completion totals", () => {
  it.each(["week", "month"] as const)(
    "%s retains the oldest partial day of the API's rolling window",
    (window) => {
      vi.stubEnv("TZ", "UTC");
      const now = Date.parse("2026-10-06T12:00:00Z");
      const n = window === "week" ? 7 : 30;
      const cutoff = now - n * 24 * 60 * 60 * 1000;
      const oldest = new Date(cutoff).toISOString();
      const tasks = [task(oldest), task(new Date(now).toISOString(), 2)];
      const prs = [review(oldest.replace(".000Z", "Z")), review(new Date(now).toISOString(), 2)];
      const buckets = bucketByDay(tasks, prs, window, now);
      expectCounts(buckets, tasks.length, prs.length);
      expect(buckets[0].date.getTime()).toBe(cutoff - 12 * 60 * 60 * 1000);
      expect(buckets[0]).toMatchObject({ tasks: 1, reviews: 1 });
      expect(buckets).toHaveLength(n + 1);
    },
  );

  it("retains a review when there are no tasks on the oldest day", () => {
    vi.stubEnv("TZ", "UTC");
    const buckets = bucketByDay([], [review("2026-09-29T12:00:00Z")], "week", Date.parse("2026-10-06T12:00:00Z"));
    expectCounts(buckets, 0, 1);
    expect(buckets[0].date.getDate()).toBe(29);
  });

  it("covers the extra calendar day spanned by 168 hours at spring DST", () => {
    vi.stubEnv("TZ", "Europe/Madrid");
    // April 1 00:30 CEST minus 168 hours = March 24 23:30 CET.
    const buckets = bucketByDay(
      [task("2026-03-24T22:30:00.000Z")],
      [review("2026-03-24T22:30:00Z")],
      "week", Date.parse("2026-03-31T22:30:00Z"),
    );
    expectCounts(buckets, 1, 1);
    expect(buckets).toHaveLength(9);
    expect(buckets[0].date.getDate()).toBe(24);
    expect(buckets.at(-1)?.date.getDate()).toBe(1);
  });

  it("keeps cached completions when the browser crosses midnight", () => {
    vi.stubEnv("TZ", "UTC");
    const buckets = bucketByDay(
      [task("2026-09-29T23:45:00.000Z")], [review("2026-09-29T23:45:00Z")],
      "week", Date.parse("2026-10-07T00:01:00Z"),
    );
    expectCounts(buckets, 1, 1);
  });

  it("groups mixed precision timestamps by local completion date", () => {
    vi.stubEnv("TZ", "America/Los_Angeles");
    const buckets = bucketByDay(
      [task("2026-10-06T01:00:00.123Z")], [review("2026-10-06T01:00:00Z")],
      "week", Date.parse("2026-10-06T18:00:00Z"),
    );
    expectCounts(buckets, 1, 1);
    expect(buckets.find((b) => b.date.getDate() === 5)).toMatchObject({ tasks: 1, reviews: 1 });
  });

  it.each(["week", "month"] as const)("%s keeps zero-day slots and tolerates absent arrays", (window) => {
    vi.stubEnv("TZ", "UTC");
    const buckets = bucketByDay(null, undefined, window, Date.parse("2026-10-06T12:00:00Z"));
    expect(buckets).toHaveLength(window === "week" ? 7 : 30);
    expectCounts(buckets, 0, 0);
  });
});

import { afterEach, expect, it, vi } from "vitest";
import { fetchWorkspaceReviews } from "./workspaceOverview";
import type { Project } from "./types";

const project = (id: number, repo = `org/repo-${id}`) =>
  ({ id, github_repo: repo }) as Project;
const queue = {
  state: "ok",
  prs: [],
  reviewed: [],
  fetchedAt: "2026-11-19T08:00:00Z",
  stale: false,
} as const;
afterEach(() => vi.unstubAllGlobals());

it("bounds repository requests to four and preserves every successful result", async () => {
  let active = 0;
  let peak = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      active++;
      peak = Math.max(peak, active);
      await new Promise((resolve) => setTimeout(resolve, 2));
      active--;
      return Response.json(queue);
    }),
  );
  const result = await fetchWorkspaceReviews(
    Array.from({ length: 13 }, (_, i) => project(i + 1)),
    new AbortController().signal,
  );
  expect(peak).toBe(4);
  expect(result.map((entry) => entry.projectId)).toEqual(
    Array.from({ length: 13 }, (_, i) => i + 1),
  );
  expect(
    result.every((entry) => !entry.failed && entry.data?.state === "ok"),
  ).toBe(true);
});

it("isolates failures, retains only matching cached repository results, and marks them stale", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) => {
      if (path.includes("/1/") || path.includes("/2/"))
        throw new Error("offline");
      return Response.json(queue);
    }),
  );
  const result = await fetchWorkspaceReviews(
    [project(1), project(2, "org/new-repo"), project(3)],
    new AbortController().signal,
    [
      {
        projectId: 1,
        repository: "org/repo-1",
        failed: false,
        data: { ...queue, prs: [], reviewed: [] },
      },
      {
        projectId: 2,
        repository: "org/old-repo",
        failed: false,
        data: { ...queue, prs: [], reviewed: [] },
      },
    ],
  );
  expect(result[0]).toMatchObject({
    failed: true,
    data: { stale: true, fetchedAt: queue.fetchedAt },
  });
  expect(result[1]).toMatchObject({ failed: true, data: null });
  expect(result[2]).toMatchObject({ failed: false, data: { state: "ok" } });
});

it("cancels outstanding requests instead of reporting cancellation as empty review queues", async () => {
  const controller = new AbortController();
  const fetch = vi.fn(async () => {
    controller.abort();
    throw new Error("aborted");
  });
  vi.stubGlobal("fetch", fetch);
  await expect(
    fetchWorkspaceReviews([project(1), project(2)], controller.signal),
  ).rejects.toThrow();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("keeps project-specific hidden preferences separate for a shared repository", async () => {
  const fetch = vi.fn(async (path: string) => Response.json({
    ...queue,
    prs: path.includes("/1/") ? [] : [{ number: 7 }],
  }));
  vi.stubGlobal("fetch", fetch);
  const result = await fetchWorkspaceReviews(
    [project(1, "org/shared"), project(2, "ORG/shared")],
    new AbortController().signal,
  );
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(result.map((entry) => entry.projectId)).toEqual([1, 2]);
  expect(result[0].data?.prs).toEqual([]);
  expect(result[1].data?.prs).toEqual([{ number: 7 }]);
});

it("bypasses the repository cache only for a manual refresh", async () => {
  const fetch = vi.fn<(path: string) => Promise<Response>>(async () =>
    Response.json(queue),
  );
  vi.stubGlobal("fetch", fetch);
  const signal = new AbortController().signal;
  await fetchWorkspaceReviews([project(1)], signal, [], true);
  expect(fetch.mock.calls[0][0]).toBe(
    "/api/projects/1/pull-requests?refresh=1",
  );
  await fetchWorkspaceReviews([project(1)], signal);
  expect(fetch.mock.calls[1][0]).toBe("/api/projects/1/pull-requests");
});

import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import {
  applyRefreshedPRStatus,
  applyRefreshedPRStatuses,
  refreshActiveProjectPRStatuses,
  markPRStatusRefreshFailed,
  type PRStatusSnapshot,
} from "./prStatus";

const snapshot = (number: number, head: string): PRStatusSnapshot => ({
  repository: "acme/app",
  number,
  head,
  lifecycle: "open",
  humans: { state: "approved", reviewers: [] },
  bots: { state: "none", reviewers: [] },
  ci: { state: "pass", jobs: [] },
  fetched_at: "2026-11-16T10:00:00Z",
  stale: false,
});

it.each([false, true])(
  "shows a forced result and resumes an in-flight board batch (previous data: %s)",
  async (previous) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const key = ["task-pr-statuses", 1];
    if (previous)
      client.setQueryData(key, {
        statuses: [snapshot(12, "old"), snapshot(13, "other-old")],
      });
    let release!: (value: { statuses: PRStatusSnapshot[] }) => void;
    let calls = 0;
    const observer = new QueryObserver(client, {
      queryKey: key,
      queryFn: () => {
        calls++;
        return calls === 1
          ? new Promise<{ statuses: PRStatusSnapshot[] }>((resolve) => {
              release = resolve;
            })
          : Promise.resolve({
              statuses: [snapshot(12, "fresh"), snapshot(13, "other-fresh")],
            });
      },
    });
    const unsubscribe = observer.subscribe(() => {});
    try {
      expect(calls).toBe(1);
      await applyRefreshedPRStatus(
        client,
        1,
        "ACME/APP",
        12,
        snapshot(12, "fresh"),
      );
      expect(
        client.getQueryData<PRStatusSnapshot>(["pr-status", 1, "acme/app", 12])
          ?.head,
      ).toBe("fresh");
      await vi.waitFor(() =>
        expect(
          client
            .getQueryData<{ statuses: PRStatusSnapshot[] }>(key)
            ?.statuses.map((s) => s.head),
        ).toEqual(["fresh", "other-fresh"]),
      );
      expect(calls).toBe(2);
      release({
        statuses: [snapshot(12, "obsolete"), snapshot(13, "other-old")],
      });
      await Promise.resolve();
      expect(
        client
          .getQueryData<{ statuses: PRStatusSnapshot[] }>(key)
          ?.statuses.map((s) => s.head),
      ).toEqual(["fresh", "other-fresh"]);
    } finally {
      unsubscribe();
      client.clear();
    }
  },
);

it("marks only the failed PR stale, preserves its data, and recovers after success", async () => {
  const client = new QueryClient();
  const key = ["task-pr-statuses", 1];
  client.setQueryData(key, {
    statuses: [snapshot(12, "old"), snapshot(13, "unaffected")],
  });
  client.setQueryData(["pr-status", 1, "acme/app", 12], snapshot(12, "old"));
  markPRStatusRefreshFailed(client, 1, "ACME/APP", 12);
  const data = client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)!;
  expect(data.statuses[0]).toMatchObject({
    head: "old",
    stale: true,
    error: "Could not refresh PR status",
  });
  expect(data.statuses[1]).toEqual(snapshot(13, "unaffected"));
  expect(
    client.getQueryData<PRStatusSnapshot>(["pr-status", 1, "acme/app", 12])
      ?.stale,
  ).toBe(true);
  await applyRefreshedPRStatus(
    client,
    1,
    "acme/app",
    12,
    snapshot(12, "fresh"),
  );
  expect(
    client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)?.statuses[0],
  ).toEqual(snapshot(12, "fresh"));
  client.clear();
});

it("refreshes and marks failures consistently in project and active overview caches", async () => {
  const client = new QueryClient();
  const keys = [
    ["task-pr-statuses", 1],
    ["task-pr-statuses", 1, "active"],
  ];
  for (const key of keys)
    client.setQueryData(key, {
      statuses: [snapshot(12, "old"), snapshot(13, "other")],
    });
  client.setQueryData(["task-pr-statuses", 2, "active"], {
    statuses: [snapshot(12, "unrelated")],
  });
  markPRStatusRefreshFailed(client, 1, "ACME/APP", 12);
  for (const key of keys)
    expect(
      client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)?.statuses[0]
        .stale,
    ).toBe(true);
  await applyRefreshedPRStatus(
    client,
    1,
    "acme/app",
    12,
    snapshot(12, "fresh"),
  );
  for (const key of keys)
    expect(
      client
        .getQueryData<{ statuses: PRStatusSnapshot[] }>(key)
        ?.statuses.map((status) => status.head),
    ).toEqual(["fresh", "other"]);
  expect(
    client.getQueryData<{ statuses: PRStatusSnapshot[] }>([
      "task-pr-statuses",
      2,
      "active",
    ])?.statuses[0].head,
  ).toBe("unrelated");
  client.clear();
});

it("refreshes mounted overview statuses without refetching other workspaces or inactive projects", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const observers: QueryObserver<{ statuses: PRStatusSnapshot[] }>[] = [];
  const unsubscribers: (() => void)[] = [];
  const fresh = snapshot(12, "fresh");
  const fetch = vi.fn<typeof globalThis.fetch>(async () =>
    Response.json({ statuses: [fresh] }),
  );
  vi.stubGlobal("fetch", fetch);
  for (const id of [1, 2, 99]) {
    client.setQueryData(["task-pr-statuses", id, "active"], {
      statuses: [snapshot(12, `old-${id}`)],
    });
    if (id === 2) continue;
    const observer = new QueryObserver<{ statuses: PRStatusSnapshot[] }>(
      client,
      {
        queryKey: ["task-pr-statuses", id, "active"],
        staleTime: Infinity,
        queryFn: async () =>
          (
            await globalThis.fetch(
              `/api/projects/${id}/task-pr-statuses?active=1`,
            )
          ).json(),
      },
    );
    observers.push(observer);
    unsubscribers.push(observer.subscribe(() => {}));
  }
  client.setQueryData(["task-pr-statuses", 1], {
    statuses: [snapshot(12, "old-board"), snapshot(13, "other")],
  });
  try {
    await refreshActiveProjectPRStatuses(client, [1, 2]);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe(
      "/api/projects/1/task-pr-statuses?active=1&refresh=1",
    );
    expect(
      client
        .getQueryData<{ statuses: PRStatusSnapshot[] }>(["task-pr-statuses", 1])
        ?.statuses.map((s) => s.head),
    ).toEqual(["fresh", "other"]);
    for (const id of [2, 99])
      expect(
        client.getQueryData<{ statuses: PRStatusSnapshot[] }>([
          "task-pr-statuses",
          id,
          "active",
        ])?.statuses[0].head,
      ).toBe(`old-${id}`);
    await observers[0].refetch();
    expect(fetch.mock.calls[1]?.[0]).toBe(
      "/api/projects/1/task-pr-statuses?active=1",
    );
  } finally {
    unsubscribers.forEach((unsubscribe) => unsubscribe());
    client.clear();
    vi.unstubAllGlobals();
  }
});

it("keeps an incomplete inactive board invalidated after a multi-PR refresh", async () => {
  const client = new QueryClient();
  const key = ["task-pr-statuses", 1];
  const pending = client
    .fetchQuery({ queryKey: key, queryFn: () => new Promise<never>(() => {}) })
    .catch(() => {});
  await applyRefreshedPRStatuses(client, 1, [
    snapshot(12, "fresh-12"),
    snapshot(13, "fresh-13"),
  ]);
  expect(client.getQueryState(key)?.isInvalidated).toBe(true);
  expect(
    client
      .getQueryData<{ statuses: PRStatusSnapshot[] }>(key)
      ?.statuses.map((s) => s.head),
  ).toEqual(["fresh-12", "fresh-13"]);
  await pending;
  client.clear();
});

it("bounds project refreshes and preserves successful updates when another project fails", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const unsubscribers: (() => void)[] = [];
  let active = 0,
    peak = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) => {
      active++;
      peak = Math.max(peak, active);
      await new Promise((resolve) => setTimeout(resolve, 2));
      active--;
      if (path.includes("/3/")) throw new Error("offline");
      return Response.json({ statuses: [snapshot(12, "fresh")] });
    }),
  );
  for (let id = 1; id <= 7; id++) {
    const key = ["task-pr-statuses", id, "active"];
    client.setQueryData(key, { statuses: [snapshot(12, "old")] });
    const observer = new QueryObserver(client, {
      queryKey: key,
      staleTime: Infinity,
      queryFn: async () => ({ statuses: [] }),
    });
    unsubscribers.push(observer.subscribe(() => {}));
  }
  try {
    await expect(
      refreshActiveProjectPRStatuses(client, [1, 2, 3, 4, 5, 6, 7]),
    ).rejects.toThrow("Couldn't refresh some linked PR statuses");
    expect(peak).toBe(4);
    expect(
      client.getQueryData<{ statuses: PRStatusSnapshot[] }>([
        "task-pr-statuses",
        3,
        "active",
      ])?.statuses[0],
    ).toMatchObject({ head: "old", stale: true });
    expect(
      client.getQueryData<{ statuses: PRStatusSnapshot[] }>([
        "task-pr-statuses",
        7,
        "active",
      ])?.statuses[0].head,
    ).toBe("fresh");
  } finally {
    unsubscribers.forEach((unsubscribe) => unsubscribe());
    client.clear();
    vi.unstubAllGlobals();
  }
});

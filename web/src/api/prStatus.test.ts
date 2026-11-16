import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { applyRefreshedPRStatus, markPRStatusRefreshFailed, type PRStatusSnapshot } from "./prStatus";

const snapshot = (number: number, head: string): PRStatusSnapshot => ({
  repository: "acme/app", number, head, lifecycle: "open", humans: { state: "approved", reviewers: [] },
  bots: { state: "none", reviewers: [] }, ci: { state: "pass", jobs: [] }, fetched_at: "2026-11-16T10:00:00Z", stale: false,
});

it.each([false, true])("shows a forced result and resumes an in-flight board batch (previous data: %s)", async (previous) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const key = ["task-pr-statuses", 1];
  if (previous) client.setQueryData(key, { statuses: [snapshot(12, "old"), snapshot(13, "other-old")] });
  let release!: (value: { statuses: PRStatusSnapshot[] }) => void;
  let calls = 0;
  const observer = new QueryObserver(client, { queryKey: key, queryFn: () => {
    calls++;
    return calls === 1 ? new Promise<{ statuses: PRStatusSnapshot[] }>((resolve) => { release = resolve; })
      : Promise.resolve({ statuses: [snapshot(12, "fresh"), snapshot(13, "other-fresh")] });
  } });
  const unsubscribe = observer.subscribe(() => {});
  try {
    expect(calls).toBe(1);
    await applyRefreshedPRStatus(client, 1, "ACME/APP", 12, snapshot(12, "fresh"));
    expect(client.getQueryData<PRStatusSnapshot>(["pr-status", 1, "acme/app", 12])?.head).toBe("fresh");
    await vi.waitFor(() => expect(client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)?.statuses.map((s) => s.head)).toEqual(["fresh", "other-fresh"]));
    expect(calls).toBe(2);
    release({ statuses: [snapshot(12, "obsolete"), snapshot(13, "other-old")] });
    await Promise.resolve();
    expect(client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)?.statuses.map((s) => s.head)).toEqual(["fresh", "other-fresh"]);
  } finally {
    unsubscribe();
    client.clear();
  }
});

it("marks only the failed PR stale, preserves its data, and recovers after success", async () => {
  const client = new QueryClient();
  const key = ["task-pr-statuses", 1];
  client.setQueryData(key, { statuses: [snapshot(12, "old"), snapshot(13, "unaffected")] });
  client.setQueryData(["pr-status", 1, "acme/app", 12], snapshot(12, "old"));
  markPRStatusRefreshFailed(client, 1, "ACME/APP", 12);
  const data = client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)!;
  expect(data.statuses[0]).toMatchObject({ head: "old", stale: true, error: "Could not refresh PR status" });
  expect(data.statuses[1]).toEqual(snapshot(13, "unaffected"));
  expect(client.getQueryData<PRStatusSnapshot>(["pr-status", 1, "acme/app", 12])?.stale).toBe(true);
  await applyRefreshedPRStatus(client, 1, "acme/app", 12, snapshot(12, "fresh"));
  expect(client.getQueryData<{ statuses: PRStatusSnapshot[] }>(key)?.statuses[0]).toEqual(snapshot(12, "fresh"));
  client.clear();
});

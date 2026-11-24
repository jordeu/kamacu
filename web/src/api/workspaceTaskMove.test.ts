import { QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "./types";
import { moveWorkspaceTask } from "./workspaceTaskMove";

afterEach(() => vi.unstubAllGlobals());
const task = {
  id: 12,
  project_id: 4,
  title: "Fix switching",
  status: "in_progress",
  source: "manual",
  position: 1,
} as Task;
const workspaceKey = ["tasks", "workspace", 1];
const projectKey = ["tasks", 4];
const detailKey = ["task", 12];
const read = (client: QueryClient, key: (string | number)[]) =>
  client.getQueryData<Task[]>(key);

it("moves optimistically, reconciles server fields, and updates only the relevant caches", async () => {
  const client = new QueryClient();
  for (const key of [workspaceKey, projectKey])
    client.setQueryData(key, [task]);
  client.setQueryData(detailKey, task);
  client.setQueryData(["tasks", "workspace", 2], [{ ...task, id: 90 }]);
  client.setQueryData(["task-pr-statuses", 4, "active"], { statuses: [] });
  const result = {
    ...task,
    status: "in_review",
    position: -1,
    updated_at: "2026-11-24T10:00:00Z",
  } as Task;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init: RequestInit) => {
      expect(path).toBe("/api/tasks/12/move");
      expect(JSON.parse(init.body as string)).toEqual({
        status: "in_review",
        after_id: null,
        expected_status: "in_progress",
      });
      for (const key of [workspaceKey, projectKey])
        expect(read(client, key)?.[0].status).toBe("in_review");
      expect(client.getQueryData<Task>(detailKey)?.status).toBe("in_review");
      return Response.json(result);
    }),
  );
  await expect(
    moveWorkspaceTask(client, 1, task, "in_review"),
  ).resolves.toEqual(result);
  for (const key of [workspaceKey, projectKey])
    expect(read(client, key)).toEqual([result]);
  expect(read(client, ["tasks", "workspace", 2])?.[0].id).toBe(90);
  expect(
    client.getQueryState(["task-pr-statuses", 4, "active"])?.isInvalidated,
  ).toBe(true);
  client.clear();
});

it.each(["todo", "done"] as const)(
  "removes %s from the overview and can restore it with an undo move",
  async (status) => {
    const client = new QueryClient();
    for (const key of [workspaceKey, projectKey])
      client.setQueryData(key, [task]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path: string, init: RequestInit) =>
        Response.json({
          ...task,
          status: JSON.parse(init.body as string).status,
        }),
      ),
    );
    const moved = await moveWorkspaceTask(client, 1, task, status);
    expect(read(client, workspaceKey)).toEqual([]);
    expect(read(client, projectKey)?.[0].status).toBe(status);
    await moveWorkspaceTask(client, 1, moved, "in_progress");
    expect(read(client, workspaceKey)?.[0].status).toBe("in_progress");
    client.clear();
  },
);

it("rolls back a failed move without losing unrelated updates during the request", async () => {
  const client = new QueryClient();
  for (const key of [workspaceKey, projectKey])
    client.setQueryData(key, [task]);
  client.setQueryData(detailKey, task);
  const other = { ...task, id: 19, title: "Concurrent edit" };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      expect(read(client, workspaceKey)).toEqual([]);
      client.setQueryData<Task[]>(workspaceKey, [other]);
      return Response.json({ error: "Move rejected" }, { status: 409 });
    }),
  );
  await expect(moveWorkspaceTask(client, 1, task, "done")).rejects.toThrow(
    "Move rejected",
  );
  expect(read(client, workspaceKey)).toEqual([other, task]);
  expect(read(client, projectKey)).toEqual([task]);
  expect(client.getQueryData(detailKey)).toEqual(task);
  client.clear();
});

it("does not create partial project or detail caches when moving from the overview", async () => {
  const client = new QueryClient();
  client.setQueryData(workspaceKey, [task]);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ ...task, status: "in_review" })),
  );
  await moveWorkspaceTask(client, 1, task, "in_review");
  expect(client.getQueryData(projectKey)).toBeUndefined();
  expect(client.getQueryData(detailKey)).toBeUndefined();
  client.clear();
});

it("preserves related PR links when the move response omits list metadata", async () => {
  const client = new QueryClient();
  const linked = {
    ...task,
    pr_links: [
      {
        task_id: 12,
        repository: "org/app",
        number: 17,
        role: "implementation",
        url: "https://github.com/org/app/pull/17",
      },
    ],
  } as Task;
  for (const key of [workspaceKey, projectKey])
    client.setQueryData(key, [linked]);
  client.setQueryData(detailKey, linked);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ ...task, status: "in_review" })),
  );
  const result = await moveWorkspaceTask(client, 1, linked, "in_review");
  expect(result.pr_links).toEqual(linked.pr_links);
  for (const key of [workspaceKey, projectKey])
    expect(read(client, key)?.[0].pr_links).toEqual(linked.pr_links);
  expect(client.getQueryData<Task>(detailKey)?.pr_links).toEqual(
    linked.pr_links,
  );
  client.clear();
});

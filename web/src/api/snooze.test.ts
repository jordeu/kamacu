import { beforeEach, expect, it, vi } from "vitest";
import { QueryClient, useMutation } from "@tanstack/react-query";
import type { Task } from "./types";
import { useTaskSnooze } from "./snooze";

const { getClient } = vi.hoisted(() => ({ getClient: vi.fn() }));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...await importOriginal<typeof import("@tanstack/react-query")>(),
  useQueryClient: getClient,
  useMutation: vi.fn(),
}));

let client: QueryClient;
beforeEach(() => {
  client = new QueryClient();
  getClient.mockReturnValue(client);
  vi.mocked(useMutation).mockClear();
});

it("preserves enriched task details when snooze and reactivate return only row fields", () => {
  const task = { id: 1, project_id: 2, status: "in_progress", title: "Review" } as Task;
  const details = { ...task, pr_links: [{ repo: "owner/repo", number: 3 }], observed_work: [{ summary: "Reviewed" }] };
  client.setQueryData(["task", 1], details);
  client.setQueryData(["tasks", 2], [details]);
  useTaskSnooze(task);
  const options = vi.mocked(useMutation).mock.calls[0][0];
  const onSuccess = options.onSuccess as (task: Task) => void;
  const snoozed = { ...task, status: "todo", snoozed_until: "2027-01-01T09:00:00Z", snoozed_from: "in_progress" } as Task;
  onSuccess(snoozed);
  expect(client.getQueryData(["task", 1])).toEqual({ ...details, ...snoozed });
  expect(client.getQueryData(["tasks", 2])).toEqual([{ ...details, ...snoozed }]);
  onSuccess({ ...task, snoozed_until: null, snoozed_from: null });
  expect(client.getQueryData(["task", 1])).toEqual({ ...details, snoozed_until: null, snoozed_from: null });
});

import {
  QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { post } from "./client";
import type { Status, Task } from "./types";

const visible = (task: Task) =>
  task.status === "in_progress" || task.status === "in_review";

/** Update existing list caches only; a partial project board must not be created. */
export async function moveWorkspaceTask(
  client: QueryClient,
  workspaceId: number,
  task: Task,
  status: Status,
) {
  const workspaceKey = ["tasks", "workspace", workspaceId];
  const projectKey = ["tasks", task.project_id];
  const detailKey = ["task", task.id];
  const keys = [workspaceKey, projectKey];
  await Promise.all(
    [...keys, detailKey].map((queryKey) =>
      client.cancelQueries({ queryKey, exact: true }),
    ),
  );
  const before = keys.map((key) => ({
    key,
    task: client.getQueryData<Task[]>(key)?.find((item) => item.id === task.id),
  }));
  const detail = client.getQueryData<Task>(detailKey);
  const optimistic: Task = {
    ...task,
    status,
    updated_at: new Date().toISOString(),
    ...(status !== "todo" ? { snoozed_until: null, snoozed_from: null } : {}),
  };
  function apply(value: Task) {
    for (const key of keys)
      client.setQueryData<Task[]>(key, (items) => {
        if (!items) return items;
        const remaining = items.filter((item) => item.id !== task.id);
        return key === workspaceKey && !visible(value)
          ? remaining
          : [...remaining, value];
      });
    client.setQueryData<Task>(detailKey, (previous) => previous && value);
  }
  apply(optimistic);
  try {
    const response = await post<Task>(`/api/tasks/${task.id}/move`, {
      status,
      after_id: null,
      expected_status: task.status,
    });
    // Move responses contain persisted task fields, but omit enriched PR links.
    // Keep those links until the next list read supplies the authoritative set.
    const result = {
      ...response,
      pr_links: response.pr_links ?? task.pr_links,
    };
    apply(result);
    return result;
  } catch (error) {
    // Restore this task only, preserving any other task updates during the request.
    for (const previous of before)
      client.setQueryData<Task[]>(previous.key, (items) => {
        if (!items) return items;
        const remaining = items.filter((item) => item.id !== task.id);
        return previous.task ? [...remaining, previous.task] : remaining;
      });
    client.setQueryData(detailKey, detail);
    throw error;
  } finally {
    // Reconcile ordering, recurrence, task details, and linked-PR scope on both views.
    for (const queryKey of [
      ...keys,
      detailKey,
      ["task-pr-statuses", task.project_id],
    ])
      void client.invalidateQueries({ queryKey });
  }
}

export function useMoveWorkspaceTask(workspaceId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ task, status }: { task: Task; status: Status }) =>
      moveWorkspaceTask(client, workspaceId, task, status),
  });
}

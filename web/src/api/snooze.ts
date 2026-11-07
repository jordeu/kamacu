import { useMutation, useQueryClient } from "@tanstack/react-query";
import { post } from "./client";
import type { Task } from "./types";

export function useTaskSnooze(task: Task) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (until: Date | null) => until
      ? post<Task>(`/api/tasks/${task.id}/snooze`, { until: until.toISOString() })
      : post<Task>(`/api/tasks/${task.id}/reactivate`),
    onSuccess: (updated) => {
      // Mutation responses contain row fields; GET also includes PR links and
      // observed work. Preserve those until the enriched query is refreshed.
      client.setQueryData<Task>(["task", updated.id], (existing) => ({
        ...existing,
        ...updated,
      }));
      client.setQueryData<Task[]>(["tasks", task.project_id], (tasks) =>
        tasks?.map((existing) => existing.id === updated.id ? { ...existing, ...updated } : existing),
      );
    },
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["tasks", task.project_id] });
      void client.invalidateQueries({ queryKey: ["task", task.id] });
    },
  });
}

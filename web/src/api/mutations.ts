import { useMutation, useQueryClient } from "@tanstack/react-query";
import { del, patch, post } from "./client";
import type { Project, Status, Task } from "./types";

export type MoveArgs = { id: number; status: Status; afterId: number | null };

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { name?: string; repo_path: string }) =>
      post<Project>("/api/projects", body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useRenameProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      patch<Project>(`/api/projects/${id}`, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useUpdateProjectSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      description,
      github_repo,
    }: {
      id: number;
      description: string;
      github_repo: string;
    }) => patch<Project>(`/api/projects/${id}`, { description, github_repo }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useDeleteProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => del(`/api/projects/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useCreateTask(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { title: string; description?: string }) =>
      post<Task>(`/api/projects/${projectId}/tasks`, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });
}

export function useUpdateTask(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...body
    }: {
      id: number;
      title?: string;
      description?: string;
    }) => patch<Task>(`/api/tasks/${id}`, body),
    onSuccess: (_task, { id }) => {
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
      queryClient.invalidateQueries({ queryKey: ["task", id] });
    },
  });
}

export function useDeleteTask(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => del(`/api/tasks/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });
}

/**
 * Applies a move locally to the cached task list: sets the moved task's
 * status and resequences positions in the target column so the moved task
 * sits directly after `afterId` (or at the top when `afterId` is null).
 * Server positions become truth again on settle (invalidate).
 */
function applyMove(
  tasks: Task[] | undefined,
  { id, status, afterId }: MoveArgs,
): Task[] | undefined {
  if (!tasks) return tasks;
  const moved = tasks.find((t) => t.id === id);
  if (!moved) return tasks;

  const column = tasks
    .filter((t) => t.status === status && t.id !== id)
    .sort((a, b) => a.position - b.position);

  const afterIndex =
    afterId == null ? -1 : column.findIndex((t) => t.id === afterId);
  column.splice(afterIndex + 1, 0, { ...moved, status });

  const newPositions = new Map(column.map((t, i) => [t.id, i + 1]));

  return tasks.map((t) => {
    if (t.id === id) return { ...t, status, position: newPositions.get(id)! };
    const position = newPositions.get(t.id);
    return position !== undefined ? { ...t, position } : t;
  });
}

export function useMoveTask(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status, afterId }: MoveArgs) =>
      post<Task>(`/api/tasks/${id}/move`, { status, after_id: afterId }),
    onMutate: async (args) => {
      await queryClient.cancelQueries({ queryKey: ["tasks", projectId] });
      const prev = queryClient.getQueryData<Task[]>(["tasks", projectId]);
      queryClient.setQueryData<Task[]>(["tasks", projectId], (old) =>
        applyMove(old, args),
      );
      return { prev };
    },
    onError: (_error, _args, context) => {
      queryClient.setQueryData(["tasks", projectId], context?.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });
}

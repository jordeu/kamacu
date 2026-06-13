import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type { Project, Task } from "./types";

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => get<Project[]>("/api/projects"),
  });
}

export function useTasks(projectId: number) {
  return useQuery({
    queryKey: ["tasks", projectId],
    queryFn: () => get<Task[]>(`/api/projects/${projectId}/tasks`),
    enabled: !isNaN(projectId),
  });
}

export function useTask(taskId: number) {
  return useQuery({
    queryKey: ["task", taskId],
    queryFn: () => get<Task>(`/api/tasks/${taskId}`),
    enabled: !isNaN(taskId),
  });
}

/**
 * On-open origin prefill (D-08): suggests an `owner/name` derived from the
 * project's git `origin` remote. `enabled` is driven by the dialog-open state
 * so git is shelled only when the dialog opens — never on the project list.
 */
export function useProjectGithubOrigin(projectId: number, enabled: boolean) {
  return useQuery({
    queryKey: ["github-origin", projectId],
    queryFn: () =>
      get<{ suggestion: string }>(`/api/projects/${projectId}/github-origin`),
    enabled: enabled && !isNaN(projectId),
    staleTime: Infinity, // origin rarely changes; fetch once per dialog open via `enabled`
  });
}

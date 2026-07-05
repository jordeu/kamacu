import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type { Project, Task, Workspace } from "./types";

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => get<Project[]>("/api/projects"),
  });
}

export function useWorkspaces() {
  return useQuery({
    queryKey: ["workspaces"],
    queryFn: () => get<Workspace[]>("/api/workspaces"),
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

/**
 * Whether the host has the `gh` CLI installed (GET /api/github/status, always
 * 200). Gates the GitHub integration toggle's default/enable behavior so the
 * integration is never shown on — and cannot be enabled — when gh is missing
 * (GHSET-01/GHSET-03). Call-time on the server, so install/uninstall is picked
 * up on the next fetch with no restart.
 */
export function useGithubStatus() {
  return useQuery({
    queryKey: ["github-status"],
    queryFn: () => get<{ gh_available: boolean }>("/api/github/status"),
  });
}

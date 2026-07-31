import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type {
  ActivityResponse,
  Agent,
  Project,
  Task,
  Workspace,
} from "./types";

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

// M001: the configurable agents feed (default-first, then name-sorted -- the
// backend orders it). Consumed by the Settings Agents section and the Project
// Settings agent selector.
export function useAgents() {
  return useQuery({
    queryKey: ["agents"],
    queryFn: () => get<Agent[]>("/api/agents"),
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

/**
 * Phase 11 — the Activity page's combined data fetch (Phase 10 D-01: one
 * endpoint → one query, one loading state). Switching scope/window just
 * changes the queryKey.
 *
 * The scope/window types are inlined to match the exact parseScope/
 * parseWindow grammar (activity_helpers.go:51-80) without forward-referencing
 * useActivityView.ts (structurally compatible — TypeScript structural typing).
 * The authoritative type aliases live in useActivityView.ts.
 *
 * `enabled` lets the page defer the fetch until the persisted scope resolves
 * (Phase 11 RESEARCH Pitfall 6 — don't fire with a null-derived scope). No
 * `refetchInterval` — Activity is an occasional view, not a live dashboard
 * (contrast pullRequests.ts which polls every 60s).
 */
export function useActivity(
  scope: "global" | `workspace:${number}` | `project:${number}`,
  window: "week" | "month",
  enabled: boolean = true,
) {
  return useQuery({
    queryKey: ["activity", scope, window],
    queryFn: () =>
      get<ActivityResponse>(
        `/api/activity?scope=${scope}&window=${window}`,
      ),
    enabled,
  });
}

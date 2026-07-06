import { useMutation, useQueryClient } from "@tanstack/react-query";
import { del, patch, post } from "./client";
import type { Agent, Project, Status, Task, Workspace } from "./types";

export type MoveArgs = { id: number; status: Status; afterId: number | null };

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      name?: string;
      repo_path?: string;
      repo?: string;
      // v1.9: create into the active workspace (WSPROJ-02). Omitted/undefined
      // falls back to the Personal default server-side, so passing undefined is
      // always safe.
      workspace_id?: number;
    }) => post<Project>("/api/projects", body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

export function useCreateWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name }: { name: string }) =>
      post<Workspace>("/api/workspaces", { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
    },
  });
}

export function useRenameWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      patch<Workspace>(`/api/workspaces/${id}`, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
    },
  });
}

export function useDeleteWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => del(`/api/workspaces/${id}`),
    onSuccess: () => {
      // Invalidate both: deleting a workspace changes the workspace list AND
      // (defensively) what the sidebar project filter should show.
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

/**
 * Transfer a project to another workspace (WSPROJ-01, D-17). This is a PATCH to
 * the *project* endpoint (an optional `workspace_id` on the partial-PATCH — no
 * dedicated transfer route), so it invalidates ["projects"] (the sidebar list),
 * not ["workspaces"].
 */
export function useMoveProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      workspace_id,
    }: {
      id: number;
      workspace_id: number;
    }) => patch<Project>(`/api/projects/${id}`, { workspace_id }),
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
      icon_letters,
      icon_color,
    }: {
      id: number;
      description: string;
      github_repo?: string;
      icon_letters?: string;
      icon_color?: string;
    }) => {
      // Build the PATCH body conditionally so an `undefined` field is never
      // sent (D-09: omitted key = untouched, matching the backend
      // partial-PATCH contract).
      const body: {
        description: string;
        github_repo?: string;
        icon_letters?: string;
        icon_color?: string;
      } = { description };
      if (github_repo !== undefined) body.github_repo = github_repo;
      if (icon_letters !== undefined) body.icon_letters = icon_letters;
      if (icon_color !== undefined) body.icon_color = icon_color;
      return patch<Project>(`/api/projects/${id}`, body);
    },
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


// ── M001: configurable agents CRUD ──────────────────────────────────────────
// Mirrors the workspaces mutation shapes (useCreateWorkspace / useRenameWorkspace
// / useDeleteWorkspace). Each invalidates ["agents"]; setDefault also invalidates
// ["projects"] because the default affects new-project agent assignment.

export function useCreateAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, command }: { name: string; command: string }) =>
      post<Agent>("/api/agents", { name, command }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useUpdateAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      name,
      command,
      engine,
    }: {
      id: number;
      name?: string;
      command?: string;
      engine?: "claude" | "custom";
    }) => patch<Agent>(`/api/agents/${id}`, { name, command, engine }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useDeleteAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => del(`/api/agents/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

// Set an agent as the global default (POST /api/agents/{id}/default). The
// server preserves the exactly-one invariant transactionally.
export function useSetDefaultAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => post<Agent>(`/api/agents/${id}/default`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
}

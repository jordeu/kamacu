import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, post } from "./client";
import type { ApiError } from "./client";
import type { Task } from "./types";

/** Fresh worktree state for the cleanup dialog (GET on dialog open). */
export interface WorktreeState {
  branch: string;
  path: string;
  dirty_files: number;
  running_sessions: number;
}

/**
 * Worktree state fetch — Pitfall 8 guard: gcTime 0 + staleTime 0 + caller
 * enables only while the dialog is open, so the dialog always sees reality
 * (dirty/session counts at open time), never cache.
 */
export function useWorktreeState(taskId: number, enabled: boolean) {
  return useQuery<WorktreeState, ApiError>({
    queryKey: ["worktree", taskId],
    queryFn: () => get<WorktreeState>(`/api/tasks/${taskId}/worktree`),
    enabled,
    gcTime: 0,
    staleTime: 0,
  });
}

/**
 * Create or Retry worktree creation (D-25/D-26). The response IS truth —
 * a 200 may carry a fresh worktree_error (failure-with-201 semantics).
 */
export function useCreateWorktree(taskId: number, projectId: number) {
  const queryClient = useQueryClient();
  return useMutation<Task, ApiError, void>({
    mutationFn: () => post<Task>(`/api/tasks/${taskId}/worktree`),
    onSuccess: (task) => {
      queryClient.setQueryData(["task", taskId], task);
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
    },
  });
}

/**
 * Cleanup (DELETE) with the server-enforced D-32/D-33 gates: 409 when
 * sessions are running and !stop_sessions, 409 when dirty and !force.
 */
export function useCleanupWorktree(taskId: number, projectId: number) {
  const queryClient = useQueryClient();
  return useMutation<
    void,
    ApiError,
    { stop_sessions: boolean; force: boolean }
  >({
    mutationFn: (body) => del(`/api/tasks/${taskId}/worktree`, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["task", taskId] });
      queryClient.invalidateQueries({ queryKey: ["tasks", projectId] });
      queryClient.invalidateQueries({ queryKey: ["sessions", taskId] });
    },
  });
}

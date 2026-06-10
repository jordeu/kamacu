import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, post } from "./client";
import type { ApiError } from "./client";

export interface TermSession {
  id: string; // opaque uuid — NEVER parse
  label: string; // "bash #3" (dev) or "Bash 3" (task-scoped) — server-assigned
  status: "running" | "exited";
  exitCode?: number;
  createdAt: string; // RFC3339
  taskId?: number; // omitted for unscoped dev sessions
}

export function useSessions(taskId?: number) {
  return useQuery({
    queryKey: taskId !== undefined ? ["sessions", taskId] : ["sessions"],
    queryFn: () =>
      get<TermSession[]>(
        taskId !== undefined
          ? `/api/sessions?task_id=${taskId}`
          : "/api/sessions",
      ),
    // Keeps non-attached rows' status honest (research Pattern 4)
    refetchInterval: 5000,
  });
}

export function useSpawnSession(taskId?: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      taskId !== undefined
        ? post<TermSession>("/api/sessions", { task_id: taskId })
        : post<TermSession>("/api/sessions"),
    onSuccess: (session) => {
      // Spawn-select race fix (Phase 2): write the fresh session into the
      // scoped cache BEFORE invalidating so the new tab renders immediately.
      if (taskId !== undefined) {
        queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
          old ? [session, ...old] : [session],
        );
      }
      // Prefix-matches the scoped ["sessions", taskId] keys too.
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}

export function useStopSession() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: (id: string) => post<void>(`/api/sessions/${id}/stop`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}

export function useDeleteSession() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: (id: string) => del(`/api/sessions/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}

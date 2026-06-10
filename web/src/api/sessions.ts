import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, post } from "./client";
import type { ApiError } from "./client";

export interface TermSession {
  id: string; // opaque uuid — NEVER parse
  label: string; // "bash #3" — server-assigned
  status: "running" | "exited";
  exitCode?: number;
  createdAt: string; // RFC3339
}

export function useSessions() {
  return useQuery({
    queryKey: ["sessions"],
    queryFn: () => get<TermSession[]>("/api/sessions"),
    // Keeps non-attached rows' status honest (research Pattern 4)
    refetchInterval: 5000,
  });
}

export function useSpawnSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => post<TermSession>("/api/sessions"),
    onSuccess: () => {
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

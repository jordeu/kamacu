import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, post } from "./client";
import type { ApiError } from "./client";

export interface TermSession {
  id: string; // opaque uuid — NEVER parse ("" on a restored orphaned ghost)
  label: string; // "bash #3" (dev), "Bash 3" (task-scoped), "Agent" — server-assigned
  status: "running" | "exited";
  exitCode?: number;
  createdAt: string; // RFC3339
  taskId?: number; // omitted for unscoped dev sessions
  kind?: "bash" | "agent"; // session discriminator (04-02 server Info JSON)
  agentStatus?: "working" | "idle" | "waiting" | "exited"; // agent sessions only
  stopRequested?: boolean; // Kamacu-initiated stop (gray-dot discriminator)
  // Restored tmux survivor (TMUX-05, D-88): a DB-derived ghost (id "") the
  // server emits for a tmux session that outlived a Kamacu restart. orphaned
  // is the "needs a one-shot reattach spawn" signal; tmuxName carries the name
  // to reattach against. Both absent on every real, live session — tmux stays
  // invisible (D-77).
  orphaned?: boolean;
  tmuxName?: string;
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

// useReattachTmux fires a one-shot reattach spawn for a restored (orphaned)
// tmux survivor (TMUX-05, D-88). The mutate arg is the persisted tmux name; the
// server runs new-session -A (attach-or-create) and returns a REAL in-memory
// session (orphaned:false, real id) that replaces the ghost. No Resume button,
// no banner — the reattach is automatic and invisible (D-77 divergence from the
// agent Resume flow).
export function useReattachTmux(taskId: number) {
  const queryClient = useQueryClient();
  return useMutation<TermSession, ApiError, string>({
    mutationFn: (name: string) =>
      post<TermSession>("/api/sessions", {
        task_id: taskId,
        reattach_tmux_name: name,
      }),
    onSuccess: (session) => {
      // Same spawn-select race fix as useSpawnSession: write the real session
      // into the scoped cache so the tab attaches immediately; the next poll
      // drops the orphaned ghost (its row no longer surfaces a survivor entry
      // once the live session covers the name).
      queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
        old ? [session, ...old] : [session],
      );
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}

export function useSpawnAgent(taskId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      post<TermSession>("/api/sessions", { task_id: taskId, kind: "agent" }),
    onSuccess: (session) => {
      // Same spawn-select race fix as useSpawnSession: write the fresh
      // session into the scoped cache so the pane attaches immediately.
      queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
        old ? [session, ...old] : [session],
      );
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
      // Board/tab dots appear without waiting a poll period.
      queryClient.invalidateQueries({ queryKey: ["agent-statuses"] });
    },
  });
}

export function useResumeAgent(taskId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      post<TermSession>("/api/sessions", {
        task_id: taskId,
        kind: "agent",
        resume: true,
      }),
    onSuccess: (session) => {
      // Same spawn-select race fix as useSpawnAgent: write the fresh session
      // into the scoped cache so the pane attaches from the mutation result.
      queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
        old ? [session, ...old] : [session],
      );
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
      queryClient.invalidateQueries({ queryKey: ["agent-statuses"] });
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

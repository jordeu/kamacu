import { useQuery } from "@tanstack/react-query";
import { get } from "./client";

export interface AgentStatusEntry {
  taskId: number;
  projectId: number;
  sessionId: string;
  status: "working" | "idle" | "waiting" | "exited";
  exitCode: number | null;
  stopRequested: boolean;
  resumable: boolean;
}

/** Single source of truth for card dots, the Agent tab dot, and sidebar
 *  chips (research Pattern 4) — one query, no per-project fan-out. */
export function useAgentStatuses() {
  return useQuery({
    queryKey: ["agent-statuses"],
    queryFn: () => get<AgentStatusEntry[]>("/api/agents/status"),
    refetchInterval: 5000, // UI-SPEC: waiting surfaces within 5s
  });
}

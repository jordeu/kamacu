import { useQuery } from "@tanstack/react-query";
import { get } from "./client";

export interface AgentStatusEntry {
  taskId: number;
  projectId: number;
  sessionId: string;
  status: "working" | "idle" | "waiting" | "exited" | "running"; // M001: custom engines report running/exited only
  exitCode: number | null;
  stopRequested: boolean;
  resumable: boolean;
  prNumber: number | null; // null for source='manual' (D-15)
  // "global" = the Scratchpad agent entry the server synthesizes since
  // Phase 15 (agents.go — taskId 0, "Scratchpad"/"Global" labels). The only
  // runtime branch on this field is the PR badge (=== "github_pr"), which a
  // global value flows through harmlessly (Pitfall 7 audit).
  source: "manual" | "github_pr" | "global";
  taskTitle: string; // tasks.title — bar row label (SBAR-10)
  projectName: string; // projects.name — bar row label (SBAR-10)
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

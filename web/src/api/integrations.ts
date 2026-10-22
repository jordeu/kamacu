import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";

export type IntegrationEngine = "claude" | "codex" | "opencode";
export interface IntegrationStatus {
  engine: IntegrationEngine;
  available: boolean;
  enabled: boolean;
  managed: boolean;
  skill: string;
  skill_path: string;
  mcp: string;
  message?: string;
}
export function useIntegrations() {
  return useQuery({ queryKey: ["integrations"], queryFn: () => get<IntegrationStatus[]>("/api/integrations"), staleTime: 30_000 });
}
export function useUpdateIntegration() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ engine, action }: { engine: IntegrationEngine; action: "enable" | "repair" | "remove" }) =>
      post<IntegrationStatus>(`/api/integrations/${engine}/${action}`),
    onSettled: () => client.invalidateQueries({ queryKey: ["integrations"] }),
  });
}

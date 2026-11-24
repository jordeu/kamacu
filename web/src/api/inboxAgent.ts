import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post, put } from "./client";

export interface InboxRule { id: string; text: string; enabled: boolean }
export interface InboxAgentConfig {
  profile_id: number | null;
  revision: number;
  context_bytes: number;
  output_tokens: number;
  timeout_seconds: number;
  rules: InboxRule[];
}
export function useInboxAgentConfig() {
  return useQuery({ queryKey: ["inbox-agent-config"], queryFn: () => get<InboxAgentConfig>("/api/inbox-agent/config") });
}
export function useSaveInboxAgentConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (config: InboxAgentConfig) => put<InboxAgentConfig>("/api/inbox-agent/config", config),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["inbox-agent-config"] }),
  });
}
export function useTestInboxAgent() {
  return useMutation({ mutationFn: () => post<{ ok: boolean }>("/api/inbox-agent/config/test") });
}

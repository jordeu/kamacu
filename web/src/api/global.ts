import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put } from "./client";

// GlobalConfig mirrors the Go globalConfig wire (global.go) exactly — it is
// BOTH the GET /api/global shape and the PUT response shape (D-23: the PUT
// re-reads through the GET load path, so one interface serves both).
//
// github_repo is a nullable pointer by design (the 00017 marker contract):
// null ⇒ folder root or unconfigured, non-null ⇒ managed clone. There is
// deliberately no managed boolean — the pointer IS the marker.
//
// agent.engine is deliberately a plain string: the Go wire emits "claude" |
// "custom" | "opencode" (the OpenCode system seed). Do NOT reuse the stale
// Agent.engine union in types.ts, which lacks "opencode" (16-RESEARCH
// Pitfall 2).
export interface GlobalConfig {
  root_path: string;
  github_repo: string | null;
  agent_id: number;
  updated_at: string;
  root_exists: boolean;
  live: { agent: number; bash: number; tmux: number };
  agent: { id: number; name: string; engine: string };
}

/**
 * The ONE global-config query (D-17 one round-trip): shared by the /global
 * view (16-02) and the Settings Scratchpad section (16-03). The 5s refetch
 * keeps root_exists and the live counts honest — both are server-derived per
 * request, never mirrored client-side.
 */
export function useGlobal() {
  return useQuery({
    queryKey: ["global"],
    queryFn: () => get<GlobalConfig>("/api/global"),
    refetchInterval: 5000,
  });
}

/**
 * Partial config save (PUT /api/global, D-20 pointer semantics): omit
 * untouched keys; root_path "" is THE clear; a non-empty repo sets the
 * managed clone; agent_id sets the default agent. On success the PUT
 * response — which IS the GET shape (D-23) — is written straight into the
 * ["global"] cache; no extra refetch. On failure the cache is untouched, so
 * controlled inputs snap back to server truth.
 */
export function useSaveGlobal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      root_path?: string;
      repo?: string;
      agent_id?: number;
    }) => put<GlobalConfig>("/api/global", body),
    onSuccess: (g) => qc.setQueryData(["global"], g),
  });
}

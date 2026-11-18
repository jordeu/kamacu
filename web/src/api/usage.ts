import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get } from "./client";

export interface UsageWindow {
  key: string;
  label: string;
  utilization: number;
  resetsAt: string | null;
}

export interface ExtraUsage {
  enabled: boolean;
  usedCredits: number | null;
  monthlyLimit: number | null;
  utilization: number | null;
  currency: string;
  historyAvailable: boolean;
  daily: { date: string; spent: number; partial: boolean }[] | null;
}

export interface UsageResponse {
  extraUsage: ExtraUsage | null;
  state: "ok" | "no_credentials" | "auth_expired" | "error";
  stale: boolean;
  fetchedAt: string | null;
  windows: UsageWindow[] | null;
}

/** Quota poll — TanStack dedupes the ["usage"] key across mounts. */
export function useQuota() {
  return useQuery({
    queryKey: ["usage"],
    queryFn: () => get<UsageResponse>("/api/usage"),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false, // QUOTA-04: visible-only (explicit, though false is the default)
  });
}

export function useRefreshQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => get<UsageResponse>("/api/usage?refresh=1"),
    onSuccess: (data) => qc.setQueryData(["usage"], data), // write-then-done, useSpawnSession precedent
  });
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put } from "./client";
import type { ApiError } from "./client";

export interface SettingEntry {
  value: string;
  default: string;
  options?: string[];
}

export type Settings = Record<string, SettingEntry>;

/** All four settings in one fetch (GET /api/settings). */
export function useSettings() {
  return useQuery({
    queryKey: ["settings"],
    queryFn: () => get<Settings>("/api/settings"),
  });
}

/**
 * Per-key save (PUT /api/settings/{key}) — no optimistic update: the
 * per-field commit model holds the draft locally; a 400 leaves the cache
 * untouched (SET-04 isolation). On success the response entry replaces the
 * cached one and becomes the new revert target.
 */
export function useSaveSetting(key: string) {
  const qc = useQueryClient();
  return useMutation<SettingEntry, ApiError, string>({
    mutationFn: (value) => put<SettingEntry>(`/api/settings/${key}`, { value }),
    onSuccess: (entry) =>
      qc.setQueryData<Settings>(
        ["settings"],
        (old) => old && { ...old, [key]: entry },
      ),
  });
}

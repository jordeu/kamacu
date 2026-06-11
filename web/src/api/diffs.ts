import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type { ApiError } from "./client";

/**
 * Structured diff JSON from GET /api/tasks/{id}/diff (plan 05-02 Pattern 4
 * contract). The server pre-structures everything into per-file hunks — the
 * frontend is a dumb map, no client-side diff parsing.
 */
export interface DiffResponse {
  base: string;
  totals: { files: number; additions: number; deletions: number };
  files: DiffFile[];
}

export interface DiffFile {
  path: string;
  oldPath: string | null;
  status: "modified" | "new" | "deleted" | "renamed";
  binary: boolean;
  additions: number | null; // null for binary
  deletions: number | null;
  hunks: DiffHunk[];
}

export interface DiffHunk {
  header: string;
  lines: DiffLine[];
}

export interface DiffLine {
  kind: "context" | "add" | "del";
  text: string;
}

/**
 * The Diff tab's data source (D-61). The DiffTab mounts only while the tab is
 * active, so the default staleTime 0 refetches on every mount — that IS the
 * fetch-on-activation behavior. The manual refresh button calls refetch().
 * Never polls: no refetchInterval. While refetching with data present,
 * TanStack keeps the previous data rendered (UI-SPEC: no blanking), so no
 * placeholderData is needed. On error, callers read `error.message` for the
 * muted git-stderr line (client.ts surfaces the server {error} string there).
 */
export function useTaskDiff(taskId: number) {
  return useQuery<DiffResponse, ApiError>({
    queryKey: ["diff", taskId],
    queryFn: () => get<DiffResponse>(`/api/tasks/${taskId}/diff`),
  });
}

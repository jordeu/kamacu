import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put } from "./client";
import type { ApiError } from "./client";

/**
 * Structured diff JSON from GET /api/tasks/{id}/diff (plan 05-02 Pattern 4
 * contract). The server pre-structures everything into per-file hunks — the
 * frontend is a dumb map, no client-side diff parsing.
 */
export interface DiffResponse {
  base: string;
  totals: {
    files: number;
    additions: number;
    deletions: number;
    uncommitted: number;
    unpushed: number;
  };
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
  // Content hash of the file's current diff (plan 22-02 backend contract). The
  // Viewed toggle is keyed by task + path + hash; a new hash means the file
  // changed since it was last viewed (DIFF-04 auto-reset).
  hash: string;
  // Server-persisted per-file review state (DIFF-03). Reflected on every open,
  // including after a server restart.
  viewed: boolean;
  // Commit-state markers: `uncommitted` = content not fully captured in
  // commits (staged/unstaged/untracked — the `git status` signal);
  // `unpushed` = committed content missing from the remote. Both can be true
  // at once; neither affects the Viewed hash.
  uncommitted: boolean;
  unpushed: boolean;
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

/**
 * Toggle a file's "Viewed" state (DIFF-03) against the plan 22-02 endpoint
 * PUT /api/tasks/{id}/diff/viewed (body { path, hash, viewed } → 204). Mirrors
 * the useMoveTask optimistic pattern: onMutate cancels + snapshots + flips
 * `viewed` on the matching path in the ["diff", taskId] cache; onError rolls
 * back; onSettled invalidates so the authoritative server state reconciles.
 */
export function useToggleViewed(taskId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      path,
      hash,
      viewed,
    }: {
      path: string;
      hash: string;
      viewed: boolean;
    }) =>
      put<void>(`/api/tasks/${taskId}/diff/viewed`, { path, hash, viewed }),
    onMutate: async ({ path, viewed }) => {
      await queryClient.cancelQueries({ queryKey: ["diff", taskId] });
      const prev = queryClient.getQueryData<DiffResponse>(["diff", taskId]);
      queryClient.setQueryData<DiffResponse>(["diff", taskId], (old) =>
        old
          ? {
              ...old,
              files: old.files.map((f) =>
                f.path === path ? { ...f, viewed } : f,
              ),
            }
          : old,
      );
      return { prev };
    },
    onError: (_error, _args, context) => {
      queryClient.setQueryData(["diff", taskId], context?.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["diff", taskId] });
    },
  });
}

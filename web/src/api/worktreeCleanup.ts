import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";
import type { ApiError } from "./client";

/**
 * The worktree-cleanup panel's data layer (WTREE-01..04). It mirrors the Diff
 * tab's fetch-on-mount + manual-Refresh model (D-10/D-61): useWorktreeList sets
 * no polling interval, so it fetches on mount and only re-fetches when the
 * manual Refresh button calls refetch() — the panel never polls (a dense admin
 * table that reflects a point-in-time snapshot, not a live feed).
 *
 * Every mutation invalidates the single ["worktrees"] query key on settle
 * (Pitfall 4) so the list re-reads reality after any destructive action —
 * remove, bulk clean, or clear-pointer. One key, one invalidation path.
 *
 * These types encode the Wave 2 backend contract (plan 23-02) verbatim; the
 * Wave 3 UI implements against them with no guesswork.
 */

/** git-vs-DB status of a worktree row. */
export type Classification = "referenced" | "orphan" | "stale";

/** PR lifecycle state when the worktree backs a github_pr task. */
export type PRState = "merged" | "closed" | "open";

/**
 * One worktree row (GET /api/worktrees). `task_id` is 0 for an orphan (git-listed
 * with no DB task). `unpushed` is null when the base could not be resolved
 * (render "Unpushed?"). `blocked` marks a D-01 permission-blocked shell that needs
 * manual removal; `blocked_path` names the offending directory.
 */
export interface WorktreeRow {
  repo: string;
  path: string;
  task_id: number;
  classification: Classification;
  association: string | null;
  pr_state: PRState | null;
  branch: string;
  dirty: number;
  unpushed: number | null;
  stash: number;
  blocked: boolean;
  blocked_path: string | null;
  sessions: number;
}

/** A project's worktrees, grouped for the panel's per-project sections. */
export interface ProjectGroup {
  project_id: number;
  project_name: string;
  icon_letters: string;
  icon_color: string;
  worktrees: WorktreeRow[];
}

/** GET /api/worktrees response: grouped rows + headline counts. */
export interface WorktreeListResponse {
  projects: ProjectGroup[];
  counts: { total: number; orphaned: number };
}

/** POST /api/worktrees/remove request body. */
export interface RemoveArgs {
  repo: string;
  path: string;
  task_id: number;
  force: boolean;
  stop_sessions: boolean;
}

/**
 * Remove outcome. A 204 (client.ts → undefined) means the worktree was removed;
 * a 200 { outcome: "blocked", path } is the D-01 blocked case (NOT an error) —
 * the shell is permission-blocked and needs manual `sudo` removal.
 */
export interface RemoveResult {
  outcome: "removed" | "blocked";
  path?: string;
}

/** One entry in the bulk clean-eligible preview / result set. */
export interface EligibleItem {
  repo: string;
  path: string;
  task_id: number;
  project_name: string;
  reason: "orphaned" | "done" | "pr_merged" | "pr_closed";
}

/** POST /api/worktrees/clean-eligible?dry_run=1 → the preview set. */
export interface CleanEligiblePreview {
  items: EligibleItem[];
}

/** POST /api/worktrees/clean-eligible → the applied result. */
export interface CleanEligibleResult {
  removed: number;
  skipped: number;
}

/** POST /api/worktrees/clear-pointer request body (D-07 stale pointer). */
export interface ClearPointerArgs {
  task_id: number;
}

/**
 * The panel's data source. Fetch-on-mount, no polling interval (D-10 no-poll):
 * the default staleTime refetches on mount, and the manual Refresh button calls
 * refetch(). On error, callers read `error.message` for the server {error} line
 * (client.ts surfaces it there).
 */
export function useWorktreeList() {
  return useQuery<WorktreeListResponse, ApiError>({
    queryKey: ["worktrees"],
    queryFn: () => get<WorktreeListResponse>("/api/worktrees"),
  });
}

/**
 * Force-remove a single worktree (D-03). Because a 204 resolves to undefined
 * (client.ts), map an undefined result to { outcome: "removed" }; a 200 body
 * carries { outcome: "blocked", path } for the D-01 permission-blocked case.
 * onSettled invalidates ["worktrees"] so the list re-reads reality.
 */
export function useRemoveWorktree() {
  const queryClient = useQueryClient();
  return useMutation<RemoveResult, ApiError, RemoveArgs>({
    mutationFn: async (body) => {
      const r = await post<RemoveResult | undefined>(
        "/api/worktrees/remove",
        body,
      );
      return r ?? { outcome: "removed" };
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["worktrees"] });
    },
  });
}

export interface CleanupOptions {
  include_unpushed: boolean;
  include_dirty: boolean;
}

export interface CleanupProgress extends CleanEligibleResult {
  total: number;
  completed: number;
  done: boolean;
}

export function useCleanEligiblePreview(
  options: CleanupOptions,
  enabled = true,
) {
  return useQuery<CleanEligiblePreview, ApiError>({
    queryKey: ["worktree-cleanup-preview", options],
    enabled,
    refetchOnWindowFocus: false,
    queryFn: () =>
      post<CleanEligiblePreview>(
        "/api/worktrees/clean-eligible?dry_run=1",
        options,
      ),
    staleTime: 0,
  });
}

// Read server progress rather than estimating elapsed time. A missing final
// record is an interrupted cleanup, never a successful completion.
export async function cleanEligible(
  options: CleanupOptions,
  targets: EligibleItem[],
  onProgress: (progress: CleanupProgress) => void,
): Promise<CleanupProgress> {
  const response = await fetch("/api/worktrees/clean-eligible?stream=1", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...options, targets }),
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(
      body?.error ??
        "Could not start cleanup. Refresh the preview and try again.",
    );
  }
  if (!response.body)
    throw new Error(
      "Cleanup progress is unavailable. Refresh to check the worktrees.",
    );
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = "";
  let last: CleanupProgress | undefined;
  try {
    while (true) {
      const { value, done } = await reader.read();
      pending += decoder.decode(value, { stream: !done });
      const lines = pending.split("\n");
      pending = lines.pop() ?? "";
      for (const line of lines) {
        if (!line.trim()) continue;
        last = JSON.parse(line) as CleanupProgress;
        onProgress(last);
      }
      if (done) break;
    }
  } finally {
    reader.releaseLock();
  }
  if (!last?.done)
    throw new Error(
      "Cleanup was interrupted. Some worktrees may have been removed. Refresh the preview to check what remains.",
    );
  return last;
}

/**
 * Clear a stale DB pointer (D-07) — nulls the task's worktree link and prunes
 * the stale git registration; deletes no files. onSettled invalidates
 * ["worktrees"] so the cleared row drops out of the list.
 */
export function useClearPointer() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, ClearPointerArgs>({
    mutationFn: (body) => post<void>("/api/worktrees/clear-pointer", body),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["worktrees"] });
    },
  });
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get } from "./client";

/** A single review-requested PR. Fetched rich (the head/base/fork fields feed
 *  Phase 12/13's worktree checkout + diff base), rendered minimal in Phase 11
 *  (D-01/D-02). `checks` is the server-side reduction of `statusCheckRollup`
 *  to one signal — the raw array is never shipped to the browser (D-00d). */
export interface PRSummary {
  number: number;
  title: string;
  author: string; // author.login
  updatedAt: string; // ISO (RFC3339) — relative time + D-14 sort
  url: string;
  checks: "pass" | "fail" | "pending" | "none";
  // fetched-but-unrendered (Phase 12/13): worktree checkout + diff base + fork handling
  headRefName: string;
  headRefOid: string;
  baseRefName: string;
  isCrossRepository: boolean;
}

export interface PullRequestsResponse {
  state: "ok" | "no_gh" | "auth_required" | "disabled" | "error";
  stale: boolean;
  fetchedAt: string | null;
  prs: PRSummary[] | null;
}

/** Per-project PR review poll. refetchIntervalInBackground:false pauses the
 *  poll when the browser tab is hidden (GHCOL-04). Only the visible project's
 *  column mounts this, so the steady-state gh call set is ~1 (D-11). Verbatim
 *  shape of useQuota — the endpoint is always-200, so consumers branch on
 *  state/stale, never HTTP status. */
export function usePullRequests(projectId: number) {
  return useQuery({
    queryKey: ["pull-requests", projectId],
    queryFn: () =>
      get<PullRequestsResponse>(`/api/projects/${projectId}/pull-requests`),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}

export function useRefreshPullRequests(projectId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      get<PullRequestsResponse>(
        `/api/projects/${projectId}/pull-requests?refresh=1`,
      ),
    onSuccess: (data) => qc.setQueryData(["pull-requests", projectId], data),
  });
}

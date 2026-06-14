import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";
import type { ApiError } from "./client";
import type { Task } from "./types";

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

/** The live PR detail the open/reattach endpoint returns alongside the task
 *  (a fresh `gh pr view` — 12-04). The review header/meta/Description read
 *  from THIS, not task.title/description, so they never drift from GitHub
 *  (D-08/D-11, Pitfall 6). */
export interface PRDetailWire {
  number: number;
  title: string;
  body: string;
  author: string;
  url: string;
  baseRefName: string;
  // 12-06 wire fields — feed the GitHub-style merge line
  // "<author> wants to merge <N> commits into <base> from <head>" (12-07).
  headRefName: string;
  commits: number;
}

export interface OpenReviewResponse {
  task: Task;
  pr: PRDetailWire;
}

/** The query key under which TaskPage reads the live PR detail stashed at open
 *  time (see useOpenReview.onSuccess). Keyed by task id so a deep-link/reattach
 *  routing to /tasks/{id} finds it. */
export function prDetailKey(taskId: number) {
  return ["pr-detail", taskId] as const;
}

/** Re-hydrate the live PR detail on a review-view mount (12-07). useOpenReview
 *  seeds prDetailKey at open time, but a hard reload wipes the client cache —
 *  this query re-fetches GET .../pull-requests/{n} under the SAME key so the
 *  header (link/author/from-branch) + the seed's interpolated title come from
 *  live GitHub again. The open-time seed is the initial cache (no flicker on a
 *  normal open); staleTime defers an immediate refetch right after open already
 *  seeded it. Errors (toggle off / unlinked / gh down) leave `data` undefined,
 *  and the header degrades to its task-field fallback — never breaks. */
export function usePullRequestDetail(
  projectId: number,
  prNumber: number | null | undefined,
  taskId: number,
  enabled: boolean,
) {
  return useQuery({
    queryKey: prDetailKey(taskId), // SAME key useOpenReview seeds → shares the open-time cache
    queryFn: () =>
      get<PRDetailWire>(`/api/projects/${projectId}/pull-requests/${prNumber}`),
    enabled: enabled && prNumber != null,
    staleTime: 30_000, // avoid an immediate refetch right after open already seeded it
  });
}

/** Open-or-reattach a PR review (12-04): POST .../pull-requests/{n}/review →
 *  find-or-create the source='github_pr' task, provision the detached PR-head
 *  worktree, and return {task, pr}. Mirrors useCreateWorktree's mutation shape
 *  (worktrees.ts). On success the caller routes to /projects/{id}/tasks/{task.id};
 *  the live pr detail is stashed for TaskPage to render the read-only header. */
export function useOpenReview(projectId: number) {
  const qc = useQueryClient();
  return useMutation<OpenReviewResponse, ApiError, number>({
    mutationFn: (prNumber) =>
      post<OpenReviewResponse>(
        `/api/projects/${projectId}/pull-requests/${prNumber}/review`,
      ),
    onSuccess: (res) => {
      // Stash the live PR detail + the ready task so the review view renders
      // the read-only PR header/body immediately on route, without a refetch.
      qc.setQueryData(prDetailKey(res.task.id), res.pr);
      qc.setQueryData(["task", res.task.id], res.task);
    },
  });
}

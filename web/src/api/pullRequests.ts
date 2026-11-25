import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post, put } from "./client";
import type { ApiError } from "./client";
import type { Task } from "./types";
import type { ProjectReviewQueue } from "./workspaceOverview";

/** A single review-requested PR. Fetched rich (the head/base/fork fields feed
 *  Phase 12/13's worktree checkout + diff base), rendered minimal in Phase 11
 *  (D-01/D-02). `checks` is the server-side reduction of `statusCheckRollup`
 *  to one signal — the raw array is never shipped to the browser (D-00d). */
export interface PRSummary {
  hidden?: boolean;
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
  reviewed: PRSummary[] | null; // reviewed-by:@me, deduped against prs server-side (D-13/D-14)
}

/** Per-project PR review poll. refetchIntervalInBackground:false pauses the
 *  poll when the browser tab is hidden (GHCOL-04). Only the visible project's
 *  column mounts this, so the steady-state gh call set is ~1 (D-11). Verbatim
 *  shape of useQuota — the endpoint is always-200, so consumers branch on
 *  state/stale, never HTTP status. */
export function usePullRequests(projectId: number) {
  return useQuery({
    queryKey: ["pull-requests", projectId],
    queryFn: ({ signal }) =>
      get<PullRequestsResponse>(`/api/projects/${projectId}/pull-requests?include_hidden=1`, { signal }),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}

export function useRefreshPullRequests(projectId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      await qc.cancelQueries({ queryKey: ["pull-requests", projectId] });
      // Use the same cancellable query as polling. A hide/restore must be able
      // to cancel this response before it overwrites the saved visibility.
      return qc.fetchQuery({
        queryKey: ["pull-requests", projectId],
        queryFn: ({ signal }) => get<PullRequestsResponse>(
          `/api/projects/${projectId}/pull-requests?refresh=1&include_hidden=1`, { signal },
        ),
        staleTime: 0,
      });
    },
  });
}

export function useSetPRHidden(projectId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ number, hidden }: { number: number; hidden: boolean }) =>
      put<{ hidden: boolean }>(`/api/projects/${projectId}/pull-requests/${number}/visibility`, { hidden }),
    onSuccess: async ({ hidden }, { number }) => {
      await Promise.all([
        qc.cancelQueries({ queryKey: ["pull-requests", projectId] }),
        qc.cancelQueries({ queryKey: ["workspace-reviews"] }),
      ]);
      // The preference is already saved. Apply it even if the subsequent list
      // fetch fails, and keep an older refresh from bringing the card back.
      qc.setQueryData<PullRequestsResponse>(["pull-requests", projectId], (data) => data && ({
        ...data,
        prs: data.prs?.map((pr) => pr.number === number ? { ...pr, hidden } : pr) ?? null,
        reviewed: data.reviewed?.map((pr) => pr.number === number ? { ...pr, hidden } : pr) ?? null,
      }));
      if (hidden) {
        qc.setQueriesData<ProjectReviewQueue[]>({ queryKey: ["workspace-reviews"] }, (queues) =>
          queues?.map((queue) => queue.projectId !== projectId || !queue.data ? queue : ({
            ...queue,
            data: {
              ...queue.data,
              prs: queue.data.prs?.filter((pr) => pr.number !== number) ?? null,
              reviewed: queue.data.reviewed?.filter((pr) => pr.number !== number) ?? null,
            },
          })),
        );
      }
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["pull-requests", projectId] }),
        qc.invalidateQueries({ queryKey: ["workspace-reviews"] }),
      ]);
    },
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
  // 13-01: PR lifecycle state — drives the merged/closed review banner (D-09).
  state: "OPEN" | "CLOSED" | "MERGED";
}

export interface OpenReviewResponse {
  task: Task;
  pr: PRDetailWire;
}

/** The query key under which TaskPage reads the live PR detail stashed at open
 *  time (see useOpenReview.onSuccess). Include the full source identity so
 *  a response from another project/PR cannot be reused for the same task. */
export function prDetailKey(taskId: number, projectId: number, prNumber: number | null | undefined) {
  return ["pr-detail", taskId, projectId, prNumber] as const;
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
    queryKey: prDetailKey(taskId, projectId, prNumber), // SAME key useOpenReview seeds → shares the open-time cache
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
      qc.setQueryData(prDetailKey(res.task.id, res.task.project_id, res.task.pr_number ?? res.pr.number), res.pr);
      qc.setQueryData(["task", res.task.id], res.task);
    },
  });
}

/** Fetch the full open-PR list only while the picker is visible. */
export function useOpenPullRequests(projectId: number, enabled: boolean) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled) return;
    // Disabling a query does not cancel an in-flight fetch. Dismissal must
    // abort it explicitly so the server can stop the paginated gh process.
    return () => {
      void qc.cancelQueries({ queryKey: ["open-pull-requests", projectId], exact: true });
    };
  }, [enabled, projectId, qc]);

  return useQuery({
    queryKey: ["open-pull-requests", projectId],
    queryFn: ({ signal }) => get<PullRequestsResponse>(`/api/projects/${projectId}/pull-requests?scope=open`, { signal }),
    enabled,
    staleTime: 30_000,
  });
}

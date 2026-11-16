import { QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";

export interface PRStatusReviewer {
  login: string;
  state: string;
  detail?: string;
  findings?: number;
}
export interface PRStatusGroup {
  state: string;
  reviewers: PRStatusReviewer[];
}
export interface PRStatusSnapshot {
  repository: string;
  number: number;
  lifecycle: string;
  head: string;
  author?: string;
  humans: PRStatusGroup;
  bots: PRStatusGroup;
  ci: { state: string; jobs: { name: string; state: string; detail?: string }[] };
  fetched_at: string | null;
  stale: boolean;
  error?: string;
}

export async function applyRefreshedPRStatus(client: QueryClient, projectId: number, repository: string, number: number, snapshot: PRStatusSnapshot) {
  const boardKey = ["task-pr-statuses", projectId];
  const prKey = ["pr-status", projectId, repository.toLowerCase(), number];
  const boardState = client.getQueryState<{ statuses: PRStatusSnapshot[] }>(boardKey);
  await Promise.all([client.cancelQueries({ queryKey: boardKey }), client.cancelQueries({ queryKey: prKey })]);
  if (boardState) {
    client.setQueryData<{ statuses: PRStatusSnapshot[] }>(boardKey, (previous) => {
      const statuses = previous?.statuses ?? [];
      const matches = (status: PRStatusSnapshot) => status.repository.toLowerCase() === repository.toLowerCase() && status.number === number;
      return { statuses: statuses.some(matches) ? statuses.map((status) => matches(status) ? snapshot : status) : [...statuses, snapshot] };
    });
    // Resume a canceled batch so the remaining PRs still receive their results.
    // Inactive, incomplete boards stay invalidated until their next mount.
    if (!boardState.data || boardState.fetchStatus === "fetching") void client.invalidateQueries({ queryKey: boardKey });
  }
  client.setQueryData(prKey, snapshot);
}

export function markPRStatusRefreshFailed(client: QueryClient, projectId: number, repository: string, number: number) {
  const failed = (status: PRStatusSnapshot): PRStatusSnapshot => ({ ...status, stale: true, error: "Could not refresh PR status" });
  client.setQueryData<{ statuses: PRStatusSnapshot[] }>(["task-pr-statuses", projectId], (previous) => previous && ({
    statuses: previous.statuses.map((status) => status.repository.toLowerCase() === repository.toLowerCase() && status.number === number ? failed(status) : status),
  }));
  client.setQueryData<PRStatusSnapshot>(["pr-status", projectId, repository.toLowerCase(), number], (previous) => previous && failed(previous));
}

export function useRefreshPRStatus(projectId: number, repository: string, number: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => post<PRStatusSnapshot>(`/api/projects/${projectId}/task-pr-statuses/refresh`, { repository, number }),
    onSuccess: (snapshot) => applyRefreshedPRStatus(client, projectId, repository, number, snapshot),
    onError: () => markPRStatusRefreshFailed(client, projectId, repository, number),
  });
}

/** All linked cards in a project share one poll, including the task header. */
export function useProjectPRStatuses(projectId: number) {
  return useQuery({
    queryKey: ["task-pr-statuses", projectId],
    // Keep the shared in-flight read across card remounts and navigation.
    queryFn: () => get<{ statuses: PRStatusSnapshot[] }>(
      `/api/projects/${projectId}/task-pr-statuses`,
    ),
    staleTime: 45_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}

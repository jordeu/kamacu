import {
  QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
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
  ci: {
    state: string;
    jobs: { name: string; state: string; detail?: string }[];
  };
  fetched_at: string | null;
  stale: boolean;
  error?: string;
}

/** Apply a refresh to every card scope together, then resume incomplete reads. */
export async function applyRefreshedPRStatuses(
  client: QueryClient,
  projectId: number,
  snapshots: PRStatusSnapshot[],
) {
  if (!snapshots.length) return;
  const boardKey = ["task-pr-statuses", projectId];
  const boards = client
    .getQueryCache()
    .findAll({ queryKey: boardKey })
    .map((query) => ({ key: query.queryKey, state: query.state }));
  const identity = (status: PRStatusSnapshot) =>
    `${status.repository.toLowerCase()}/${status.number}`;
  const fresh = new Map(snapshots.map((status) => [identity(status), status]));
  const prKey = (status: PRStatusSnapshot) => [
    "pr-status",
    projectId,
    status.repository.toLowerCase(),
    status.number,
  ];
  await Promise.all([
    client.cancelQueries({ queryKey: boardKey }),
    ...snapshots.map((status) =>
      client.cancelQueries({ queryKey: prKey(status) }),
    ),
  ]);
  for (const board of boards) {
    client.setQueryData<{ statuses: PRStatusSnapshot[] }>(
      board.key,
      (previous) => {
        const remaining = new Map(fresh);
        const statuses = (previous?.statuses ?? []).map((status) => {
          const key = identity(status);
          remaining.delete(key);
          return fresh.get(key) ?? status;
        });
        return { statuses: [...statuses, ...remaining.values()] };
      },
    );
  }
  for (const status of snapshots) client.setQueryData(prKey(status), status);
  // Invalidate after every snapshot has been applied: a later setQueryData
  // must not accidentally make an incomplete canceled batch look complete.
  for (const board of boards) {
    if (!board.state.data || board.state.fetchStatus === "fetching")
      void client.invalidateQueries({ queryKey: board.key, exact: true });
  }
}

export function applyRefreshedPRStatus(
  client: QueryClient,
  projectId: number,
  repository: string,
  number: number,
  snapshot: PRStatusSnapshot,
) {
  return applyRefreshedPRStatuses(client, projectId, [
    { ...snapshot, repository: repository.toLowerCase(), number },
  ]);
}

export function markPRStatusRefreshFailed(
  client: QueryClient,
  projectId: number,
  repository: string,
  number: number,
) {
  const failed = (status: PRStatusSnapshot): PRStatusSnapshot => ({
    ...status,
    stale: true,
    error: "Could not refresh PR status",
  });
  client.setQueriesData<{ statuses: PRStatusSnapshot[] }>(
    { queryKey: ["task-pr-statuses", projectId] },
    (previous) =>
      previous && {
        statuses: previous.statuses.map((status) =>
          status.repository.toLowerCase() === repository.toLowerCase() &&
          status.number === number
            ? failed(status)
            : status,
        ),
      },
  );
  client.setQueryData<PRStatusSnapshot>(
    ["pr-status", projectId, repository.toLowerCase(), number],
    (previous) => previous && failed(previous),
  );
}

export function useRefreshPRStatus(
  projectId: number,
  repository: string,
  number: number,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      post<PRStatusSnapshot>(
        `/api/projects/${projectId}/task-pr-statuses/refresh`,
        { repository, number },
      ),
    onSuccess: (snapshot) =>
      applyRefreshedPRStatus(client, projectId, repository, number, snapshot),
    onError: () =>
      markPRStatusRefreshFailed(client, projectId, repository, number),
  });
}

/** All linked cards in a project share one poll, including the task header. */
export function useProjectPRStatuses(projectId: number, activeOnly = false) {
  return useQuery({
    queryKey: activeOnly
      ? ["task-pr-statuses", projectId, "active"]
      : ["task-pr-statuses", projectId],
    // Keep the shared in-flight read across card remounts and navigation.
    queryFn: () =>
      get<{ statuses: PRStatusSnapshot[] }>(
        `/api/projects/${projectId}/task-pr-statuses${activeOnly ? "?active=1" : ""}`,
      ),
    staleTime: 45_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}

/** Refresh only mounted overview status queues, with bounded project requests. */
export async function refreshActiveProjectPRStatuses(
  client: QueryClient,
  projectIds: number[],
) {
  const ids = new Set(projectIds);
  const queries = client
    .getQueryCache()
    .findAll({ queryKey: ["task-pr-statuses"], type: "active" })
    .filter(
      (query) =>
        query.queryKey[2] === "active" && ids.has(query.queryKey[1] as number),
    );
  let cursor = 0;
  let failures = 0;
  async function worker() {
    while (cursor < queries.length) {
      const query = queries[cursor++];
      const projectId = query.queryKey[1] as number;
      try {
        await client.cancelQueries({ queryKey: query.queryKey, exact: true });
        const result = await client.fetchQuery({
          queryKey: query.queryKey,
          queryFn: ({ signal }) =>
            get<{ statuses: PRStatusSnapshot[] }>(
              `/api/projects/${projectId}/task-pr-statuses?active=1&refresh=1`,
              {
                signal: AbortSignal.any([signal, AbortSignal.timeout(35_000)]),
              },
            ),
          staleTime: 0,
          retry: false,
        });
        await applyRefreshedPRStatuses(client, projectId, result.statuses);
      } catch {
        failures++;
        const cached = client.getQueryData<{ statuses: PRStatusSnapshot[] }>(
          query.queryKey,
        );
        for (const status of cached?.statuses ?? [])
          markPRStatusRefreshFailed(
            client,
            projectId,
            status.repository,
            status.number,
          );
      }
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(4, queries.length) }, worker),
  );
  if (failures)
    throw new Error(
      "Couldn't refresh some linked PR statuses. Showing last known results.",
    );
}

export function useRefreshActiveProjectPRStatuses() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (projectIds: number[]) =>
      refreshActiveProjectPRStatuses(client, projectIds),
  });
}

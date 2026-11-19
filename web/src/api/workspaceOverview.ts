import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { get } from "./client";
import type { Project, Task } from "./types";
import type { PullRequestsResponse } from "./pullRequests";

export interface ProjectReviewQueue {
  projectId: number;
  repository: string;
  data: PullRequestsResponse | null;
  failed: boolean;
}

/** Bound HTTP fan-out as well as GitHub's existing per-repository cache. A
 * failed repository must not hide successful queues or erase its last result. */
export async function fetchWorkspaceReviews(
  projects: readonly Project[],
  signal: AbortSignal,
  previous: readonly ProjectReviewQueue[] = [],
  force = false,
): Promise<ProjectReviewQueue[]> {
  const results = new Array<ProjectReviewQueue>(projects.length);
  const requests = new Map<string, Promise<PullRequestsResponse>>();
  let cursor = 0;
  async function worker() {
    while (cursor < projects.length) {
      signal.throwIfAborted();
      const index = cursor++;
      const project = projects[index];
      const repository = project.github_repo!;
      try {
        const identity = repository.toLowerCase();
        let request = requests.get(identity);
        if (!request) {
          request = get<PullRequestsResponse>(
            `/api/projects/${project.id}/pull-requests${force ? "?refresh=1" : ""}`,
            { signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]) },
          );
          requests.set(identity, request);
        }
        const data = await request;
        results[index] = {
          projectId: project.id,
          repository,
          data,
          failed: false,
        };
      } catch {
        signal.throwIfAborted();
        const cached = previous.find(
          (entry) =>
            entry.projectId === project.id && entry.repository === repository,
        );
        results[index] = {
          projectId: project.id,
          repository,
          data: cached?.data ? { ...cached.data, stale: true } : null,
          failed: true,
        };
      }
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(4, projects.length) }, worker),
  );
  return results;
}

export function useWorkspaceTasks(workspaceId: number, enabled: boolean) {
  return useQuery({
    queryKey: ["tasks", "workspace", workspaceId],
    queryFn: ({ signal }) =>
      get<Task[]>(
        `/api/tasks?workspace_id=${workspaceId}&status=in_progress&status=in_review`,
        { signal },
      ),
    enabled,
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });
}

export function useWorkspaceReviews(
  workspaceId: number,
  projects: readonly Project[],
  enabled: boolean,
) {
  const client = useQueryClient();
  const forceNextFetch = useRef(false);
  const linked = projects.filter((project) => project.github_repo?.trim());
  const queryKey = [
    "workspace-reviews",
    workspaceId,
    linked.map((project) => [project.id, project.github_repo]),
  ];
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => {
      const force = forceNextFetch.current;
      forceNextFetch.current = false;
      return fetchWorkspaceReviews(
        linked,
        signal,
        client.getQueryData<ProjectReviewQueue[]>(queryKey),
        force,
      );
    },
    enabled,
    // A concurrent board request can return state=ok before any successful
    // snapshot exists. Retry that transient state without claiming zero PRs.
    refetchInterval: (query) =>
      query.state.data?.some(
        (entry) => entry.data?.state === "ok" && !entry.data.fetchedAt,
      )
        ? 5000
        : 60_000,
    refetchIntervalInBackground: false,
  });
  return {
    ...query,
    refresh: () => {
      forceNextFetch.current = true;
      return query.refetch();
    },
  };
}

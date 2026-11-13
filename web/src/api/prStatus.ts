import { useQuery } from "@tanstack/react-query";
import { get } from "./client";

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
  humans: PRStatusGroup;
  bots: PRStatusGroup;
  ci: { state: string; jobs: { name: string; state: string; detail?: string }[] };
  fetched_at: string | null;
  stale: boolean;
  error?: string;
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

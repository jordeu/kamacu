import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api, post } from "./client";
import type { Task } from "./types";

export interface GithubIssue {
  number: number;
  title: string;
  body: string;
  url: string;
  author: string;
  state: string;
  task_id?: number;
}
interface IssuePage {
  issues: GithubIssue[];
  has_more: boolean;
}

export function useGithubIssues(
  projectId: number,
  query: string,
  includeClosed: boolean,
  assignedToMe: boolean,
) {
  return useInfiniteQuery({
    queryKey: ["github-issues", projectId, query, includeClosed, assignedToMe],
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) => {
      const params = new URLSearchParams({
        q: query,
        include_closed: String(includeClosed),
        assigned_to_me: String(assignedToMe),
        page: String(pageParam),
      });
      return api<IssuePage>(`/api/projects/${projectId}/issues?${params}`, {
        signal,
      });
    },
    getNextPageParam: (last, pages) =>
      last.has_more ? pages.length + 1 : undefined,
    retry: false,
  });
}

export function useImportGithubIssue(projectId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (number: number) =>
      post<{ task: Task; already_imported: boolean }>(
        `/api/projects/${projectId}/issues/${number}/import`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      qc.invalidateQueries({ queryKey: ["github-issues", projectId] });
    },
  });
}

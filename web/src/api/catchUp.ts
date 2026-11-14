import { useInfiniteQuery, useQuery, useQueries } from "@tanstack/react-query";
import { useProjects } from "./queries";
import { get } from "./client";
import type { FeedbackQuestion, FeedbackResult } from "./inbox";
import type { PRDetailWire } from "./pullRequests";
import type { PRStatusSnapshot } from "./prStatus";
import type { Project, Task } from "./types";

export function useCatchUpQueue() {
  return useQuery({
    queryKey: ["catch-up-queue"],
    queryFn: ({ signal }) => get<FeedbackQuestion[]>("/api/inbox/catch-up", { signal }),
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
}
export function useMessageHistory(id: string) {
  return useInfiniteQuery({
    queryKey: ["inbox-history", id],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => get<{ items: FeedbackQuestion[]; has_more: boolean }>(
      `/api/inbox/${encodeURIComponent(id)}/history?offset=${pageParam}`, { signal },
    ),
    getNextPageParam: (last, pages) => last.has_more ? pages.length * 50 : undefined,
  });
}
export function useCatchUpPRDetail(projectId: number, pr: CatchUpPR, enabled: boolean) {
  return useQuery({
    queryKey: ["catch-up-pr-detail", projectId, pr.repository.toLowerCase(), pr.number],
    queryFn: async ({ signal }) => {
      const detail = await get<PRDetailWire>(`/api/projects/${projectId}/pull-requests/${pr.number}`, { signal });
      if (detail.url?.toLowerCase() !== pr.url.toLowerCase()) throw new Error("The project's repository changed. Refresh project references before retrying.");
      return detail;
    },
    enabled,
    staleTime: 30_000,
  });
}
export function useCatchUpPRStatus(projectId: number, pr: CatchUpPR, enabled: boolean) {
  return useQuery({
    queryKey: ["pr-status", projectId, pr.repository.toLowerCase(), pr.number],
    queryFn: async ({ signal }) => {
      const status = await get<PRStatusSnapshot>(`/api/projects/${projectId}/pull-requests/${pr.number}/status`, { signal });
      if (status.repository.toLowerCase() !== pr.repository.toLowerCase() || status.number !== pr.number) throw new Error("The project's repository changed. Refresh project references before retrying.");
      return status;
    },
    enabled,
    staleTime: 45_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}
// Never replace a newer polled revision with a delayed mutation response.
export function latestCatchUpMessage(current: FeedbackQuestion | undefined, incoming: FeedbackQuestion) {
  if (current && (current.revision > incoming.revision || (current.revision === incoming.revision && current.updated_at > incoming.updated_at))) return current;
  return incoming;
}
export function catchUpActionSucceeded(result: FeedbackResult, action: "answer" | "dismiss") {
  return action === "dismiss" ? result.outcome === "dismissed" && result.question.status === "dismissed" : result.outcome === "accepted";
}
export interface CatchUpPR { repository: string; number: number; url: string }

// Only explicit PR URLs and saved direct-action identities are references.
// Rejected task associations must not reappear through text matching.
export function catchUpPRs(q: FeedbackQuestion, tasks: Task[], projects: Project[]): CatchUpPR[] {
  const found = new Map<string, CatchUpPR>();
  const linked = new Set(tasks.flatMap(t => (t.pr_links ?? []).filter(l => l.role !== "incidental").map(l => `${l.repository.toLowerCase()}/${l.number}`)));
  const rejected = new Set(tasks.flatMap(t => (t.pr_links ?? []).filter(l => l.role === "incidental").map(l => `${l.repository.toLowerCase()}/${l.number}`)));
  function add(repository: string, number: number) {
    const key = `${repository.toLowerCase()}/${number}`;
    if (number > 0 && Number.isSafeInteger(number) && (!rejected.has(key) || linked.has(key))) found.set(key, { repository, number, url: `https://github.com/${repository}/pull/${number}` });
  }
  for (const task of tasks) for (const link of task.pr_links ?? []) if (link.role !== "incidental") add(link.repository, link.number);
  const visible = [q.source_url, q.question, q.abstract, q.summary, q.context].join("\n").replace(/<!--[\s\S]*?-->/g, "");
  for (const match of visible.matchAll(/https:\/\/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/(\d+)(?=$|[\s/?#)\]}>.,;'"`])/gi)) add(match[1], Number(match[2]));
  for (const option of q.options) for (const action of option.action?.actions ?? []) {
    const repo = projects.find(p => p.id === action.project_id)?.github_repo;
    if (repo) for (const number of action.pr_numbers ?? []) add(repo, number);
  }
  return [...found.values()];
}

export function useCatchUpContext(q: FeedbackQuestion) {
  const projects = useProjects();
  const ids = [...new Set([...(q.task_id > 0 ? [q.task_id] : []), ...(q.linked_tasks ?? []).map(t => t.task_id)])];
  const queries = useQueries({ queries: ids.map(id => ({ queryKey: ["task", id], queryFn: () => get<Task>(`/api/tasks/${id}`), refetchInterval: 5000 })) });
  const tasks = queries.flatMap(query => query.data ? [query.data] : []);
  const prs = catchUpPRs(q, tasks, projects.data ?? []);
  const projectNames = [...new Set([
    ...(q.project_name ? [q.project_name] : []),
    ...[...tasks.map(t => t.project_id), ...(q.linked_tasks ?? []).map(t => t.project_id)].flatMap(id => projects.data?.find(p => p.id === id)?.name ?? []),
    ...prs.flatMap(pr => projects.data?.find(p => p.github_repo?.toLowerCase() === pr.repository.toLowerCase())?.name ?? []),
  ])];
  return { projects, tasks, queries, prs, projectNames };
}

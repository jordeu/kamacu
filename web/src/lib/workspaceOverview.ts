import type { PRSummary } from "@/api/pullRequests";
import type { ProjectReviewQueue } from "@/api/workspaceOverview";
import type { Project, Task } from "@/api/types";

export interface OverviewPR extends PRSummary {
  project: Project;
  stale: boolean;
}

export function isReviewQueueIncomplete(queue: ProjectReviewQueue) {
  return (
    queue.failed ||
    !queue.data?.fetchedAt ||
    queue.data.state !== "ok" ||
    queue.data.stale
  );
}

export function overviewReviews(
  projects: readonly Project[],
  queues: readonly ProjectReviewQueue[],
) {
  const prs = new Map<string, OverviewPR>();
  for (const queue of queues) {
    const project = projects.find(
      (candidate) =>
        candidate.id === queue.projectId &&
        candidate.github_repo === queue.repository,
    );
    if (!project) continue;
    for (const pr of queue.data?.prs ?? []) {
      // Preserve project provenance until after project/search filtering.
      const key = `${project.id}/${queue.repository.toLowerCase()}/${pr.number}`;
      if (!prs.has(key))
        prs.set(key, {
          ...pr,
          project,
          stale: isReviewQueueIncomplete(queue),
        });
    }
  }
  return [...prs.values()];
}

export function filterOverview(
  tasks: readonly Task[],
  prs: readonly OverviewPR[],
  projects: readonly Project[],
  projectId: string,
  search: string,
  sort: string,
) {
  const projectMap = new Map(projects.map((project) => [project.id, project]));
  const query = search.trim().toLocaleLowerCase();
  const matches = (id: number, text: string) =>
    projectMap.has(id) &&
    (projectId === "all" || String(id) === projectId) &&
    (!query ||
      `${projectMap.get(id)!.name} ${text}`
        .toLocaleLowerCase()
        .includes(query));
  const order = (a: string, b: string) =>
    (sort === "old" ? 1 : -1) * (Date.parse(a) - Date.parse(b));
  const manual = tasks.filter(
    (task) =>
      task.source === "manual" &&
      matches(task.project_id, `${task.title} ${task.id} ${(task.pr_links ?? []).filter((link) => link.role !== "incidental").map((link) => `${link.repository} ${link.number}`).join(" ")}`),
  );
  const matchingPRs = new Map<string, OverviewPR>();
  for (const pr of prs) {
    if (!matches(pr.project.id, `${pr.title} ${pr.number} ${pr.author}`))
      continue;
    const key = `${pr.project.github_repo!.toLowerCase()}/${pr.number}`;
    const previous = matchingPRs.get(key);
    if (!previous || (previous.stale && !pr.stale)) matchingPRs.set(key, pr);
  }
  return {
    progress: manual
      .filter((task) => task.status === "in_progress")
      .sort((a, b) => order(a.updated_at, b.updated_at)),
    review: manual
      .filter((task) => task.status === "in_review")
      .sort((a, b) => order(a.updated_at, b.updated_at)),
    prs: [...matchingPRs.values()].sort((a, b) =>
      order(a.updatedAt, b.updatedAt),
    ),
  };
}

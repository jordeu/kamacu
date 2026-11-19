import { expect, it } from "vitest";
import { filterOverview, overviewReviews } from "./workspaceOverview";
import type { Project, Task } from "@/api/types";
import type { ProjectReviewQueue } from "@/api/workspaceOverview";

const projects = [
  { id: 1, name: "sched", github_repo: "org/sched" },
  { id: 2, name: "fusion", github_repo: "org/fusion" },
] as Project[];
const task = (
  id: number,
  project_id: number,
  status: string,
  source = "manual",
) =>
  ({
    id,
    project_id,
    status,
    source,
    title: `Restore task ${id}`,
    updated_at: `2026-11-${id < 10 ? "0" : ""}${id}T08:00:00Z`,
  }) as Task;
const tasks = [
  task(1, 1, "in_progress"),
  task(2, 2, "in_review"),
  task(3, 1, "todo"),
  task(4, 1, "in_review", "github_pr"),
  task(5, 99, "in_progress"),
  task(6, 1, "in_progress"),
];
const queue = (projectId: number, repository: string) =>
  ({
    projectId,
    repository,
    failed: false,
    data: {
      state: "ok",
      stale: false,
      prs: [
        {
          number: 7,
          title: "Queue repair",
          author: "alice",
          updatedAt: "2026-11-10T00:00:00Z",
        },
      ],
      reviewed: [{ number: 8 }],
    },
  }) as ProjectReviewQueue;

it("does not mix board tasks with review workspaces, other statuses or other workspaces", () => {
  const result = filterOverview(tasks, [], projects, "all", "", "recent");
  expect(result.progress.map((entry) => entry.id)).toEqual([6, 1]);
  expect(result.review.map((entry) => entry.id)).toEqual([2]);
  expect(
    filterOverview(tasks, [], projects, "all", "", "old").progress.map(
      (entry) => entry.id,
    ),
  ).toEqual([1, 6]);
  expect(filterOverview(tasks, [], projects, "1", "", "recent").review).toEqual(
    [],
  );
  expect(
    filterOverview(tasks, [], projects, "all", "FUSION", "recent").review,
  ).toHaveLength(1);
});

it("deduplicates PRs by repository and number without collapsing equal numbers across repos", () => {
  const prs = overviewReviews(projects, [
    queue(1, "org/sched"),
    queue(1, "org/sched"),
    queue(2, "org/fusion"),
    queue(99, "org/other"),
  ]);
  expect(prs).toHaveLength(2);
  expect(prs.every((pr) => pr.number === 7)).toBe(true);
  expect(
    filterOverview([], prs, projects, "all", "ALICE", "recent").prs,
  ).toHaveLength(2);
  expect(
    filterOverview([], prs, projects, "2", "7", "recent").prs,
  ).toHaveLength(1);
});

it("rejects cached results for a relinked repository and exposes stale PR provenance", () => {
  const entry = queue(1, "org/sched");
  entry.failed = true;
  expect(overviewReviews(projects, [entry])[0].stale).toBe(true);
  expect(
    overviewReviews([{ ...projects[0], github_repo: "org/new" }], [entry]),
  ).toEqual([]);
});

it("keeps shared-repository PRs in each project's filter and deduplicates the all-project view", () => {
  const shared = [...projects, { ...projects[0], id: 3, name: "sched tools" }];
  const prs = overviewReviews(shared, [
    queue(1, "org/sched"),
    queue(3, "org/sched"),
  ]);
  expect(filterOverview([], prs, shared, "3", "", "recent").prs).toHaveLength(
    1,
  );
  expect(filterOverview([], prs, shared, "all", "", "recent").prs).toHaveLength(
    1,
  );
  expect(
    filterOverview([], prs, shared, "all", "sched tools", "recent").prs,
  ).toHaveLength(1);
});

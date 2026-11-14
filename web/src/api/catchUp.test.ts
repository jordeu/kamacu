import { expect, it } from "vitest";
import { catchUpPRs, catchUpActionSucceeded, latestCatchUpMessage } from "./catchUp";
import type { FeedbackQuestion } from "./inbox";
import type { Project, Task } from "./types";

it("collects explicit PRs across tasks, visible message links and saved actions without reviving rejected associations", () => {
  const q = { source_url: "https://github.com/acme/one/pull/1", question: "Review?", context: "<!-- https://github.com/internal/hidden/pull/99 --> [PR](https://github.com/acme/two/pull/2#discussion_3) https://github.com/ACME/one/pull/1 https://github.com/acme/no/pull/3 https://github.com/acme/no/pull/4evil", options: [{ action: { actions: [{ project_id: 7, pr_numbers: [5] }] } }] } as FeedbackQuestion;
  const tasks = [{ pr_links: [{ repository: "acme/no", number: 3, role: "incidental" }, { repository: "acme/one", number: 1, role: "implementation" }] }] as Task[];
  const projects = [{ id: 7, github_repo: "acme/action" }] as Project[];
  expect(catchUpPRs(q, tasks, projects).map(pr => pr.url.toLowerCase())).toEqual(["https://github.com/acme/one/pull/1", "https://github.com/acme/two/pull/2", "https://github.com/acme/action/pull/5"]);
});

it("keeps a valid PR link when another task rejected that association", () => {
  const q = { options: [] } as unknown as FeedbackQuestion;
  const tasks = [{ pr_links: [{ repository: "acme/repo", number: 3, role: "incidental" }] }, { pr_links: [{ repository: "ACME/REPO", number: 3, role: "implementation" }] }] as Task[];
  expect(catchUpPRs(q, tasks, [])).toHaveLength(1);
});
it("recognizes a quoted PR reference without accepting a malformed number", () => {
  const q = { context: '`https://github.com/acme/repo/pull/7` "https://github.com/acme/repo/pull/8" https://github.com/acme/repo/pull/9bad', options: [] } as unknown as FeedbackQuestion;
  expect(catchUpPRs(q, [], []).map(pr => pr.number)).toEqual([7, 8]);
});

it("advances on the server's dismissal result without advancing on stale or previously resolved requests", () => {
  const question = { status: "dismissed" } as FeedbackQuestion;
  expect(catchUpActionSucceeded({ outcome: "dismissed", question }, "dismiss")).toBe(true);
  expect(catchUpActionSucceeded({ outcome: "already_resolved", question }, "dismiss")).toBe(false);
  expect(catchUpActionSucceeded({ outcome: "stale_revision", question }, "dismiss")).toBe(false);
  expect(catchUpActionSucceeded({ outcome: "accepted", question: { ...question, status: "answered" } }, "answer")).toBe(true);
});
it("keeps a newer polled message when an older mutation response arrives late", () => {
  const latest = { id: "q", revision: 3, updated_at: 10, status: "pending" } as FeedbackQuestion;
  const delayed = { ...latest, revision: 2, status: "answered" as const };
  expect(latestCatchUpMessage(latest, delayed)).toBe(latest);
  expect(latestCatchUpMessage(latest, { ...latest, updated_at: 9 })).toBe(latest);
  expect(latestCatchUpMessage(delayed, latest)).toBe(latest);
  expect(latestCatchUpMessage(undefined, latest)).toBe(latest);
});

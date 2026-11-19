import { expect, it } from "vitest";
import { catchUpPRs, catchUpHeadline, catchUpReadingText, catchUpSourceText, catchUpActionSucceeded, latestCatchUpMessage } from "./catchUp";
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

it("includes PRs from the decision explanation while respecting rejected links", () => {
  const q = { explanation: "Check https://github.com/acme/repo/pull/17 before merging. <!-- https://github.com/acme/hidden/pull/1 --> https://github.com/acme/rejected/pull/2", options: [] } as unknown as FeedbackQuestion;
  const tasks = [{ pr_links: [{ repository: "acme/rejected", number: 2, role: "incidental" }] }] as Task[];
  expect(catchUpPRs(q, tasks, []).map(pr => pr.url)).toEqual(["https://github.com/acme/repo/pull/17"]);
});


it("keeps source facts that occur only in the summary, including for legacy messages", () => {
  const q = { summary: "Merged, but still needs Terraform apply.", context: "The next nightly run must confirm the fix." } as FeedbackQuestion;
  expect(catchUpSourceText(q)).toBe("Merged, but still needs Terraform apply.\n\nThe next nightly run must confirm the fix.");
  expect(catchUpSourceText({ ...q, context: "  " })).toBe(q.summary);
});
it("does not repeat a source summary already present in the full context", () => {
  const q = { summary: "The fix is merged.", context: "The fix is merged.\n\nIt still needs to be deployed." } as FeedbackQuestion;
  expect(catchUpSourceText(q)).toBe(q.context);
  expect(catchUpSourceText({ ...q, summary: q.context, context: q.summary })).toBe(q.context);
});

it("keeps conflicting source text instead of treating a negated substring as a repeat", () => {
  const q = { summary: "Not approved.", context: "approved." } as FeedbackQuestion;
  expect(catchUpSourceText(q)).toBe("Not approved.\n\napproved.");
});


it("keeps a legacy headline's facts in the title without copying it into the body", () => {
  const q = { summary: "Merged; deployment still needs Terraform apply.", context: "Check the next nightly run after applying it." } as FeedbackQuestion;
  expect(catchUpHeadline(q)).toBe(q.summary);
  expect(catchUpReadingText(q)).toBe(q.context);
  expect(catchUpSourceText(q)).toContain(q.summary);
});
it("removes only an exact leading headline paragraph, preserving caveats and source formatting", () => {
  const q = { summary: "PR approved", context: "PR approved\n\n    check_artifact_version()\n\nCI still needs checking." } as FeedbackQuestion;
  expect(catchUpReadingText(q)).toBe("    check_artifact_version()\n\nCI still needs checking.");
  const qualified = "PR approved, but the tested artifact version is unknown.";
  expect(catchUpReadingText({ ...q, explanation: qualified })).toBe(qualified);
  expect(catchUpSourceText(q)).toBe(q.context);
});
it("falls back to context when the explanation adds nothing beyond the headline", () => {
  const q = { summary: "Fix merged", explanation: "Fix merged", context: "It is not released yet." } as FeedbackQuestion;
  expect(catchUpReadingText(q)).toBe(q.context);
  expect(catchUpReadingText({ ...q, context: "Fix merged" })).toBe("");
});

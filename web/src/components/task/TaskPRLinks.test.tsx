import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import type { Task, TaskPRLink } from "@/api/types";
import { TaskPRLinks } from "./TaskPRLinks";

const link: TaskPRLink = {
  task_id: 1, repository: "acme/app", number: 12,
  url: "https://github.com/acme/app/pull/12", role: "implementation",
};
const render = (links?: TaskPRLink[], compact = false) => renderToStaticMarkup(
  <QueryClientProvider client={new QueryClient()}><TaskPRLinks task={{ project_id: 1, pr_links: links } as Task} compact={compact} /></QueryClientProvider>,
);

it("renders a GitHub link with an accessible new-tab announcement", () => {
  const html = render([link]);
  expect(html).toContain(`href="${link.url}"`);
  expect(html).toContain('target="_blank"');
  expect(html).toContain('rel="noopener noreferrer"');
  expect(html).toContain("PR #12</span>");
  expect(html).toContain("Open acme/app PR #12 on GitHub (opens in a new tab)");
});

it("shows the most recently linked PR with a disclosure on compact cards", () => {
  const html = render([
    { ...link, linked_at: "2026-11-14T10:00:00Z", observed_at: "2026-11-14T10:00:00Z" },
    { ...link, number: 47, url: "https://github.com/acme/app/pull/47", linked_at: "2026-11-13T10:00:00Z", observed_at: "2026-11-15T10:00:00Z" },
    { ...link, number: 99, role: "incidental", observed_at: "2026-11-15T10:00:00Z" },
  ], true);
  expect(html).toContain(`href="${link.url}"`);
  expect(html).not.toContain("/pull/47");
  expect(html).not.toContain("#99");
  expect(html).toContain("Show 1 more linked pull request");
  expect(html).toContain('aria-expanded="false"');
});

it("uses the last entry when timestamps are absent and leaves single links alone", () => {
  const html = render([link, { ...link, number: 47, url: "https://github.com/acme/app/pull/47" }], true);
  expect(html).toContain("/pull/47");
  expect(html).not.toContain(`href="${link.url}"`);
  expect(render([link], true)).not.toContain("more linked pull");
});

it("reserves no space for missing or rejected associations", () => {
  expect(render()).toBe("");
  expect(render([])).toBe("");
  expect(render([{ ...link, role: "incidental" }])).toBe("");
});

it("distinguishes identical PR numbers across repositories and hides rejected links", () => {
  const html = render([
    link,
    { ...link, repository: "acme/api", url: "https://github.com/acme/api/pull/12", role: "review" },
    { ...link, number: 99, role: "incidental" },
  ]);
  expect(html).toContain("acme/app PR #12</span>");
  expect(html).toContain("acme/api PR #12</span>");
  expect(html).not.toContain("#99");
});

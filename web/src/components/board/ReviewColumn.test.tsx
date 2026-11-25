import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ReviewColumn } from "./ReviewColumn";
import { PRCard } from "./PRCard";
import type { PRSummary } from "@/api/pullRequests";

const scenario = vi.hoisted(() => ({ state: "ok", emptyAwaiting: false }));
beforeEach(() => { scenario.state = "ok"; scenario.emptyAwaiting = false; });

vi.mock("@/api/settings", () => ({
  useSettings: () => ({ data: { github_integration: { value: "on" } } }),
}));
vi.mock("@/api/queries", () => ({
  useProjects: () => ({ data: [{ id: 1, github_repo: "org/repo" }] }),
}));
vi.mock("@/api/agents", () => ({ useAgentStatuses: () => ({ data: [] }) }));
vi.mock("./ReviewPRPicker", () => ({ ReviewPRPicker: () => null }));
vi.mock("@/api/pullRequests", () => ({
  usePullRequests: () => ({ data: {
    state: scenario.state, stale: scenario.state !== "ok",
    prs: scenario.emptyAwaiting ? [] : [
      { number: 1, title: "Visible request", author: "dev", checks: "none" },
      { number: 2, title: "Hidden request", author: "dev", checks: "none", hidden: true },
    ],
    reviewed: [
      { number: 3, title: "Visible reviewed", author: "dev", checks: "none" },
      { number: 4, title: "Hidden reviewed", author: "dev", checks: "none", hidden: true },
    ],
  } }),
  useRefreshPullRequests: () => ({ isPending: false }),
  useOpenReview: () => ({ isPending: false, isError: false }),
  useSetPRHidden: () => ({ isPending: false, isError: false }),
}));

afterEach(() => vi.unstubAllGlobals());

it("excludes hidden PRs from both sections by default", () => {
  vi.stubGlobal("localStorage", { getItem: () => "0" });
  const html = renderToStaticMarkup(<MemoryRouter><ReviewColumn projectId={1} /></MemoryRouter>);
  expect(html).toContain("Visible request");
  expect(html).toContain("Visible reviewed");
  expect(html).not.toContain("Hidden request");
  expect(html).not.toContain("Hidden reviewed");
  expect(html).toContain('aria-label="Hide PR #1"');
  expect(html).toContain('aria-label="Filter PR visibility"');
});

it("excludes hidden requests from the collapsed count", () => {
  vi.stubGlobal("localStorage", { getItem: () => null });
  const html = renderToStaticMarkup(<MemoryRouter><ReviewColumn projectId={1} /></MemoryRouter>);
  expect(html).toContain("Expand review column (1 PRs)");
});

it("offers restore for hidden cards", () => {
  const pr = { number: 2, title: "Hidden request", checks: "none", hidden: true } as PRSummary;
  const html = renderToStaticMarkup(<MemoryRouter><PRCard pr={pr} projectId={1} allowHiding /></MemoryRouter>);
  expect(html).toContain('aria-label="Restore PR #2"');
  expect(html).toContain("Restore PR");
  expect(html).not.toContain("Hide PR");
  // The open action closes before the restore action begins; neither is
  // nested in another button or a container with a button role.
  expect(html).toMatch(/<button[^>]*aria-label="Open review for PR #2"[^>]*><\/button>/);
  expect(html).not.toContain('role="button"');
});

it("retains cached reviewed PRs when GitHub fails and no awaiting PRs match", () => {
  vi.stubGlobal("localStorage", { getItem: () => "0" });
  scenario.state = "error";
  scenario.emptyAwaiting = true;
  const html = renderToStaticMarkup(<MemoryRouter><ReviewColumn projectId={1} /></MemoryRouter>);
  expect(html).toContain("Visible reviewed");
  expect(html).toContain("Recently reviewed");
  expect(html).toContain("Try refresh.");
  expect(html).not.toContain("Hidden reviewed");
});

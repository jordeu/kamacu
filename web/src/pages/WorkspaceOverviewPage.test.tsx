import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import WorkspaceOverviewPage from "./WorkspaceOverviewPage";

const mocks = vi.hoisted(() => ({
  tasks: vi.fn(),
  reviews: vi.fn(),
  settings: vi.fn(),
}));
vi.mock("@/api/queries", () => ({
  useWorkspaces: () => ({ data: [{ id: 1, name: "Seqera" }] }),
  useProjects: () => ({
    data: [
      {
        id: 1,
        workspace_id: 1,
        name: "sched",
        github_repo: "org/sched",
        icon_letters: "SC",
        icon_color: "#84689e",
      },
    ],
  }),
}));
vi.mock("@/api/workspaceOverview", () => ({
  useWorkspaceTasks: mocks.tasks,
  useWorkspaceReviews: mocks.reviews,
}));
vi.mock("@/api/settings", () => ({ useSettings: mocks.settings }));
vi.mock("@/api/agents", () => ({ useAgentStatuses: () => ({ data: [] }) }));
vi.mock("@/api/pullRequests", () => ({
  useOpenReview: () => ({ isPending: false, isError: false }),
}));
vi.mock("@/lib/useActiveWorkspace", () => ({
  useActiveWorkspace: () => ({ setActiveWorkspaceId: () => {} }),
}));

const pr = {
  number: 17,
  title: "Fix restore",
  author: "alice",
  updatedAt: "2026-11-19T08:00:00Z",
  checks: "pass",
  url: "https://github.com/org/sched/pull/17",
};
const cached = {
  projectId: 1,
  repository: "org/sched",
  failed: false,
  data: { state: "ok", stale: false, fetchedAt: pr.updatedAt, prs: [pr] },
};
beforeEach(() => {
  mocks.tasks.mockReturnValue({ data: [], dataUpdatedAt: 1795075200000 });
  mocks.reviews.mockReturnValue({
    data: [cached],
    dataUpdatedAt: 1795075200000,
  });
  mocks.settings.mockReturnValue({
    data: { github_integration: { value: "on" } },
  });
});
function render(path = "/workspaces/1/overview") {
  return renderToStaticMarkup(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          path="/workspaces/:workspaceId/overview"
          element={<WorkspaceOverviewPage />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

it("renders the selected queue order and links tasks to their project", () => {
  mocks.tasks.mockReturnValue({
    data: [
      {
        id: 10,
        project_id: 1,
        title: "Restore task",
        source: "manual",
        status: "in_progress",
        updated_at: pr.updatedAt,
      },
    ],
    dataUpdatedAt: 1795075200000,
  });
  const html = render();
  expect(html.indexOf('id="overview-progress-heading"')).toBeLessThan(
    html.indexOf('id="overview-review-heading"'),
  );
  expect(html.indexOf('id="overview-review-heading"')).toBeLessThan(
    html.indexOf('id="overview-prs-heading"'),
  );
  expect(html).toContain('href="/projects/1/tasks/10"');
  expect(html).toContain(pr.url);
});

it("never reports an unknown PR queue as zero or caught up", () => {
  mocks.reviews.mockReturnValue({
    data: [{ ...cached, data: null, failed: true }],
    dataUpdatedAt: 1795075200000,
  });
  const html = render();
  expect(html).toContain('aria-label="PRs awaiting your review: —"');
  expect(html).toContain("PR count is partial");
  expect(html).not.toContain("No PRs are waiting for your review");
});

it("hides cached PRs when GitHub integration is turned off", () => {
  mocks.settings.mockReturnValue({
    data: { github_integration: { value: "off" } },
  });
  const html = render();
  expect(html).toContain("GitHub integration is turned off");
  expect(html).not.toContain(pr.url);
  expect(mocks.reviews).toHaveBeenLastCalledWith(1, expect.any(Array), false);
});

it("marks cached results and applies URL search filters", () => {
  mocks.reviews.mockReturnValue({
    data: [{ ...cached, data: { ...cached.data, stale: true } }],
    dataUpdatedAt: 1795075200000,
  });
  expect(render()).toContain("Cached");
  expect(render()).toContain('aria-label="PRs awaiting your review: 1 shown"');
  expect(render("/workspaces/1/overview?q=missing")).not.toContain(pr.url);
});

it("does not show an empty task queue when loading failed", () => {
  mocks.tasks.mockReturnValue({
    data: undefined,
    isError: true,
    dataUpdatedAt: 0,
  });
  const html = render();
  expect(html).toContain("Tasks are unavailable");
  expect(html).not.toContain("No tasks in progress");
});

it("does not report a rate-limited initial GitHub fetch as an empty queue", () => {
  mocks.reviews.mockReturnValue({
    data: [
      {
        ...cached,
        data: { state: "ok", stale: false, prs: null, fetchedAt: null },
      },
    ],
    dataUpdatedAt: 1795075200000,
  });
  const html = render();
  expect(html).toContain('aria-label="PRs awaiting your review: —"');
  expect(html).not.toContain("No PRs are waiting for your review");
});

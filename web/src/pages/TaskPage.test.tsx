import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import TaskPage from "./TaskPage";

const mocks = vi.hoisted(() => ({
  taskQuery: vi.fn(),
  scopedProjects: vi.fn(),
  prDetail: vi.fn(),
  navigate: vi.fn(),
}));
vi.mock("@/api/queries", () => ({
  useTask: mocks.taskQuery,
  useProjects: mocks.scopedProjects,
  useAgents: vi.fn(),
}));
vi.mock("@/api/pullRequests", () => ({ usePullRequestDetail: mocks.prDetail }));
vi.mock("react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-router")>()),
  Navigate: (props: { to: string; replace: boolean; state: unknown }) => {
    mocks.navigate(props);
    return <span>Redirecting to task owner</span>;
  },
}));
vi.mock("@/components/terminal/TerminalPane", () => ({
  TerminalPane: () => null,
}));
vi.mock("@/components/task/AgentTab", () => ({ AgentTab: () => null }));

beforeEach(() => vi.clearAllMocks());

function render() {
  return renderToStaticMarkup(
    <MemoryRouter
      initialEntries={[
        {
          pathname: "/projects/6/tasks/1288",
          search: "?feedback_session=agent",
          hash: "#details",
          state: { startAgent: true },
        },
      ]}
    >
      <Routes>
        <Route
          path="/projects/:projectId/tasks/:taskId"
          element={<TaskPage />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

it("resolves the task owner before mounting project-scoped consumers", () => {
  mocks.taskQuery.mockReturnValue({
    data: { id: 1288, project_id: 1, source: "github_pr", pr_number: 1939 },
    isPending: false,
    isError: false,
  });
  expect(render()).toContain("Redirecting to task owner");
  expect(mocks.taskQuery).toHaveBeenCalledWith(1288);
  expect(mocks.navigate).toHaveBeenCalledWith({
    to: "/projects/1/tasks/1288?feedback_session=agent#details",
    replace: true,
    state: { startAgent: true },
  });
  expect(mocks.scopedProjects).not.toHaveBeenCalled();
  expect(mocks.prDetail).not.toHaveBeenCalled();
});

it("does not mount task effects or fetch PRs while ownership is unresolved", () => {
  mocks.taskQuery.mockReturnValue({
    data: undefined,
    isPending: true,
    isError: false,
  });
  render();
  expect(mocks.navigate).not.toHaveBeenCalled();
  expect(mocks.scopedProjects).not.toHaveBeenCalled();
  expect(mocks.prDetail).not.toHaveBeenCalled();
});

it("does not redirect a missing task or mount project-scoped consumers", () => {
  mocks.taskQuery.mockReturnValue({
    data: undefined,
    isPending: false,
    isError: true,
  });
  expect(render()).toContain("Task not found");
  expect(mocks.navigate).not.toHaveBeenCalled();
  expect(mocks.scopedProjects).not.toHaveBeenCalled();
  expect(mocks.prDetail).not.toHaveBeenCalled();
});

it("never redirects to or mounts a task different from the requested id", () => {
  mocks.taskQuery.mockReturnValue({
    data: { id: 1287, project_id: 1, source: "github_pr", pr_number: 1939 },
    isPending: false,
    isError: false,
  });
  expect(render()).toContain("Task not found");
  expect(mocks.navigate).not.toHaveBeenCalled();
  expect(mocks.scopedProjects).not.toHaveBeenCalled();
  expect(mocks.prDetail).not.toHaveBeenCalled();
});

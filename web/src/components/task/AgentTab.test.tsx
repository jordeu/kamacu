import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentTab } from "./AgentTab";
import { useSpawnAgent, type TermSession } from "@/api/sessions";

vi.mock("@/api/queries", () => ({ useTask: () => ({ data: { worktree_path: "/worktree", branch: "task" } }) }));
vi.mock("@/api/global", () => ({ useGlobal: () => ({}) }));
vi.mock("@/api/agents", () => ({ useAgentStatuses: () => ({ data: [] }) }));
vi.mock("@/api/sessions", () => ({
  useSpawnAgent: vi.fn(),
  useResumeAgent: () => ({}),
  useSpawnGlobalAgent: () => ({}),
  useResumeGlobalAgent: () => ({}),
  useStopSession: () => ({}),
}));
vi.mock("@/components/terminal/TerminalPane", () => ({ TerminalPane: () => <div>Running terminal</div> }));

type Spawn = ReturnType<typeof useSpawnAgent>;
function show(state: Partial<Spawn>, session?: TermSession) {
  const shared = { mutate: vi.fn(), ...state } as unknown as Spawn;
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <AgentTab scope={{ kind: "task", taskId: 1 }} agentSession={session} taskSpawnMutation={shared} />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.mocked(useSpawnAgent).mockReturnValue({ mutate: vi.fn() } as unknown as Spawn);
});
describe("shared automatic and manual agent startup", () => {
  it("disables manual startup while the automatic request is pending", () => {
    const html = show({ isPending: true });
    expect(html).toContain("Starting…");
    expect(html).toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
  });
  it("offers a retry after failure and clears the error while retrying", () => {
    const failed = show({ isError: true, isPending: false });
    expect(failed).toContain("Couldn&#x27;t start a session");
    expect(failed).not.toMatch(/<button[^>]*\sdisabled(?:=|\s|>)/);
    expect(show({ isError: false, isPending: true })).not.toContain("Couldn&#x27;t start a session");
  });
  it("hides startup errors when a running agent session is available", () => {
    const html = show({ isError: true }, { id: "agent-1", status: "running" } as TermSession);
    expect(html).toContain("Running terminal");
    expect(html).not.toContain("Couldn&#x27;t start a session");
  });
});

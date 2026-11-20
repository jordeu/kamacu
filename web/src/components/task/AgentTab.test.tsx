import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentTab } from "./AgentTab";
import { useSpawnAgent, type TermSession } from "@/api/sessions";
import type { ReactNode } from "react";

const promptActions = vi.hoisted(() => ({
  sendPrompt: vi.fn(),
  items: new Map<string, () => void>(),
}));

vi.mock("@/api/queries", () => ({ useTask: () => ({ data: { worktree_path: "/worktree", branch: "task" } }) }));
vi.mock("@/api/global", () => ({ useGlobal: () => ({}) }));
vi.mock("@/api/agents", () => ({ useAgentStatuses: () => ({ data: [] }) }));
vi.mock("@/api/settings", () => ({ useSettings: () => ({ data: {
  custom_prompts: { value: JSON.stringify([{ title: "Explain changes", text: "Explain these changes.\nGive examples." }]) },
} }) }));
vi.mock("@/api/sessions", () => ({
  useSpawnAgent: vi.fn(),
  useResumeAgent: () => ({}),
  useSpawnGlobalAgent: () => ({}),
  useResumeGlobalAgent: () => ({}),
  useStopSession: () => ({}),
}));
vi.mock("@/components/terminal/TerminalPane", () => ({ TerminalPane: ({ headerMenu, onReady }: { headerMenu: ReactNode; onReady: (api: { sendPrompt: (text: string) => boolean }) => void }) => {
  onReady({ sendPrompt: promptActions.sendPrompt });
  return <div>Running terminal{headerMenu}</div>;
} }));
vi.mock("@/components/ui/dropdown-menu", () => {
  const Container = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return {
    DropdownMenu: Container, DropdownMenuTrigger: Container, DropdownMenuContent: Container,
    DropdownMenuSeparator: () => <hr />,
    DropdownMenuItem: ({ children, onSelect }: { children: ReactNode; onSelect: () => void }) => {
      promptActions.items.set(renderToStaticMarkup(<>{children}</>), onSelect);
      return <div>{children}</div>;
    },
  };
});

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
  promptActions.sendPrompt.mockClear();
  promptActions.sendPrompt.mockReturnValue(true);
  promptActions.items.clear();
  vi.mocked(useSpawnAgent).mockReturnValue({ mutate: vi.fn() } as unknown as Spawn);
});

describe("custom prompt insertion", () => {
  it("shows the saved title and sends the exact text only when selected", () => {
    const html = show({}, { id: "agent-1", status: "running" } as TermSession);
    expect(html).toContain("Explain changes");
    expect(promptActions.sendPrompt).not.toHaveBeenCalled();
    const action = [...promptActions.items.entries()].find(([label]) => label.includes("Explain changes"))?.[1];
    expect(action).toBeDefined();
    action!();
    expect(promptActions.sendPrompt).toHaveBeenCalledExactlyOnceWith("Explain these changes.\nGive examples.");
  });

  it("does not offer insertion for an exited session", () => {
    expect(show({}, { id: "agent-1", status: "exited" } as TermSession)).not.toContain("Explain changes");
    expect(promptActions.items.size).toBe(0);
  });
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

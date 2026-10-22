import { describe, expect, it } from "vitest";
import type { TermSession } from "@/api/sessions";
import { hasViewedAgentExited, shouldHandleAgentExit } from "./agentExit";

const agent: TermSession = {
  id: "agent-1", label: "Agent", kind: "agent", status: "exited",
  exitCode: 0, createdAt: "2026-10-22T12:00:00Z",
};

describe("agent process exit", () => {
  it("handles normal process exits and explicit stops", () => {
    expect(shouldHandleAgentExit(agent)).toBe(true);
    expect(shouldHandleAgentExit({ ...agent, exitCode: 143, stopRequested: true })).toBe(true);
  });
  it("ignores crashes and unknown exit codes", () => {
    expect(shouldHandleAgentExit({ ...agent, exitCode: 1 })).toBe(false);
    expect(shouldHandleAgentExit({ ...agent, exitCode: undefined })).toBe(false);
  });
  it("ignores response completion and shell exits", () => {
    for (const agentStatus of ["idle", "waiting", "working"] as const) {
      expect(shouldHandleAgentExit({ ...agent, status: "running", agentStatus })).toBe(false);
    }
    expect(shouldHandleAgentExit({ ...agent, kind: "bash" })).toBe(false);
  });
});

describe("viewed task exit", () => {
  it("returns to the board only for the agent observed on this task", () => {
    const viewed = { taskId: 42, sessionId: agent.id };
    expect(hasViewedAgentExited(viewed, 42, agent)).toBe(true);
    expect(hasViewedAgentExited(viewed, 43, agent)).toBe(false);
    expect(hasViewedAgentExited(viewed, 42, { ...agent, id: "another-agent" })).toBe(false);
  });
  it("leaves historical exits visible when reopening a task", () => {
    expect(hasViewedAgentExited(null, 42, agent)).toBe(false);
  });
});

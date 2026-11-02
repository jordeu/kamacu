import { describe, expect, it } from "vitest";
import type { TermSession } from "@/api/sessions";
import { hasViewedAgentExited, shouldHandleAgentExit } from "./agentExit";

const agent: TermSession = {
  id: "agent-1", label: "Agent", kind: "agent", status: "exited",
  exitCode: 0, exitSource: "terminal", createdAt: "2026-10-22T12:00:00Z",
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
  it("ignores external requests and unattributed exits", () => {
    expect(shouldHandleAgentExit({ ...agent, exitSource: "external" })).toBe(false);
    expect(shouldHandleAgentExit({ ...agent, exitSource: "external", stopRequested: true, exitCode: 143 })).toBe(false);
    expect(shouldHandleAgentExit({ ...agent, exitSource: undefined })).toBe(false);
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
    expect(hasViewedAgentExited(viewed, 42, agent, true)).toBe(true);
    expect(hasViewedAgentExited(viewed, 43, agent, true)).toBe(false);
    expect(hasViewedAgentExited(viewed, 42, { ...agent, id: "another-agent" }, true)).toBe(false);
  });
  it("does not prompt when another tab or an unfocused window is active", () => {
    expect(hasViewedAgentExited({ taskId: 42, sessionId: agent.id }, 42, agent, false)).toBe(false);
  });
  it("leaves historical exits visible when reopening a task", () => {
    expect(hasViewedAgentExited(null, 42, agent, true)).toBe(false);
  });
});

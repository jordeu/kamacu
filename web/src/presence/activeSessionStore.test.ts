import { describe, expect, it } from "vitest";
import { getActiveSession, setActiveSession, setTerminalViewObscured } from "./activeSessionStore";

describe("activeSessionStore", () => {
  it("stops reporting a covered terminal and restores the current selection on close", () => {
    setActiveSession("sess-a");
    setTerminalViewObscured(true);
    expect(getActiveSession()).toBeNull();
    setActiveSession("sess-b");
    expect(getActiveSession()).toBeNull();
    setTerminalViewObscured(false);
    expect(getActiveSession()).toBe("sess-b");
    setActiveSession(null);
  });
  it("round-trips the focused terminal session and resets to null", () => {
    setActiveSession("sess-a");
    expect(getActiveSession()).toBe("sess-a");
    setActiveSession(null);
    expect(getActiveSession()).toBeNull();
  });
});

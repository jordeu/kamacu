import { describe, expect, it } from "vitest";
import { getActiveSession, setActiveSession } from "./activeSessionStore";

describe("activeSessionStore", () => {
  it("round-trips the focused terminal session and resets to null", () => {
    setActiveSession("sess-a");
    expect(getActiveSession()).toBe("sess-a");
    setActiveSession(null);
    expect(getActiveSession()).toBeNull();
  });
});

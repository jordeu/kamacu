import { describe, expect, it } from "vitest";
import type { PRStatusSnapshot } from "@/api/prStatus";
import { primaryPRStatus } from "./prStatus";

export function snapshot(human = "approved", ci = "pass", bot = "comments"): PRStatusSnapshot {
  return { repository: "acme/app", number: 12, lifecycle: "open", head: "head",
    humans: { state: human, reviewers: [] }, bots: { state: bot, reviewers: [] },
    ci: { state: ci, jobs: [] }, fetched_at: "2026-11-13T10:00:00Z", stale: false };
}

describe("primary PR status", () => {
  it("keeps human approval and CI independent of every bot outcome", () => {
    for (const humans of ["approved", "waiting", "changes", "unknown", "none"]) {
      for (const ci of ["pass", "fail", "pending", "unknown", "none"]) {
        const base = primaryPRStatus(snapshot(humans, ci));
        for (const bot of ["comments", "running", "waiting", "clean", "unknown", "stale", "none", "changes"]) {
          expect(primaryPRStatus(snapshot(humans, ci, bot))).toEqual(base);
        }
      }
    }
  });
  it("uses CI failure, then human changes, before pending or positive results", () => {
    expect(primaryPRStatus(snapshot("changes", "fail")).label).toBe("CI failing");
    expect(primaryPRStatus(snapshot("changes", "pending")).tone).toBe("red");
    expect(primaryPRStatus(snapshot("waiting", "pass")).tone).toBe("amber");
    expect(primaryPRStatus(snapshot("approved", "pending")).tone).toBe("amber");
    expect(primaryPRStatus(snapshot("approved", "pass")).tone).toBe("green");
  });
  it("does not present missing or stale data as a fresh approval", () => {
    expect(primaryPRStatus().tone).toBe("neutral");
    expect(primaryPRStatus(snapshot("unknown")).tone).toBe("neutral");
    expect(primaryPRStatus(snapshot("none")).tone).toBe("neutral");
    expect(primaryPRStatus(snapshot("approved", "none")).tone).toBe("neutral");
    expect(primaryPRStatus({ ...snapshot(), stale: true }).tone).toBe("neutral");
    expect(primaryPRStatus({ ...snapshot("approved", "fail"), stale: true }).label).toBe("CI failing · last known");
  });
});

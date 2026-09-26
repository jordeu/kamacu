import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  parseRouteIds,
  startPresenceBeacon,
  type BeaconDeps,
  type HeartbeatBody,
} from "./beacon";

/**
 * The presence beacon core contract: the UI's "what am I looking at" beat.
 * Pure framework-free logic (injectable clock dependencies) so the timing,
 * visibility gating, and route parsing are provable without a DOM.
 */

interface Harness {
  beats: HeartbeatBody[];
  deps: BeaconDeps;
}

function harness(overrides?: Partial<BeaconDeps>): Harness {
  const beats: HeartbeatBody[] = [];
  const deps: BeaconDeps = {
    getClientId: () => "tab-1",
    getRoute: () => "/projects/3/tasks/7",
    getSessionId: () => "sess-a",
    getVisible: () => true,
    send: (beat) => beats.push(beat),
    intervalMs: 5000,
    ...overrides,
  };
  return { beats, deps };
}

describe("parseRouteIds", () => {
  it("parses board and task routes", () => {
    expect(parseRouteIds("/projects/3")).toEqual({ projectId: 3 });
    expect(parseRouteIds("/projects/3/tasks/7")).toEqual({
      projectId: 3,
      taskId: 7,
    });
  });

  it("returns nothing for non-project routes and garbage ids", () => {
    expect(parseRouteIds("/settings")).toEqual({});
    expect(parseRouteIds("/global")).toEqual({});
    expect(parseRouteIds("/projects/abc/tasks/def")).toEqual({});
  });
});

describe("startPresenceBeacon", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("sends an immediate beat on start with parsed ids, session, and client id", () => {
    const h = harness();
    startPresenceBeacon(h.deps);
    expect(h.beats).toHaveLength(1);
    expect(h.beats[0]).toEqual({
      client_id: "tab-1",
      route: "/projects/3/tasks/7",
      project_id: 3,
      task_id: 7,
      session_id: "sess-a",
      visible: true,
    });
  });

  it("sends on each interval tick while visible, with refreshed values", () => {
    const h = harness({
      getRoute: vi.fn()
        .mockReturnValueOnce("/projects/3/tasks/7")
        .mockReturnValueOnce("/projects/4")
        .mockReturnValueOnce("/global"),
      getSessionId: vi.fn()
        .mockReturnValueOnce("sess-a")
        .mockReturnValueOnce(undefined)
        .mockReturnValueOnce(undefined),
    });
    startPresenceBeacon(h.deps);
    vi.advanceTimersByTime(10_000);
    expect(h.beats).toHaveLength(3);
    expect(h.beats[1]).toEqual({
      client_id: "tab-1",
      route: "/projects/4",
      project_id: 4,
      visible: true,
    });
    expect(h.beats[2]).toEqual({
      client_id: "tab-1",
      route: "/global",
      visible: true,
    });
  });

  it("skips interval sends while hidden", () => {
    const h = harness({ getVisible: () => false });
    startPresenceBeacon(h.deps);
    vi.advanceTimersByTime(20_000);
    // Only the start beat — hidden tabs stay silent.
    expect(h.beats).toHaveLength(1);
    expect(h.beats[0].visible).toBe(false);
  });

  it("poke() sends immediately even while hidden (the final hide beat)", () => {
    const h = harness({ getVisible: () => false });
    const beacon = startPresenceBeacon(h.deps);
    beacon.poke();
    expect(h.beats).toHaveLength(2);
    expect(h.beats[1].visible).toBe(false);
  });

  it("stop() halts the interval", () => {
    const h = harness();
    const beacon = startPresenceBeacon(h.deps);
    beacon.stop();
    vi.advanceTimersByTime(60_000);
    expect(h.beats).toHaveLength(1);
  });
});

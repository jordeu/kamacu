import { beforeEach, describe, expect, it, vi } from "vitest";
import { get, post } from "./client";
import { dismissTaskCompletions } from "./inbox";

vi.mock("./client", () => ({ get: vi.fn(), post: vi.fn() }));
const message = (id: string) => ({ id, revision: 3 });
beforeEach(() => vi.resetAllMocks());

describe("opening a task", () => {
  it("uses one task-scoped snapshot even with more than one inbox page of completions", async () => {
    const signal = new AbortController().signal;
    vi.mocked(get).mockResolvedValue(Array.from({ length: 60 }, (_, i) => message(`m${i}`)));
    await dismissTaskCompletions(1, signal);
    expect(get).toHaveBeenCalledExactlyOnceWith("/api/inbox/tasks/1/completions", { signal });
    expect(post).toHaveBeenCalledTimes(60);
    expect(post).toHaveBeenLastCalledWith("/api/inbox/m59/dismiss", { revision: 3 }, { signal });
  });

  it("supports Scratchpad", async () => {
    const signal = new AbortController().signal;
    vi.mocked(get).mockResolvedValue([message("scratch")]);
    await dismissTaskCompletions(0, signal);
    expect(get).toHaveBeenCalledWith("/api/inbox/tasks/0/completions", { signal });
    expect(post).toHaveBeenCalledExactlyOnceWith("/api/inbox/scratch/dismiss", { revision: 3 }, { signal });
  });

  it("leaves the inbox untouched when no matching completion exists", async () => {
    vi.mocked(get).mockResolvedValue([]);
    await dismissTaskCompletions(1, new AbortController().signal);
    expect(post).not.toHaveBeenCalled();
  });

  it("does not dismiss a delayed snapshot after navigation aborts the visit", async () => {
    const controller = new AbortController();
    let resolve!: (items: unknown) => void;
    vi.mocked(get).mockReturnValue(new Promise((done) => { resolve = done; }));
    const pending = dismissTaskCompletions(1, controller.signal);
    controller.abort();
    // Even if the response wins the fetch/abort race, no dismiss is sent.
    resolve([message("arrived-after-leaving")]);
    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
    expect(post).not.toHaveBeenCalled();
  });

  it("stops sending dismissals when navigation occurs during a request", async () => {
    const controller = new AbortController();
    vi.mocked(get).mockResolvedValue([message("first"), message("second")]);
    vi.mocked(post).mockImplementation(async () => { controller.abort(); });
    await expect(dismissTaskCompletions(1, controller.signal)).rejects.toMatchObject({ name: "AbortError" });
    expect(post).toHaveBeenCalledTimes(1);
  });

  it("surfaces failures so the view can offer a retry", async () => {
    vi.mocked(get).mockRejectedValue(new Error("offline"));
    await expect(dismissTaskCompletions(1, new AbortController().signal)).rejects.toThrow("offline");
    expect(post).not.toHaveBeenCalled();
  });
});

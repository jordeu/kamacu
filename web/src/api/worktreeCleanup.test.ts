import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanEligible, type CleanupProgress } from "./worktreeCleanup";

const options = { include_unpushed: true, include_dirty: false };
const targets = [
  {
    repo: "/repo",
    path: "/wt",
    task_id: 1,
    project_name: "App",
    reason: "done" as const,
  },
];

afterEach(() => vi.unstubAllGlobals());

function mockStream(chunks: string[]) {
  const encoder = new TextEncoder();
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(
      new ReadableStream({
        start(controller) {
          for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
          controller.close();
        },
      }),
    ),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("cleanEligible progress", () => {
  it("handles records split across network chunks and submits the confirmed scope", async () => {
    const initial = {
      total: 1,
      completed: 0,
      removed: 0,
      skipped: 0,
      done: false,
    };
    const final = {
      total: 1,
      completed: 1,
      removed: 1,
      skipped: 0,
      done: true,
    };
    const data = JSON.stringify(initial) + "\n" + JSON.stringify(final) + "\n";
    const fetchMock = mockStream([
      data.slice(0, 17),
      data.slice(17, 51),
      data.slice(51),
    ]);
    const updates: CleanupProgress[] = [];
    await expect(
      cleanEligible(options, targets, (p) => updates.push(p)),
    ).resolves.toEqual(final);
    expect(updates).toEqual([initial, final]);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      ...options,
      targets,
    });
  });

  it("does not report completion when a stream ends without the final record", async () => {
    mockStream([
      JSON.stringify({
        total: 1,
        completed: 1,
        removed: 1,
        skipped: 0,
        done: false,
      }) + "\n",
    ]);
    await expect(cleanEligible(options, targets, vi.fn())).rejects.toThrow(
      "Cleanup was interrupted",
    );
  });

  it("surfaces a server error before streaming starts", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: "Worktree scan timed out" }), {
            status: 504,
          }),
        ),
    );
    const onProgress = vi.fn();
    await expect(cleanEligible(options, targets, onProgress)).rejects.toThrow(
      "Worktree scan timed out",
    );
    expect(onProgress).not.toHaveBeenCalled();
  });
});

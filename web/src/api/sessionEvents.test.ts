import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { subscribeSessionEvents } from "./sessionEvents";

afterEach(() => vi.unstubAllGlobals());

it("refreshes session lists and status dots on events and closes on unmount", async () => {
  const source = { onmessage: null as null | (() => void), close: vi.fn() };
  const EventSourceMock = vi.fn(function () { return source; });
  vi.stubGlobal("EventSource", EventSourceMock);
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const unsubscribe = subscribeSessionEvents(client);

  expect(EventSourceMock).toHaveBeenCalledWith("/api/sessions/events");
  source.onmessage?.();
  await vi.waitFor(() => expect(invalidate).toHaveBeenCalledTimes(2));
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["sessions"] });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["agent-statuses"] });
  unsubscribe();
  expect(source.close).toHaveBeenCalledOnce();
});

it.each([["sessions", 42], ["sessions", "global"], ["agent-statuses"]])(
  "replaces an initial request that predates an event (%s %s)",
  async (...queryKey) => {
    const source = { onmessage: null as null | (() => void), close: vi.fn() };
    vi.stubGlobal("EventSource", vi.fn(function () { return source; }));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    let resolveOld!: (data: string[]) => void;
    const oldResponse = new Promise<string[]>((resolve) => { resolveOld = resolve; });
    const queryFn = vi.fn()
      .mockReturnValueOnce(oldResponse)
      .mockResolvedValue(["current"]);
    const observer = new QueryObserver(client, { queryKey, queryFn });
    const stopObserver = observer.subscribe(() => {});
    const unsubscribe = subscribeSessionEvents(client);
    try {
      expect(queryFn).toHaveBeenCalledOnce();
      source.onmessage?.();
      await vi.waitFor(() => expect(client.getQueryData(queryKey)).toEqual(["current"]));
      resolveOld(["stale"]);
      await oldResponse;
      expect(client.getQueryData(queryKey)).toEqual(["current"]);
      expect(queryFn).toHaveBeenCalledTimes(2);
    } finally {
      unsubscribe();
      stopObserver();
      client.clear();
    }
  },
);

it("does not refresh after subscription cleanup", async () => {
  const source = { onmessage: null as null | (() => void), close: vi.fn() };
  vi.stubGlobal("EventSource", vi.fn(function () { return source; }));
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const unsubscribe = subscribeSessionEvents(client);
  source.onmessage?.();
  unsubscribe();
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(invalidate).not.toHaveBeenCalled();
});

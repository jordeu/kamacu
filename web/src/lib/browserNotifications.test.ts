import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { get } from "@/api/client";
import type { FeedbackQuestion } from "@/api/inbox";
import { browserNotificationPermission, InboxNotificationTracker, pendingNotificationMessages, requestBrowserNotifications, showInboxNotification, subscribeInboxNotifications } from "./browserNotifications";

vi.mock("@/api/client", () => ({ get: vi.fn() }));
const message = (id: string) => ({ id, question: `Question ${id}` }) as FeedbackQuestion;
beforeEach(() => vi.resetAllMocks());
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

describe("new inbox messages", () => {
  it("skips the initial backlog and detects arrivals even when the count stays the same", () => {
    const tracker = new InboxNotificationTracker();
    expect(tracker.update([message("old")], true)).toEqual([]);
    expect(tracker.update([message("new")], true)).toEqual([message("new")]);
    expect(tracker.update([message("new")], true)).toEqual([]);
    // A message archived and later reopened needs attention again.
    expect(tracker.update([message("old"), message("new")], true)).toEqual([message("old")]);
    expect(tracker.update([message("duplicate"), message("duplicate")], true)).toEqual([message("duplicate")]);
  });

  it("does not replay messages received while disabled or permission is missing", () => {
    const tracker = new InboxNotificationTracker();
    tracker.update([], false);
    expect(tracker.update([message("muted")], false)).toEqual([]);
    expect(tracker.update([message("muted")], true)).toEqual([]);
    expect(tracker.update([message("next"), message("muted")], true)).toEqual([message("next")]);
  });

  it("reads an atomic snapshot of the whole queue and forwards cancellation", async () => {
    const queue = Array.from({ length: 60 }, (_, index) => message(String(index)));
    vi.mocked(get).mockResolvedValueOnce(queue);
    const signal = new AbortController().signal;
    expect(await pendingNotificationMessages(signal)).toEqual(queue);
    expect(get).toHaveBeenCalledExactlyOnceWith("/api/inbox/catch-up", { signal });
  });

  it("leaves failed snapshots for the query to retry", async () => {
    vi.mocked(get).mockRejectedValueOnce(new Error("Offline"));
    await expect(pendingNotificationMessages(new AbortController().signal)).rejects.toThrow("Offline");
  });
});

describe("notification polling lifecycle", () => {
  function setup() {
    vi.useFakeTimers();
    const permission = { value: "granted" as NotificationPermission };
    const constructor = vi.fn(function () { return { close: vi.fn() }; });
    Object.defineProperty(constructor, "permission", { get: () => permission.value });
    vi.stubGlobal("Notification", constructor);
    vi.stubGlobal("window", { isSecureContext: true, Notification: constructor });
    vi.stubGlobal("navigator", {});
    return { constructor, permission };
  }

  it("cancels in-flight reads and timers when disabled, then baselines a fresh activation", async () => {
    const { constructor } = setup();
    vi.mocked(get).mockResolvedValueOnce([]).mockResolvedValueOnce([message("new")]);
    const stop = subscribeInboxNotifications(vi.fn());
    await vi.advanceTimersByTimeAsync(3000);
    expect(constructor).toHaveBeenCalledOnce();
    const signal = vi.mocked(get).mock.calls[0][1]!.signal!;
    stop();
    expect(signal.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(6000);
    expect(get).toHaveBeenCalledTimes(2);

    vi.mocked(get).mockResolvedValueOnce([message("new"), message("muted")]);
    const stopAgain = subscribeInboxNotifications(vi.fn());
    await vi.advanceTimersByTimeAsync(0);
    expect(constructor).toHaveBeenCalledOnce();
    stopAgain();
  });

  it("ignores a response that arrives after cleanup even if fetch ignores abort", async () => {
    const { constructor } = setup();
    let resolve!: (messages: FeedbackQuestion[]) => void;
    vi.mocked(get).mockResolvedValueOnce([]).mockReturnValueOnce(new Promise(done => { resolve = done; }));
    const stop = subscribeInboxNotifications(vi.fn());
    await vi.advanceTimersByTimeAsync(3000);
    stop();
    resolve([message("too-late")]);
    await vi.advanceTimersByTimeAsync(6000);
    expect(constructor).not.toHaveBeenCalled();
    expect(get).toHaveBeenCalledTimes(2);
  });

  it("checks current permission before delivery and retries failures without losing its baseline", async () => {
    const { constructor, permission } = setup();
    vi.mocked(get).mockResolvedValueOnce([]).mockRejectedValueOnce(new Error("Offline"))
      .mockResolvedValueOnce([message("new")]);
    const stop = subscribeInboxNotifications(vi.fn());
    await vi.advanceTimersByTimeAsync(6000);
    expect(constructor).toHaveBeenCalledOnce();
    permission.value = "denied";
    await vi.advanceTimersByTimeAsync(3000);
    expect(get).toHaveBeenCalledTimes(3);
    stop();
  });

  it("waits for the browser-wide lock before reading and cancels a waiting subscription", async () => {
    setup();
    let acquire!: () => Promise<void>;
    let signal!: AbortSignal;
    const request = vi.fn((_name, options, run) => {
      acquire = run;
      signal = options.signal;
      return new Promise<void>(() => {});
    });
    vi.stubGlobal("navigator", { locks: { request } });
    const stop = subscribeInboxNotifications(vi.fn());
    expect(request).toHaveBeenCalledWith("kamacu:inbox-notifications", expect.objectContaining({ signal: expect.any(AbortSignal) }), expect.any(Function));
    expect(get).not.toHaveBeenCalled();
    stop();
    expect(signal.aborted).toBe(true);
    await acquire();
    expect(get).not.toHaveBeenCalled();
  });
});

describe("browser support and delivery", () => {
  it("handles unsupported browsers and insecure contexts", () => {
    vi.stubGlobal("window", {});
    expect(browserNotificationPermission()).toBe("unsupported");
    vi.stubGlobal("window", { isSecureContext: false, Notification: {} });
    expect(browserNotificationPermission()).toBe("unsupported");
    vi.stubGlobal("Notification", { permission: "denied" });
    vi.stubGlobal("window", { isSecureContext: true, Notification });
    expect(browserNotificationPermission()).toBe("denied");
  });

  it("notifies permission subscribers even when permission requests fail", async () => {
    const dispatchEvent = vi.fn();
    vi.stubGlobal("window", { dispatchEvent });
    vi.stubGlobal("Notification", { requestPermission: vi.fn().mockRejectedValue(new Error("Unavailable")) });
    await requestBrowserNotifications();
    expect(dispatchEvent).toHaveBeenCalledWith(expect.objectContaining({ type: "kamacu:notification-permission" }));
  });

  it("opens the exact inbox message when clicked", () => {
    const notification = { close: vi.fn(), onclick: () => {} };
    const constructor = vi.fn(function () { return notification; });
    vi.stubGlobal("Notification", constructor);
    const focus = vi.fn();
    vi.stubGlobal("window", { focus });
    const open = vi.fn();
    showInboxNotification(message("a/b"), open);
    expect(constructor).toHaveBeenCalledWith("Kamacu · New inbox message", expect.objectContaining({ body: "Question a/b", tag: "kamacu:inbox:a/b" }));
    notification.onclick();
    expect(focus).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledWith("/inbox?message=a%2Fb");
    expect(notification.close).toHaveBeenCalledOnce();
  });

  it("survives browsers that reject the notification constructor", () => {
    vi.stubGlobal("Notification", vi.fn(function () { throw new Error("Unsupported"); }));
    expect(() => showInboxNotification(message("new"), vi.fn())).not.toThrow();
  });
});

import { get } from "@/api/client";
import type { FeedbackQuestion } from "@/api/inbox";

export function browserNotificationPermission(): NotificationPermission | "unsupported" {
  return typeof window !== "undefined" && window.isSecureContext && "Notification" in window
    ? Notification.permission
    : "unsupported";
}

export async function requestBrowserNotifications() {
  try {
    await Notification.requestPermission();
  } catch {
    // Some browsers expose the API but cannot grant permission here.
  }
  window.dispatchEvent(new Event("kamacu:notification-permission"));
}

// Catch up already provides the entire visible queue in a single server read.
export async function pendingNotificationMessages(signal: AbortSignal) {
  return get<FeedbackQuestion[]>("/api/inbox/catch-up", { signal });
}

export class InboxNotificationTracker {
  private seen = new Set<string>();
  private initialized = false;

  update(messages: FeedbackQuestion[], enabled: boolean) {
    const notify: FeedbackQuestion[] = [];
    const pending = new Set<string>();
    for (const message of messages) {
      if (!pending.has(message.id) && !this.seen.has(message.id) && this.initialized && enabled) notify.push(message);
      pending.add(message.id);
    }
    // Bound memory to the current queue and allow an archived message to notify
    // again when it is reopened. Content refreshes and requeueing stay silent.
    this.seen = pending;
    this.initialized = true;
    return notify;
  }
}

// One tab polls and delivers for this browser. Waiting tabs take over when the
// owner closes, loses permission, or disables notifications. Each activation
// starts with a fresh baseline, so muted messages and cached data cannot replay.
export function subscribeInboxNotifications(open: (url: string) => void) {
  const controller = new AbortController();
  const { signal } = controller;
  const tracker = new InboxNotificationTracker();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const stopped = new Promise<void>(resolve => {
    signal.addEventListener("abort", () => {
      clearTimeout(timer);
      resolve();
    }, { once: true });
  });
  const poll = async () => {
    try {
      if (browserNotificationPermission() !== "granted") return;
      const messages = await pendingNotificationMessages(signal);
      if (signal.aborted) return;
      for (const message of tracker.update(messages, browserNotificationPermission() === "granted")) {
        showInboxNotification(message, open);
      }
    } catch {
      // Retain the last successful snapshot and retry transient fetch failures.
    } finally {
      if (!signal.aborted) timer = setTimeout(() => void poll(), 3000);
    }
  };
  const run = async () => {
    if (signal.aborted) return;
    void poll();
    await stopped;
  };
  // Web Locks are available in modern browsers on the same secure origins
  // required for notifications. Older browsers still get notification tags.
  const running = navigator.locks
    ? navigator.locks.request("kamacu:inbox-notifications", { signal }, run)
    : run();
  void running.catch(() => { /* Cancelling a waiting lock is expected. */ });
  return () => controller.abort();
}

export function showInboxNotification(message: FeedbackQuestion, open: (url: string) => void) {
  try {
    const notification = new Notification("Kamacu · New inbox message", {
      body: message.question || message.summary || message.abstract,
      tag: `kamacu:inbox:${message.id}`,
      icon: "/favicon.svg",
    });
    notification.onclick = () => {
      window.focus();
      open(`/inbox?message=${encodeURIComponent(message.id)}`);
      notification.close();
    };
  } catch {
    // Unsupported constructors (including some mobile browsers) must not break the app.
  }
}

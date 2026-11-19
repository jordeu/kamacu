import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { SETTINGS_CHANGED_KEY, useSettings } from "@/api/settings";
import { Button } from "@/components/ui/button";
import { requestBrowserNotifications, subscribeInboxNotifications } from "@/lib/browserNotifications";
import { useBrowserNotificationPermission } from "@/lib/useBrowserNotificationPermission";

export function BrowserNotifications() {
  const settings = useSettings();
  const enabled = settings.data?.browser_notifications?.value === "on";
  const permission = useBrowserNotificationPermission();
  const navigate = useNavigate();
  const navigateRef = useRef(navigate);
  const client = useQueryClient();
  const [dismissed, setDismissed] = useState(() => sessionStorage.getItem("kamacu:notification-prompt-dismissed") === "1");
  useEffect(() => {
    navigateRef.current = navigate;
  }, [navigate]);
  useEffect(() => {
    // Declarative Router changes navigate's identity on route changes. Keep
    // the poller's baseline and tab ownership stable while navigating.
    if (enabled && permission === "granted") return subscribeInboxNotifications(url => navigateRef.current(url));
  }, [enabled, permission]);

  useEffect(() => {
    const refresh = (event: StorageEvent) => {
      if (event.key !== SETTINGS_CHANGED_KEY) return;
      const filter = { queryKey: ["settings"] };
      // An initial request may predate the save in the other tab. Replace it
      // rather than allowing invalidation to reuse that stale response.
      void client.cancelQueries(filter).then(() => client.invalidateQueries(filter));
    };
    window.addEventListener("storage", refresh);
    return () => window.removeEventListener("storage", refresh);
  }, [client]);

  if (!enabled || permission !== "default" || dismissed) return null;
  return <div role="status" className="fixed right-4 bottom-20 z-50 max-w-sm space-y-2 rounded-lg border border-border bg-background p-4 shadow-lg">
    <p className="text-sm">Get browser notifications for new inbox messages.</p>
    <div className="flex gap-2">
      <Button size="sm" onClick={() => void requestBrowserNotifications()}>Allow notifications</Button>
      <Button size="sm" variant="ghost" onClick={() => {
        sessionStorage.setItem("kamacu:notification-prompt-dismissed", "1");
        setDismissed(true);
      }}>Later</Button>
    </div>
  </div>;
}

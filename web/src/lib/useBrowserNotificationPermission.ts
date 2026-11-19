import { useEffect, useState } from "react";
import { browserNotificationPermission } from "./browserNotifications";

export function useBrowserNotificationPermission() {
  const [permission, setPermission] = useState(browserNotificationPermission);
  useEffect(() => {
    const refresh = () => setPermission(browserNotificationPermission());
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("kamacu:notification-permission", refresh);
    return () => {
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
      window.removeEventListener("kamacu:notification-permission", refresh);
    };
  }, []);
  return permission;
}

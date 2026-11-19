import { useSaveSetting, type Settings } from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { requestBrowserNotifications } from "@/lib/browserNotifications";
import { useBrowserNotificationPermission } from "@/lib/useBrowserNotificationPermission";

export function BrowserNotificationsSection({ settings }: { settings: Settings }) {
  const save = useSaveSetting("browser_notifications");
  const enabled = settings.browser_notifications?.value === "on";
  const permission = useBrowserNotificationPermission();
  return <section className="flex flex-col gap-3" aria-labelledby="browser-notifications-heading">
    <h2 id="browser-notifications-heading" className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Browser notifications</h2>
    <label className="flex items-center justify-between gap-3 text-sm">
      Notify me about new inbox messages
      <Switch checked={enabled} disabled={save.isPending} onCheckedChange={checked => save.mutate(checked ? "on" : "off")} />
    </label>
    <p className="text-xs text-muted-foreground">Enabled by default. Notifications appear while Kamacu is open, including in a background tab. Click a notification to open its message.</p>
    {permission === "unsupported" && <p className="text-xs text-muted-foreground">Browser notifications are unavailable here. Use a supported browser over HTTPS or localhost.</p>}
    {enabled && permission === "default" && <Button className="self-start" size="sm" variant="outline" onClick={() => void requestBrowserNotifications()}>Allow notifications in this browser</Button>}
    {enabled && permission === "denied" && <p className="text-xs text-muted-foreground">Notifications are blocked in this browser. Allow notifications for this site in your browser settings.</p>}
    {save.isError && <p role="alert" className="text-xs text-destructive">Couldn’t save. Try again.</p>}
  </section>;
}

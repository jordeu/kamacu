import { useSaveSetting, useSettings } from "@/api/settings";
import { Switch } from "@/components/ui/switch";

export function NotificationSchedule() {
 const query = useSettings();
 const save = useSaveSetting("notification_auto_process");
 const entry = query.data?.notification_auto_process;
 const enabled = !!entry && entry.value !== "off";
 return <div className="space-y-2">
   <div className="flex flex-wrap items-center gap-3">
     <Switch id="notification-auto-process" checked={enabled} disabled={!entry || save.isPending} onCheckedChange={checked => save.mutate(checked ? "20m" : "off")} />
     <label htmlFor="notification-auto-process" className="text-sm font-medium">Automatic processing</label>
     <select aria-label="Automatic processing interval" value={enabled ? entry.value : "20m"} disabled={!enabled || save.isPending} onChange={event => save.mutate(event.target.value)} className="h-9 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
       <option value="5m">Every 5 minutes</option>
       <option value="10m">Every 10 minutes</option>
       <option value="20m">Every 20 minutes (default)</option>
       <option value="40m">Every 40 minutes</option>
     </select>
     {save.isPending && <span role="status" className="text-xs text-muted-foreground">Saving…</span>}
   </div>
   <p className="text-xs text-muted-foreground">Process pending notifications in the background, even when this view is closed. Changing the interval starts a new wait.</p>
   {query.isError && <p role="alert" className="text-xs text-destructive">Could not load automatic processing settings.</p>}
   {save.isError && <p role="alert" className="text-xs text-destructive">{save.error.message}</p>}
 </div>;
}

import { Link } from "react-router";
import { Bell, ListFilter, LoaderCircle } from "lucide-react";
import { useNotificationStatus, useProcessNotifications } from "@/api/notifications";
import { Button } from "@/components/ui/button";

export function NotificationProcessing({ collapsed = false, onNavigate }: { collapsed?: boolean; onNavigate?: () => void }) {
 const query = useNotificationStatus();
 const start = useProcessNotifications();
 const count = query.data?.pending_count;
 const run = query.data?.processing;
 const processing = run?.status === "queued" || run?.status === "running";
 const disabled = count === undefined || count === 0 || processing || start.isPending;
 const label = processing ? `Processing ${run.notification_count} notifications` : "Process pending notifications";
 return <div className={collapsed ? "flex flex-col items-center gap-2 border-t border-border pt-3" : "space-y-2 border-b border-border p-3"}>
   <div className={collapsed ? "flex flex-col items-center gap-1" : "flex items-center gap-2"}>
     <Link onClick={onNavigate} to="/notifications" aria-label="Open raw notifications" title="Raw notifications" className="inline-flex min-h-8 items-center gap-2 rounded px-1 text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><Bell className="size-4" />{!collapsed && "Notifications"}</Link>
     <span className={`rounded-full px-2 py-0.5 text-xs font-semibold tabular-nums ${collapsed ? "" : "ml-auto"} ${(count ?? 0) > 0 ? "bg-blue-500/15 text-blue-400" : "text-muted-foreground"}`} aria-live="polite" aria-label={count === undefined ? "Loading pending notifications" : `${count} pending notifications`}>{count ?? "…"}</span>
   </div>
   <Button size={collapsed ? "icon-sm" : "sm"} variant="outline" disabled={disabled} onClick={() => start.mutate()} title={label} aria-label={label} className={collapsed ? undefined : "w-full"}>
     {processing || start.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <ListFilter className="size-4" />}{!collapsed && (processing ? "Processing…" : "Process pending")}
   </Button>
   {query.isError && <p role="alert" className={collapsed ? "sr-only" : "text-xs text-destructive"}>Could not load notification status.</p>}
   {start.isError && <p role="alert" className={collapsed ? "sr-only" : "text-xs text-destructive"}>{start.error.message}</p>}
   {run?.status === "failed" && <p role="alert" className={collapsed ? "sr-only" : "break-words text-xs text-destructive"}>{run.error} Pending notifications were kept.</p>}
   {!collapsed && run?.status === "completed" && <p role="status" className="text-xs text-muted-foreground">{run.notification_count} notifications processed · {run.action_count} inbox {run.action_count === 1 ? "action" : "actions"} created.</p>}
 </div>;
}

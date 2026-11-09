import { Link } from "react-router";
import { Bell, ListFilter, LoaderCircle } from "lucide-react";
import { useNotificationStatus, useProcessNotifications } from "@/api/notifications";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";

export function NotificationProcessing({ collapsed = false, onNavigate }: { collapsed?: boolean; onNavigate?: () => void }) {
 const query = useNotificationStatus();
 const start = useProcessNotifications();
 const count = query.data?.pending_count;
 const run = query.data?.processing;
 const processing = run?.status === "queued" || run?.status === "running";
 const configured = query.data?.model_configured !== false;
 const disabled = !configured || count === undefined || count === 0 || processing || start.isPending;
 const label = processing ? `Processing ${run.notification_count} notifications` : !configured ? "Choose a notification model in Settings" : "Process pending notifications";
 return <div className={collapsed ? "flex flex-col items-center gap-3 border-t border-border pt-4" : "max-h-[40%] shrink-0 space-y-3 overflow-y-auto border-t border-border bg-muted/20 p-4"}>
   <div className={collapsed ? "relative flex flex-col items-center" : "flex items-center gap-2"}>
     <Tooltip><TooltipTrigger asChild><Link onClick={onNavigate} to="/notifications" aria-label="Open raw notifications" title="Raw notifications" className="inline-flex min-h-9 min-w-9 items-center justify-center gap-2 rounded-md text-sm font-medium hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><Bell className="size-4" />{!collapsed && "Notifications"}</Link></TooltipTrigger><TooltipContent side="left">Notifications · {count ?? "…"} pending</TooltipContent></Tooltip>
     <span className={`rounded-full py-0.5 text-center font-semibold tabular-nums ${collapsed ? "pointer-events-none absolute -right-1.5 -top-1.5 min-w-4 px-1 text-[10px] ring-2 ring-sidebar" : "ml-auto px-2 text-xs"} ${(count ?? 0) > 0 ? "bg-blue-500/15 text-blue-400" : "text-muted-foreground"}`} aria-live="polite" aria-label={count === undefined ? "Loading pending notifications" : `${count} pending notifications`}>{collapsed && (count ?? 0) > 99 ? "99+" : count ?? "…"}</span>
   </div>
   {!collapsed && <p className="text-xs leading-relaxed text-muted-foreground">Turn incoming activity into actionable inbox messages.</p>}
   <Tooltip><TooltipTrigger asChild><Button size={collapsed ? "icon-sm" : "sm"} variant="outline" disabled={disabled} onClick={() => start.mutate()} title={label} aria-label={label} className={collapsed ? "size-9" : "w-full"}>
     {processing || start.isPending ? <LoaderCircle className="size-4 animate-spin" /> : <ListFilter className="size-4" />}{!collapsed && (processing ? "Processing…" : "Process pending")}
   </Button></TooltipTrigger><TooltipContent side="left">{label}</TooltipContent></Tooltip>
   {!configured && !collapsed && <p className="text-xs"><Link className="underline" to="/settings">Choose a notification model in Settings</Link> before processing.</p>}
   {query.isError && <p role="alert" className={collapsed ? "sr-only" : "text-xs text-destructive"}>Could not load notification status.</p>}
   {start.isError && <p role="alert" className={collapsed ? "sr-only" : "text-xs text-destructive"}>{start.error.message}</p>}
   {run?.status === "failed" && <p role="alert" className={collapsed ? "sr-only" : "break-words text-xs text-destructive"}>{run.error} Pending notifications were kept.</p>}
   {!collapsed && run?.status === "completed" && <p role="status" className="text-xs text-muted-foreground">{run.notification_count} notifications processed · {run.action_count} inbox {run.action_count === 1 ? "action" : "actions"} created.</p>}
 </div>;
}

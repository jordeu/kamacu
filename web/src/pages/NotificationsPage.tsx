import { useState } from "react";
import { ArrowUpRight, Bell, Check, Clock } from "lucide-react";
import { useArchiveNotification, useNotificationStatus, useRawNotifications } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import { NotificationProcessing } from "@/components/inbox/NotificationProcessing";

export default function NotificationsPage() {
 const [view, setView] = useState<"pending" | "archived">("pending");
 const [offset, setOffset] = useState(0);
 const query = useRawNotifications(view, offset);
 const status = useNotificationStatus();
 const change = useArchiveNotification();
 return <main className="h-full overflow-y-auto p-4 sm:p-6">
   <div className="mx-auto max-w-5xl">
     <h1 className="text-2xl font-semibold tracking-tight">Notifications</h1>
     <p className="mt-1 text-sm text-muted-foreground">Collected source notifications. Nothing is sent to the model until you choose Process pending.</p>
     <div className="mt-5 max-w-md overflow-hidden rounded border border-border"><NotificationProcessing /></div>
     <div className="mt-6 flex gap-2 border-b border-border pb-3" role="group" aria-label="Notification view">
       {(["pending", "archived"] as const).map(tab => <Button key={tab} variant={view === tab ? "secondary" : "ghost"} aria-pressed={view === tab} onClick={() => { setView(tab); setOffset(0); }}>
         {tab === "pending" ? <Clock className="size-4" /> : <Check className="size-4" />}{tab === "pending" ? `Pending (${status.data?.pending_count ?? query.data?.pending_count ?? "…"})` : "Archived"}
       </Button>)}
     </div>
     {change.isError && <p role="alert" className="my-3 text-sm text-destructive">{change.error.message}</p>}
     {query.isPending && <p className="py-8 text-sm text-muted-foreground">Loading notifications…</p>}
     {query.isError && <p role="alert" className="py-4 text-sm">Could not load notifications. <Button variant="ghost" onClick={() => void query.refetch()}>Retry</Button></p>}
     {query.data?.items.length === 0 && <div className="py-16 text-center text-muted-foreground"><Bell className="mx-auto mb-3 size-6" /><p>No {view} notifications.</p></div>}
     <div className="divide-y divide-border">{query.data?.items.map(item => <article key={item.id} className="space-y-3 py-5">
       <div className="flex flex-wrap items-start justify-between gap-3">
         <div className="min-w-0 flex-1">
           <p className="text-xs text-muted-foreground">{item.source_name} · {item.event.repository} · {item.event.reason.replaceAll("_", " ")}</p>
           <h2 className="mt-1 break-words text-sm font-medium"><a href={item.event.url} target="_blank" rel="noopener noreferrer" className="hover:underline">{item.event.title || item.event.subject_type} <ArrowUpRight className="inline size-3" /></a></h2>
           <p className="mt-1 text-xs text-muted-foreground">{item.event.subject_type} · <time dateTime={new Date(item.created_at).toISOString()}>{new Date(item.created_at).toLocaleString()}</time>{item.event.version && ` · Updated ${item.event.version}`}</p>
         </div>
         <Button size="sm" variant="outline" disabled={change.isPending || item.processing} title={item.processing ? "This notification is in the current processing run" : undefined} onClick={() => change.mutate({ id: item.id, status: view === "archived" ? "pending" : "archived" })}>{view === "archived" ? "Move to pending" : "Archive"}</Button>
       </div>
       {!!item.decisions?.length && <details className="text-xs"><summary className="cursor-pointer">Processing decisions and rules</summary>{item.decisions.map((d, i) => <div key={`${d.activity_id}-${i}`} className="mt-2 rounded bg-muted p-2"><strong>{d.outcome}</strong> · rule revision {d.rule_set_revision}{d.evaluations.map(e => <p key={e.clause_id}>{e.clause_id}: {e.result} → {e.effect}{e.result === "unknown" && ` · missing evidence: ${e.on_unknown}`}{e.reason && ` · ${e.reason}`}</p>)}<details><summary>Evidence and facts</summary><pre className="whitespace-pre-wrap break-words">{JSON.stringify({ facts: d.facts, semantic: d.semantic }, null, 2)}</pre></details></div>)}</details>}
       <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">Raw notification data</summary><pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-3">{JSON.stringify(item.event, null, 2)}</pre></details>
     </article>)}</div>
     {(offset > 0 || query.data?.has_more) && <nav aria-label="Notification pages" className="mt-5 flex justify-between"><Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</Button><Button variant="outline" disabled={!query.data?.has_more} onClick={() => setOffset(offset + 50)}>Next</Button></nav>}
   </div>
 </main>;
}

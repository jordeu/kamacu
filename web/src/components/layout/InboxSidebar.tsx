import { useEffect, useId, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ArrowUpRight, CheckCheck, ChevronLeft, ChevronRight, Inbox } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Button, buttonVariants } from "@/components/ui/button";
import { completionActions, feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, useInbox, useInboxMessage, type FeedbackQuestion } from "@/api/inbox";
import { useInboxFavicon } from "@/lib/useInboxFavicon";

import { NotificationProcessing } from "@/components/inbox/NotificationProcessing";
import { InboxMessage } from "@/components/inbox/InboxMessage";
import { InboxText, InboxTextAction } from "@/components/inbox/InboxText";
import { useIsMobile } from "@/hooks/use-mobile";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";

function MessageDialog({ question, onOpenChange, onResult, onRestoreFocus }: { question: FeedbackQuestion; onRestoreFocus: () => void; onOpenChange: (open: boolean) => void; onResult: (message: string) => void }) {
  const selected = useInboxMessage(question.id);
  const [notice, setNotice] = useState("");
  return <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl" aria-describedby={undefined} onCloseAutoFocus={(event) => { event.preventDefault(); onRestoreFocus(); }}>
    <DialogTitle className="pr-8">Inbox message</DialogTitle>
    {selected.isError && <p role="alert">Couldn’t refresh this message. <Button variant="ghost" onClick={() => void selected.refetch()}>Retry</Button></p>}
    {notice && <p role="status">{notice}</p>}
    <InboxMessage onNavigate={() => onOpenChange(false)} key={selected.data?.revision ?? question.revision} question={selected.data ?? question} dialog onResult={(message, keepOpen) => { if (keepOpen) setNotice(message); else onOpenChange(false); onResult(message); }} />
  </DialogContent>;
}

export function InboxSidebar() {
  const isMobile = useIsMobile();
  const isCompact = useIsMobile(1280);
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("kamacu:inbox-collapsed") !== "0");
  const drawerRef = useRef<HTMLDivElement>(null);
  const opener = useRef<HTMLButtonElement | null>(null);
  const toggleButton = useRef<HTMLButtonElement | null>(null);
  const [selected, setSelected] = useState<FeedbackQuestion | null>(null);
  const [offset, setOffset] = useState(0);
  const [notice, setNotice] = useState("");
  const inbox = useInbox("pending", offset, "oldest");
  // Keep the global count subscribed to a stable query while message pages load.
  const pendingCount = useInbox("pending", 0, "oldest");
  function toggle() {
    setCollapsed((current) => {
      localStorage.setItem("kamacu:inbox-collapsed", current ? "0" : "1");
      return !current;
    });
  }
  function collapse() {
    localStorage.setItem("kamacu:inbox-collapsed", "1");
    setCollapsed(true);
  }
  const count = pendingCount.data?.pending_count;
  useInboxFavicon(count);
  const panel = <div className={collapsed ? "flex h-full min-h-0 flex-col items-center gap-4 py-4" : "flex h-full min-h-0 w-full flex-col"}>
      <div className={`flex items-center gap-2 ${collapsed ? "relative flex-col" : "shrink-0 border-b border-border px-4 py-3"}`}>
        <Tooltip><TooltipTrigger asChild><Button ref={toggleButton} size="icon-sm" variant={collapsed && (count ?? 0) > 0 ? "secondary" : "ghost"} className={collapsed ? `size-9 ${ (count ?? 0) > 0 ? "bg-orange-500/10 text-orange-400 ring-1 ring-orange-400/40 hover:bg-orange-500/20" : ""}` : undefined} onClick={toggle} aria-expanded={!collapsed} aria-controls="inbox-sidebar-messages" aria-label={`${collapsed ? "Expand" : "Collapse"} inbox sidebar`} title={collapsed ? "Expand inbox" : "Collapse inbox"}>
          {collapsed ? <Inbox className="size-4" /> : <ChevronRight className="size-4" />}
        </Button></TooltipTrigger><TooltipContent side="left">{collapsed ? "Expand inbox" : "Collapse inbox"}</TooltipContent></Tooltip>
        {!collapsed && <Link to="/inbox" onClick={collapse} className="min-w-0 flex-1 text-sm font-semibold hover:underline">Inbox</Link>}
        <span aria-label={count === undefined ? "Loading pending messages" : `${count} pending messages`} aria-live="polite" className={`${collapsed ? "pointer-events-none absolute -right-1.5 -top-1.5 min-w-4 px-1 text-[10px] ring-2 ring-sidebar" : "px-2 text-xs"} rounded-full py-0.5 text-center font-semibold tabular-nums ${(count ?? 0) > 0 ? "bg-orange-500/20 text-orange-400" : "text-muted-foreground"}`}>{count === undefined ? "…" : collapsed && count > 99 ? "99+" : count}</span>
        {!collapsed && <Button asChild size="icon-sm" variant="ghost"><Link to="/inbox" onClick={collapse} aria-label="Open full inbox"><ArrowUpRight className="size-4" /></Link></Button>}
      </div>
      {!collapsed && <div id="inbox-sidebar-messages" className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        <div className="flex items-center justify-between px-4 pb-2 pt-4 text-xs text-muted-foreground"><span className="font-medium">Waiting for you</span><span>Oldest first</span></div>
        {notice && <p role="status" className="border-b border-border p-3 text-xs">{notice}</p>}
        {inbox.isError && <div role="alert" className="p-3 text-sm">Couldn’t refresh inbox.<Button variant="ghost" size="sm" onClick={() => void inbox.refetch()}>Retry</Button></div>}
        {inbox.isPending && <p className="p-4 text-sm text-muted-foreground">Loading messages…</p>}
        {inbox.data?.items.length === 0 && <div className="flex flex-col items-center gap-2 px-6 py-10 text-center"><CheckCheck className="size-7 text-muted-foreground" aria-hidden="true" /><p className="text-sm font-medium">{offset ? "No more pending messages." : "You’re all caught up"}</p><p className="text-xs text-muted-foreground">New requests and completed tasks will appear here.</p></div>}
        {inbox.data?.items.map((q) => <CompactMessage key={`${q.id}:${q.revision}`} question={q} onOpenTask={collapse} onOpenMessage={(button) => { opener.current = button; setSelected(q); }} onResult={setNotice} />)}
        {(offset > 0 || inbox.data?.has_more) && <nav aria-label="Sidebar inbox pages" className="flex justify-between p-3"><Button variant="ghost" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}><ChevronLeft className="size-4" />Previous</Button><Button variant="ghost" size="sm" disabled={!inbox.data?.has_more} onClick={() => setOffset(offset + 50)}>Next</Button></nav>}
      </div>}
      <NotificationProcessing collapsed={collapsed} onNavigate={collapse} />
    </div>;
  return <Dialog open={!!selected} onOpenChange={(open) => { if (!open) setSelected(null); }}><aside aria-label="Inbox sidebar" className={`relative hidden min-h-0 shrink-0 md:flex flex-col border-l border-border bg-sidebar pb-9 ${collapsed ? "w-12" : "w-12 xl:w-96"}`}>
    {isCompact && !isMobile && !collapsed ? <>
      <Button ref={toggleButton} size="icon-sm" variant="ghost" className="mx-auto mt-4" onClick={toggle} aria-label="Collapse inbox sidebar" aria-expanded aria-controls="inbox-sidebar-messages"><Inbox className="size-4" /></Button>
      <Sheet open onOpenChange={(open) => { if (!open) collapse(); }}>
        <SheetContent ref={drawerRef} side="right" aria-describedby={undefined} onOpenAutoFocus={(event) => { event.preventDefault(); drawerRef.current?.focus(); }} showCloseButton={false} className="data-[side=right]:w-[min(24rem,calc(100vw-3rem))] sm:max-w-none p-0" onCloseAutoFocus={(event) => { event.preventDefault(); toggleButton.current?.focus(); }}>
          <SheetHeader className="sr-only"><SheetTitle>Inbox</SheetTitle></SheetHeader>
          {panel}
        </SheetContent>
      </Sheet>
    </> : panel}
    {selected && <MessageDialog onRestoreFocus={() => { (opener.current?.isConnected ? opener.current : toggleButton.current)?.focus(); }} question={selected} onOpenChange={() => setSelected(null)} onResult={setNotice} />}
  </aside></Dialog>;
}

function MessagePreview({ text }: { text: string }) {
  const id = useId();
  const preview = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [clipped, setClipped] = useState(false);

  useEffect(() => {
    const element = preview.current;
    if (!element || expanded) return;
    const observer = new ResizeObserver(() => {
      setClipped(element.scrollHeight > element.clientHeight);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [text, expanded]);

  return <div className="space-y-1">
    <div ref={preview} id={id} className={`${expanded ? "" : "line-clamp-3"} whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground`}><InboxText text={text} /></div>
    {(clipped || expanded) && <button type="button" aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(!expanded)} className="rounded-sm text-xs font-medium text-muted-foreground underline underline-offset-4 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">{expanded ? "Show less" : "Show more"}</button>}
  </div>;
}

function CompactMessage({ question: q, onOpenTask, onOpenMessage, onResult }: { question: FeedbackQuestion; onOpenTask: () => void; onOpenMessage: (button: HTMLButtonElement) => void; onResult: (message: string) => void }) {
  const action = useFeedbackAction();
  const navigate = useNavigate();
  const notification = q.origin === "notification";
  const completion = q.kind === "completion";
  async function submit(kind: "answer" | "dismiss", option_id?: string, openTask = false) {
    try {
      // Await the mutation directly: polling can unmount this card before it finishes.
      const result = await action.mutateAsync({ question: q, action: kind, option_id });
      onResult(feedbackOutcomeMessage(result));
      if (openTask && result.outcome !== "stale_revision" && result.question.status !== "pending") {
        onOpenTask();
        navigate(feedbackTaskURL(q));
      }
    } catch {
      // The mutation exposes its error to the inline alert.
    }
  }
  return <article className="mx-3 mb-3 space-y-3 rounded-xl border border-border bg-background/60 p-3 shadow-sm" aria-label={`Message from ${notification ? q.source_name || "Notification" : q.task_id === 0 ? "Scratchpad" : q.task_title}`}>
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0 space-y-1 text-xs text-muted-foreground">
        {!notification && q.task_id !== 0 && <p className="break-words">{q.project_name}</p>}
        <p className="break-words font-medium">{notification ? q.source_name || "Notification" : q.task_id === 0 ? "Scratchpad" : q.task_title}</p>
      </div>
      <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium ${completion ? "bg-emerald-500/10 text-emerald-400" : "bg-orange-500/10 text-orange-400"}`}>{completion ? "Completed" : "Needs reply"}</span>
    </div>
    <InboxTextAction text={q.question} aria-haspopup="dialog" onClick={(event) => onOpenMessage(event.currentTarget)} className="block w-full whitespace-pre-wrap break-words rounded-sm text-left text-sm font-semibold leading-snug hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" />
    <MessagePreview text={q.abstract || q.summary} />
    {!completion && q.options.length > 0 && <div className="flex flex-col gap-2" role="group" aria-label="Choose an answer to send immediately">
      {q.options.map((option) => <InboxTextAction text={option.label} key={option.id} className={`${buttonVariants({ variant: "outline", size: "sm" })} h-auto min-h-9 max-w-full justify-start whitespace-normal break-words text-left`} disabled={action.isPending} onClick={() => submit("answer", option.id)} />)}
    </div>}
    <div className="flex flex-wrap items-center gap-2 border-t border-border/60 pt-2">
      {notification ? <a href={q.source_url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-xs text-blue-400">Open notification <ArrowUpRight className="size-3" /></a> : <Button variant="outline" size="sm" disabled={action.isPending} title={q.origin === "observer" && !completion ? "Open its agent terminal" : "Archive this message and open its agent terminal"} onClick={() => { if(q.origin==="observer" && !completion){onOpenTask();navigate(feedbackTaskURL(q));}else void submit("dismiss", undefined, true); }}>{q.task_id === 0 ? "Open Scratchpad" : "Open task"}<ArrowUpRight className="size-3" /></Button>}
      {completion && completionActions(q).map(option => <InboxTextAction text={option.label} key={option.id} className={`${buttonVariants({ variant: "outline", size: "sm" })} h-auto min-h-7 max-w-full whitespace-normal text-left`} disabled={action.isPending} onClick={() => submit("answer", option.id)} />)}
      <Button variant="ghost" size="sm" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>
    </div>
    {!completion && <Link to={`/inbox?message=${encodeURIComponent(q.id)}#message-${q.id}`} className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">{q.delivery_mode === "claude_question" ? "Full context / choices" : "Full context / custom reply"}<ArrowUpRight className="size-3" /></Link>}
    {action.isPending && <p role="status" className="text-xs text-muted-foreground">Updating message…</p>}
    {action.isError && <p role="alert" className="break-words text-xs text-destructive">{action.error.message} Try again.</p>}
  </article>;
}

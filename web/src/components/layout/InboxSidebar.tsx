import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { ArrowUpRight, ChevronLeft, ChevronRight, Inbox } from "lucide-react";
import { Button } from "@/components/ui/button";
import { feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, useInbox, type FeedbackQuestion } from "@/api/inbox";
import { AgentExitDialog } from "@/components/task/AgentExitDialog";
import { useInboxFavicon } from "@/lib/useInboxFavicon";

export function InboxSidebar() {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("kamacu:inbox-collapsed") !== "0");
  const [offset, setOffset] = useState(0);
  const [notice, setNotice] = useState("");
  const [exited, setExited] = useState<FeedbackQuestion | null>(null);
  const inbox = useInbox("pending", offset);
  // Keep the global count subscribed to a stable query while message pages load.
  const pendingCount = useInbox();
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
  return <aside aria-label="Inbox sidebar" className={`relative shrink-0 border-l border-border bg-sidebar pb-9 ${collapsed ? "w-12" : "w-12 sm:w-96"}`}>
    {exited && <AgentExitDialog taskId={exited.task_id} projectId={exited.project_id} onClose={() => setExited(null)} />}
    <div className={collapsed ? "flex h-full flex-col items-center gap-3 py-3" : "absolute inset-y-0 right-0 z-20 flex w-[min(24rem,calc(100vw-3rem))] flex-col border-l border-border bg-sidebar pb-9 sm:static sm:h-full sm:w-full sm:border-0 sm:pb-0"}>
      <div className={`flex items-center gap-2 ${collapsed ? "flex-col" : "border-b border-border p-3"}`}>
        <Button size="icon-sm" variant={collapsed && (count ?? 0) > 0 ? "secondary" : "ghost"} className={collapsed && (count ?? 0) > 0 ? "bg-orange-500/15 text-orange-400 ring-1 ring-orange-400/50 hover:bg-orange-500/25" : undefined} onClick={toggle} aria-expanded={!collapsed} aria-controls="inbox-sidebar-messages" aria-label={`${collapsed ? "Expand" : "Collapse"} inbox sidebar`} title={collapsed ? "Expand inbox" : "Collapse inbox"}>
          {collapsed ? <Inbox className="size-4" /> : <ChevronRight className="size-4" />}
        </Button>
        {!collapsed && <Link to="/inbox" className="min-w-0 flex-1 text-sm font-semibold hover:underline">Inbox</Link>}
        <span aria-label={count === undefined ? "Loading pending messages" : `${count} pending messages`} aria-live="polite" className={`rounded-full px-2 py-0.5 text-xs font-semibold tabular-nums ${(count ?? 0) > 0 ? "bg-orange-500/20 text-orange-400" : "text-muted-foreground"}`}>{count === undefined ? "…" : count}</span>
        {!collapsed && <Button asChild size="icon-sm" variant="ghost"><Link to="/inbox" aria-label="Open full inbox"><ArrowUpRight className="size-4" /></Link></Button>}
      </div>
      {!collapsed && <div id="inbox-sidebar-messages" className="min-h-0 flex-1 overflow-y-auto">
        {notice && <p role="status" className="border-b border-border p-3 text-xs">{notice}</p>}
        {inbox.isError && <div role="alert" className="p-3 text-sm">Couldn’t refresh inbox.<Button variant="ghost" size="sm" onClick={() => void inbox.refetch()}>Retry</Button></div>}
        {inbox.isPending && <p className="p-4 text-sm text-muted-foreground">Loading messages…</p>}
        {inbox.data?.items.length === 0 && <p className="p-4 text-sm text-muted-foreground">{offset ? "No more pending messages." : "No messages waiting."}</p>}
        {inbox.data?.items.map((q) => <CompactMessage key={`${q.id}:${q.revision}`} question={q} onOpenTask={collapse} onResult={(message, exit) => { setNotice(message); if (exit && exit.task_id > 0) setExited(exit); }} />)}
        {(offset > 0 || inbox.data?.has_more) && <nav aria-label="Sidebar inbox pages" className="flex justify-between p-3"><Button variant="ghost" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}><ChevronLeft className="size-4" />Previous</Button><Button variant="ghost" size="sm" disabled={!inbox.data?.has_more} onClick={() => setOffset(offset + 50)}>Next</Button></nav>}
      </div>}
    </div>
  </aside>;
}

function CompactMessage({ question: q, onOpenTask, onResult }: { question: FeedbackQuestion; onOpenTask: () => void; onResult: (message: string, exited?: FeedbackQuestion) => void }) {
  const action = useFeedbackAction();
  const navigate = useNavigate();
  const completion = q.kind === "completion";
  async function submit(kind: "answer" | "dismiss", option_id?: string, openTask = false) {
    try {
      // Await the mutation directly: polling can unmount this card before it finishes.
      const result = await action.mutateAsync({ question: q, action: kind, option_id });
      onResult(feedbackOutcomeMessage(result), option_id === "exit" && result.outcome !== "stale_revision" && result.question.kind === "completion" && result.question.reason === "exit" ? result.question : undefined);
      if (openTask && result.outcome !== "stale_revision" && result.question.status !== "pending") {
        onOpenTask();
        navigate(feedbackTaskURL(q));
      }
    } catch {
      // The mutation exposes its error to the inline alert.
    }
  }
  return <article className="space-y-3 border-b border-border p-4" aria-label={`Message from ${q.task_id === 0 ? "Scratchpad" : q.task_title}`}>
    <div className="flex items-start justify-between gap-2">
      <p className="min-w-0 break-words text-xs text-muted-foreground">{q.task_id === 0 ? "Scratchpad" : `${q.project_name} / ${q.task_title}`}</p>
      <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium">{completion ? "Completed" : "Needs reply"}</span>
    </div>
    <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{q.abstract || q.summary}</p>
    {!completion && q.question !== q.abstract && <p className="whitespace-pre-wrap break-words text-sm font-medium">{q.question}</p>}
    {!completion && q.options.length > 0 && <div className="flex flex-col gap-2" role="group" aria-label="Choose an answer to send immediately">
      {q.options.map((option) => <Button key={option.id} variant="outline" size="sm" className="h-auto min-h-9 max-w-full justify-start whitespace-normal break-words text-left" disabled={action.isPending} onClick={() => submit("answer", option.id)}>{option.label}</Button>)}
    </div>}
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="outline" size="sm" disabled={action.isPending} title="Archive this message and open its agent terminal" onClick={() => submit("dismiss", undefined, true)}>{q.task_id === 0 ? "Open Scratchpad" : "Open task"}<ArrowUpRight className="size-3" /></Button>
      {completion && <Button variant="outline" size="sm" disabled={action.isPending} onClick={() => submit("answer", "exit")}>OK, exit session</Button>}
      <Button variant="ghost" size="sm" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>
    </div>
    {!completion && <Link to={`/inbox?message=${encodeURIComponent(q.id)}#message-${q.id}`} className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">Full context / custom reply<ArrowUpRight className="size-3" /></Link>}
    {action.isPending && <p role="status" className="text-xs text-muted-foreground">Updating message…</p>}
    {action.isError && <p role="alert" className="break-words text-xs text-destructive">{action.error.message} Try again.</p>}
  </article>;
}

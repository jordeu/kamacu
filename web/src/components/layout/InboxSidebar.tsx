import { useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, ChevronLeft, ChevronRight, Inbox } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { feedbackOutcomeMessage, useFeedbackAction, useInbox, type FeedbackQuestion } from "@/api/inbox";
import { AgentExitDialog } from "@/components/task/AgentExitDialog";

export function InboxSidebar() {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("kamacu:inbox-collapsed") !== "0");
  const [offset, setOffset] = useState(0);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [notice, setNotice] = useState("");
  const [exited, setExited] = useState<FeedbackQuestion | null>(null);
  const inbox = useInbox("pending", offset);
  function toggle() {
    setCollapsed((current) => {
      localStorage.setItem("kamacu:inbox-collapsed", current ? "0" : "1");
      return !current;
    });
  }
  const count = inbox.data?.pending_count;
  return <aside aria-label="Inbox sidebar" className={`relative shrink-0 border-l border-border bg-sidebar pb-9 ${collapsed ? "w-12" : "w-12 sm:w-80"}`}>
    {exited && <AgentExitDialog taskId={exited.task_id} projectId={exited.project_id} onClose={() => setExited(null)} />}
    <div className={collapsed ? "flex h-full flex-col items-center gap-3 py-3" : "absolute inset-y-0 right-0 z-20 flex w-[min(20rem,calc(100vw-3rem))] flex-col border-l border-border bg-sidebar pb-9 sm:static sm:h-full sm:w-full sm:border-0 sm:pb-0"}>
      <div className={`flex items-center gap-2 ${collapsed ? "flex-col" : "border-b border-border p-3"}`}>
        <Button size="icon-sm" variant="ghost" onClick={toggle} aria-expanded={!collapsed} aria-controls="inbox-sidebar-messages" aria-label={`${collapsed ? "Expand" : "Collapse"} inbox sidebar`} title={collapsed ? "Expand inbox" : "Collapse inbox"}>
          {collapsed ? <Inbox className="size-4" /> : <ChevronRight className="size-4" />}
        </Button>
        {!collapsed && <Link to="/inbox" className="min-w-0 flex-1 text-sm font-semibold hover:underline">Inbox</Link>}
        <span aria-label={count === undefined ? "Loading pending messages" : `${count} pending messages`} className="text-xs font-medium tabular-nums text-muted-foreground">{count === undefined ? "…" : count}</span>
        {!collapsed && <Button asChild size="icon-sm" variant="ghost"><Link to="/inbox" aria-label="Open full inbox"><ArrowUpRight className="size-4" /></Link></Button>}
      </div>
      {!collapsed && <div id="inbox-sidebar-messages" className="min-h-0 flex-1 overflow-y-auto">
        {notice && <p role="status" className="border-b border-border p-3 text-xs">{notice}</p>}
        {inbox.isError && <div role="alert" className="p-3 text-sm">Couldn’t refresh inbox.<Button variant="ghost" size="sm" onClick={() => void inbox.refetch()}>Retry</Button></div>}
        {inbox.isPending && <p className="p-4 text-sm text-muted-foreground">Loading messages…</p>}
        {inbox.data?.items.length === 0 && <p className="p-4 text-sm text-muted-foreground">{offset ? "No more pending messages." : "No messages waiting."}</p>}
        {inbox.data?.items.map((q) => <CompactMessage key={`${q.id}:${q.revision}`} question={q} answer={drafts[`${q.id}:${q.revision}`] ?? ""} setAnswer={(answer) => setDrafts((current) => ({ ...current, [`${q.id}:${q.revision}`]: answer }))} onResult={(message, exit) => { setNotice(message); if (exit && exit.task_id > 0) setExited(exit); }} />)}
        {(offset > 0 || inbox.data?.has_more) && <nav aria-label="Sidebar inbox pages" className="flex justify-between p-3"><Button variant="ghost" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}><ChevronLeft className="size-4" />Previous</Button><Button variant="ghost" size="sm" disabled={!inbox.data?.has_more} onClick={() => setOffset(offset + 50)}>Next</Button></nav>}
      </div>}
    </div>
  </aside>;
}

function CompactMessage({ question: q, answer, setAnswer, onResult }: { question: FeedbackQuestion; answer: string; setAnswer: (answer: string) => void; onResult: (message: string, exited?: FeedbackQuestion) => void }) {
  const action = useFeedbackAction();
  const completion = q.kind === "completion";
  function submit(kind: "answer" | "dismiss", option_id?: string) {
    action.mutate({ question: q, action: kind, option_id, answer: kind === "answer" && !option_id ? answer : undefined }, {
      onSuccess: (result) => {
        if (result.question.status !== "pending") setAnswer("");
        onResult(feedbackOutcomeMessage(result), result.outcome !== "stale_revision" && result.question.kind === "completion" && result.question.reason === "exit" ? result.question : undefined);
      },
    });
  }
  return <article className="space-y-3 border-b border-border p-3" aria-label={`Message from ${q.task_id === 0 ? "Scratchpad" : q.task_title}`}>
    <p className="truncate text-xs text-muted-foreground" title={q.task_title}>{q.task_id === 0 ? "Scratchpad" : q.task_title}</p>
    <Link to={`/inbox?message=${encodeURIComponent(q.id)}#message-${q.id}`} className="block break-words text-sm font-medium leading-relaxed hover:underline">{q.abstract || Array.from(q.summary).slice(0, 180).join("")}</Link>
    <Link to={`/inbox?message=${encodeURIComponent(q.id)}#message-${q.id}`} className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">Open full message<ArrowUpRight className="size-3" /></Link>
    <div className="flex flex-wrap gap-2" role="group" aria-label="Choose an answer to send immediately">
      {q.options.map((option) => <Button key={option.id} variant="outline" size="sm" className="h-auto min-h-8 max-w-full whitespace-normal break-words text-left" disabled={action.isPending} onClick={() => submit("answer", option.id)}>{option.label}</Button>)}
    </div>
    <form className="space-y-2" onSubmit={(event) => { event.preventDefault(); if (answer.trim()) submit("answer"); }}>
      <label htmlFor={`sidebar-answer-${q.id}`} className="text-xs text-muted-foreground">{completion ? "Send the next prompt" : "Write a reply"}</label>
      <Textarea id={`sidebar-answer-${q.id}`} value={answer} onChange={(event) => setAnswer(event.target.value)} rows={2} maxLength={16000} disabled={action.isPending} className="min-h-16 text-sm" />
      <div className="flex justify-between gap-2"><Button type="button" variant="ghost" size="sm" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button><Button type="submit" size="sm" disabled={action.isPending || !answer.trim()}>{action.isPending ? "Sending…" : completion ? "Send prompt" : "Send reply"}</Button></div>
    </form>
    {action.isError && <p role="alert" className="break-words text-xs text-destructive">{action.error.message} Your reply has been kept. Try again.</p>}
  </article>;
}

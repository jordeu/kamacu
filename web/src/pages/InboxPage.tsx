import { useState } from "react";
import { useSearchParams } from "react-router";
import { Inbox, Check, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useInbox, useInboxMessage, type FeedbackQuestion } from "@/api/inbox";

import { InboxMessage } from "@/components/inbox/InboxMessage";
import { AgentExitDialog } from "@/components/task/AgentExitDialog";

export default function InboxPage() {
  const [params, setParams] = useSearchParams();
  const messageID = params.get("message");
  const selected = useInboxMessage(messageID);
  const [view, setView] = useState<"pending" | "archived">("pending");
  const [offset, setOffset] = useState(0);
  const [exited, setExited] = useState<FeedbackQuestion | null>(null);
  const [notice, setNotice] = useState("");
  const inbox = useInbox(view, offset);
  const activeView = messageID && selected.data ? (selected.data.status === "pending" ? "pending" : "archived") : view;
  return <main className="h-full overflow-y-auto p-4 sm:p-6">
    {exited && <AgentExitDialog taskId={exited.task_id} projectId={exited.project_id} onClose={() => setExited(null)} />}
    <div className="w-full">
      <h1 className="text-2xl font-semibold tracking-tight">Inbox</h1>
      <p className="mt-1 text-sm text-muted-foreground">Questions and completed work, across all your tasks.</p>
      <div className="mt-6 flex gap-2 border-b border-border pb-3" role="group" aria-label="Inbox view">
        {(["pending", "archived"] as const).map((tab) => <Button key={tab} variant={activeView === tab ? "secondary" : "ghost"} aria-pressed={activeView === tab} onClick={() => { setParams({}); setView(tab); setOffset(0); setNotice(""); }}>
          {tab === "pending" ? <Clock className="size-4" aria-hidden="true" /> : <Check className="size-4" aria-hidden="true" />}
          {tab === "pending" ? `Pending${inbox.data ? ` (${inbox.data.pending_count})` : ""}` : "Archived"}
        </Button>)}
      </div>
      {notice && <p role="status" className="my-4 flex items-center gap-2 text-sm"><Check className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />{notice}</p>}
      {messageID && <>
        {selected.isPending && <Skeleton aria-label="Loading message" className="mt-4 h-24 w-full" />}
        {selected.isError && <div role="alert" className="my-4 flex items-center gap-3 text-sm"><p>Couldn’t load this message.</p><Button variant="outline" onClick={() => void selected.refetch()}>Retry loading message</Button></div>}
        {selected.data && <InboxMessage key={`${selected.data.id}:${selected.data.revision}`} question={selected.data} expanded onResult={(message, session) => { setNotice(message); if (session && session.task_id > 0) setExited(session); }} />}
      </>}
      {!messageID && inbox.isError && <div role="alert" className="mb-5 flex flex-wrap items-center gap-3 text-sm">
        <p>Couldn’t refresh your inbox.</p><Button variant="outline" onClick={() => void inbox.refetch()}>Retry loading</Button>
      </div>}
      {!messageID && (inbox.isPending ? <div aria-label="Loading inbox" className="space-y-4"><Skeleton className="h-6 w-2/3" /><Skeleton className="h-24 w-full" /><Skeleton className="h-10 w-1/2" /></div> : inbox.data && <>
        {inbox.data.items.length === 0 ? <div className="py-16 text-center">
          <Inbox className="mx-auto mb-4 size-6 text-muted-foreground" aria-hidden="true" />
          <h2 className="text-base font-medium">{view === "pending" ? "No messages waiting" : "No archived questions"}</h2>
          <p className="mt-2 text-sm text-muted-foreground">{view === "pending" ? "Agent questions and completion summaries will appear here." : "Answered, dismissed, and unanswered questions will appear here."}</p>
        </div> : inbox.data.items.map((q) => <InboxMessage key={`${q.id}:${q.revision}`} question={q} onResult={(message, session) => { setNotice(message); if (session && session.task_id > 0) setExited(session); }} />)}
        {(offset > 0 || inbox.data.has_more) && <nav aria-label="Inbox pages" className="mt-6 flex justify-between gap-3">
          <Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</Button>
          <Button variant="outline" disabled={!inbox.data.has_more} onClick={() => setOffset(offset + 50)}>Next</Button>
        </nav>}
      </>)}
    </div>
  </main>;
}

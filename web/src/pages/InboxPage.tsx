import { useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, Inbox, ChevronRight, Check, Clock } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, useInbox, type FeedbackQuestion } from "@/api/inbox";

import { AgentExitDialog } from "@/components/task/AgentExitDialog";

function Context({ text }: { text: string }) {
  return <div className="prose prose-invert prose-sm max-w-none break-words prose-headings:text-sm prose-a:text-blue-400 prose-pre:overflow-x-auto">
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: () => null }}>{text}</ReactMarkdown>
  </div>;
}
function Question({ question: q, onResult }: { question: FeedbackQuestion; onResult: (message: string, exited?: FeedbackQuestion) => void }) {
  const [answer, setAnswer] = useState("");
  const action = useFeedbackAction();
  const completion = q.kind === "completion";
  const pending = q.status === "pending";
  function submit(kind: "answer" | "dismiss", option_id?: string) {
    action.mutate({ question: q, action: kind, option_id, answer: kind === "answer" && !option_id ? answer : undefined }, {
      onSuccess: (result) => {
        onResult(feedbackOutcomeMessage(result), result.outcome !== "stale_revision" && result.question.kind === "completion" && result.question.reason === "exit" ? result.question : undefined);
      },
    });
  }
  const status = q.status === "unanswered" ? "Unanswered" : q.status === "dismissed" ? "Dismissed" : "Answered";
  const timestamp = pending ? q.created_at : q.updated_at;
  const date = new Date(timestamp);
  const shortDate = date.toDateString() === new Date().toDateString()
    ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString([], { month: "short", day: "numeric" });
  return <details className="group/message border-b border-border last:border-0 open:bg-muted/20">
    <summary aria-label={`Open question: ${q.question}`} className="flex cursor-pointer list-none items-start gap-3 px-3 py-4 outline-none transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring sm:items-center sm:px-5 [&::-webkit-details-marker]:hidden">
      <ChevronRight aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform duration-150 group-open/message:rotate-90 motion-reduce:transition-none sm:mt-0" />
      <div className="grid min-w-0 flex-1 gap-1 sm:grid-cols-[minmax(8rem,0.8fr)_minmax(0,3fr)] sm:items-center sm:gap-5">
        <span className="truncate text-xs text-muted-foreground sm:text-sm" title={q.task_title}>{q.task_id === 0 ? "Scratchpad" : q.task_title}</span>
        <span className="min-w-0">
          <span className={`block truncate text-sm ${pending ? "font-semibold" : "font-medium"}`}>{q.question}</span>
          <span className="mt-1 block truncate text-xs text-muted-foreground">{q.status === "answered" ? q.answer : q.summary}</span>
        </span>
      </div>
      <span className="flex shrink-0 flex-col items-end gap-1.5 text-xs text-muted-foreground">
        <time dateTime={date.toISOString()} title={date.toLocaleString()}>{shortDate}</time>
        {pending ? <span className="flex items-center gap-1 text-amber-400"><span className="size-1.5 rounded-full bg-current" aria-hidden="true" /><span className="hidden sm:inline">{completion ? "Completed" : "Needs reply"}</span></span> : <span>{status}</span>}
      </span>
    </summary>
    <article aria-labelledby={`question-${q.id}`} className="border-t border-border px-4 py-6 sm:px-12">
    <div className="mx-auto max-w-3xl">
    <div className="flex flex-col items-start justify-between gap-3 sm:flex-row">
      <div className="min-w-0 flex-1">
        <p className="break-words text-sm text-muted-foreground">{q.task_id === 0 ? "Scratchpad" : `${q.project_name} / ${q.task_title}`}</p>
        <h2 id={`question-${q.id}`} className="mt-2 whitespace-pre-wrap break-words text-base font-medium">{q.question}</h2>
      </div>
      <Button asChild variant="ghost" size="sm" className="shrink-0">
        <Link to={feedbackTaskURL(q)} aria-label={q.task_id === 0 ? "Open Scratchpad" : `Open task: ${q.task_title}`}>{q.task_id === 0 ? "Open Scratchpad" : "Open task"} <ArrowUpRight className="size-4" /></Link>
      </Button>
    </div>
    {pending ? <div className="mt-6 space-y-6">
      <div><h3 className="mb-2 text-sm font-medium text-muted-foreground">Task summary</h3><p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{q.summary}</p></div>
      <div><h3 className="mb-2 text-sm font-medium text-muted-foreground">{completion ? "What was done" : "Decision context"}</h3><Context text={q.context} /></div>
    </div> : <>
      <p className="mt-3 text-sm text-muted-foreground">
        {status} · <time dateTime={new Date(q.updated_at).toISOString()}>{new Date(q.updated_at).toLocaleString()}</time>
        {q.status === "answered" && (completion && q.reason === "exit" ? " · Session exit requested" : q.answer_source === "session" ? " · Answered in terminal" : q.acknowledged_at ? " · Received by agent" : " · Awaiting agent")}
        {q.reason === "heartbeat_expired" && " · Agent stopped polling"}
        {q.reason === "session_stopped" && " · Agent session stopped"}
      </p>
      {q.status === "answered" && <p className="mt-2 whitespace-pre-wrap break-words text-sm">{q.answer}</p>}
      <div className="mt-6 space-y-4 border-t border-border pt-5 text-sm">
        <h3 className="font-medium text-muted-foreground">Task summary and context</h3>
        <p className="whitespace-pre-wrap break-words leading-relaxed">{q.summary}</p>
        <Context text={q.context} />
      </div>
    </>}
    {pending && <div className="mt-6 space-y-4 border-t border-border pt-5">
      <p className="text-sm font-medium">{completion ? "What next?" : "Your answer"} {q.options.length > 0 && <span className="ml-1 font-normal text-muted-foreground">Choose an option to send it immediately.</span>}</p>
      {q.options.length > 0 && <div className="flex flex-wrap gap-2" role="group" aria-label="Choose an answer to send immediately">
        {q.options.map((option) => <Button key={option.id} variant="outline" className="h-auto min-h-9 max-w-full whitespace-normal break-words text-left" disabled={action.isPending} onClick={() => submit("answer", option.id)}>{option.label}</Button>)}
      </div>}
      <form onSubmit={(event) => { event.preventDefault(); if (answer.trim()) submit("answer"); }}>
        <label htmlFor={`answer-${q.id}`} className="mb-2 block text-sm text-muted-foreground">{completion ? "Send the next prompt" : "Write your own answer"}</label>
        <Textarea id={`answer-${q.id}`} value={answer} onChange={(event) => setAnswer(event.target.value)} maxLength={16000} rows={3} disabled={action.isPending} className="resize-y" />
        <div className="mt-3 flex items-center justify-between gap-3">
          <Button type="button" variant="ghost" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>
          <Button type="submit" disabled={action.isPending || !answer.trim()}>{action.isPending ? "Sending…" : completion ? "Send next prompt" : "Send answer"}</Button>
        </div>
      </form>
      {action.isError && <p role="alert" className="text-sm text-destructive">{action.error.message} Your answer has been kept. Try again.</p>}
    </div>}
    </div>
  </article>
  </details>;
}
export default function InboxPage() {
  const [view, setView] = useState<"pending" | "archived">("pending");
  const [offset, setOffset] = useState(0);
  const [exited, setExited] = useState<FeedbackQuestion | null>(null);
  const [notice, setNotice] = useState("");
  const inbox = useInbox(view, offset);
  return <main className="h-full overflow-y-auto p-4 sm:p-6">
    {exited && <AgentExitDialog taskId={exited.task_id} projectId={exited.project_id} onClose={() => setExited(null)} />}
    <div className="mx-auto max-w-5xl">
      <h1 className="text-2xl font-semibold tracking-tight">Inbox</h1>
      <p className="mt-1 text-sm text-muted-foreground">Questions and completed work, across all your tasks.</p>
      <div className="mt-6 flex gap-2 border-b border-border pb-3" role="group" aria-label="Inbox view">
        {(["pending", "archived"] as const).map((tab) => <Button key={tab} variant={view === tab ? "secondary" : "ghost"} aria-pressed={view === tab} onClick={() => { setView(tab); setOffset(0); setNotice(""); }}>
          {tab === "pending" ? <Clock className="size-4" aria-hidden="true" /> : <Check className="size-4" aria-hidden="true" />}
          {tab === "pending" ? `Pending${inbox.data ? ` (${inbox.data.pending_count})` : ""}` : "Archived"}
        </Button>)}
      </div>
      {notice && <p role="status" className="my-4 flex items-center gap-2 text-sm"><Check className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />{notice}</p>}
      {inbox.isError && <div role="alert" className="mb-5 flex flex-wrap items-center gap-3 text-sm">
        <p>Couldn’t refresh your inbox.</p><Button variant="outline" onClick={() => void inbox.refetch()}>Retry loading</Button>
      </div>}
      {inbox.isPending ? <div aria-label="Loading inbox" className="space-y-4"><Skeleton className="h-6 w-2/3" /><Skeleton className="h-24 w-full" /><Skeleton className="h-10 w-1/2" /></div> : inbox.data && <>
        {inbox.data.items.length === 0 ? <div className="py-16 text-center">
          <Inbox className="mx-auto mb-4 size-6 text-muted-foreground" aria-hidden="true" />
          <h2 className="text-base font-medium">{view === "pending" ? "No messages waiting" : "No archived questions"}</h2>
          <p className="mt-2 text-sm text-muted-foreground">{view === "pending" ? "Agent questions and completion summaries will appear here." : "Answered, dismissed, and unanswered questions will appear here."}</p>
        </div> : inbox.data.items.map((q) => <Question key={`${q.id}:${q.revision}`} question={q} onResult={(message, session) => { setNotice(message); if (session && session.task_id > 0) setExited(session); }} />)}
        {(offset > 0 || inbox.data.has_more) && <nav aria-label="Inbox pages" className="mt-6 flex justify-between gap-3">
          <Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</Button>
          <Button variant="outline" disabled={!inbox.data.has_more} onClick={() => setOffset(offset + 50)}>Next</Button>
        </nav>}
      </>}
    </div>
  </main>;
}

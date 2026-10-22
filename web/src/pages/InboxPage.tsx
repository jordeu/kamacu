import { useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, Inbox } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, useInbox, type FeedbackQuestion } from "@/api/inbox";

function Context({ text }: { text: string }) {
  return <div className="prose prose-invert prose-sm max-w-none break-words prose-headings:text-sm prose-a:text-blue-400 prose-pre:overflow-x-auto">
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: () => null }}>{text}</ReactMarkdown>
  </div>;
}
function Question({ question: q, onResult }: { question: FeedbackQuestion; onResult: (message: string) => void }) {
  const [answer, setAnswer] = useState("");
  const action = useFeedbackAction();
  const pending = q.status === "pending";
  function submit(kind: "answer" | "dismiss", option_id?: string) {
    action.mutate({ question: q, action: kind, option_id, answer: kind === "answer" && !option_id ? answer : undefined }, {
      onSuccess: (result) => onResult(feedbackOutcomeMessage(result)),
    });
  }
  const status = q.status === "unanswered" ? "Unanswered" : q.status === "dismissed" ? "Dismissed" : "Answered";
  return <article aria-labelledby={`question-${q.id}`} className="border-b border-border py-6 first:pt-0 last:border-0">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0 flex-1">
        <p className="break-words text-sm text-muted-foreground">{q.task_id === 0 ? "Scratchpad" : `${q.project_name} / ${q.task_title}`}</p>
        <h2 id={`question-${q.id}`} className="mt-2 whitespace-pre-wrap break-words text-base font-medium">{q.question}</h2>
      </div>
      <Button asChild variant="ghost" size="sm" className="shrink-0">
        <Link to={feedbackTaskURL(q)} aria-label={q.task_id === 0 ? "Open Scratchpad" : `Open task: ${q.task_title}`}>{q.task_id === 0 ? "Open Scratchpad" : "Open task"} <ArrowUpRight className="size-4" /></Link>
      </Button>
    </div>
    {pending ? <div className="mt-4 space-y-3">
      <p className="whitespace-pre-wrap break-words text-sm">{q.summary}</p>
      <Context text={q.context} />
    </div> : <>
      <p className="mt-3 text-sm text-muted-foreground">
        {status} · <time dateTime={new Date(q.updated_at).toISOString()}>{new Date(q.updated_at).toLocaleString()}</time>
        {q.status === "answered" && (q.answer_source === "session" ? " · Answered in terminal" : q.acknowledged_at ? " · Received by agent" : " · Awaiting agent")}
        {q.reason === "heartbeat_expired" && " · Agent stopped polling"}
        {q.reason === "session_stopped" && " · Agent session stopped"}
      </p>
      {q.status === "answered" && <p className="mt-2 whitespace-pre-wrap break-words text-sm">{q.answer}</p>}
      <details className="mt-3 text-sm">
        <summary className="w-fit cursor-pointer rounded text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring">Show task summary and context</summary>
        <p className="my-3 whitespace-pre-wrap break-words">{q.summary}</p>
        <Context text={q.context} />
      </details>
    </>}
    {pending && <div className="mt-5 space-y-4">
      {q.options.length > 0 && <div className="flex flex-wrap gap-2" role="group" aria-label="Choose an answer to send immediately">
        {q.options.map((option) => <Button key={option.id} variant="outline" className="h-auto min-h-9 max-w-full whitespace-normal break-words text-left" disabled={action.isPending} onClick={() => submit("answer", option.id)}>{option.label}</Button>)}
      </div>}
      <form onSubmit={(event) => { event.preventDefault(); if (answer.trim()) submit("answer"); }}>
        <label htmlFor={`answer-${q.id}`} className="mb-2 block text-sm text-muted-foreground">Write your own answer</label>
        <Textarea id={`answer-${q.id}`} value={answer} onChange={(event) => setAnswer(event.target.value)} maxLength={16000} rows={3} disabled={action.isPending} className="resize-y" />
        <div className="mt-3 flex items-center justify-between gap-3">
          <Button type="button" variant="ghost" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>
          <Button type="submit" disabled={action.isPending || !answer.trim()}>{action.isPending ? "Sending…" : "Send answer"}</Button>
        </div>
      </form>
      {action.isError && <p role="alert" className="text-sm text-destructive">{action.error.message} Your answer has been kept. Try again.</p>}
    </div>}
  </article>;
}
export default function InboxPage() {
  const [view, setView] = useState<"pending" | "archived">("pending");
  const [offset, setOffset] = useState(0);
  const [notice, setNotice] = useState("");
  const inbox = useInbox(view, offset);
  return <main className="h-full overflow-y-auto p-4 sm:p-6">
    <div className="mx-auto max-w-3xl">
      <h1 className="text-lg font-medium">Inbox</h1>
      <p className="mt-1 text-sm text-muted-foreground">Questions from your agents, across all workspaces.</p>
      <div className="my-6 flex gap-2" role="group" aria-label="Inbox view">
        {(["pending", "archived"] as const).map((tab) => <Button key={tab} variant={view === tab ? "secondary" : "ghost"} aria-pressed={view === tab} onClick={() => { setView(tab); setOffset(0); setNotice(""); }}>
          {tab === "pending" ? `Pending${inbox.data ? ` (${inbox.data.pending_count})` : ""}` : "Archived"}
        </Button>)}
      </div>
      {notice && <p role="status" className="mb-5 rounded-md border border-border p-3 text-sm">{notice}</p>}
      {inbox.isError && <div role="alert" className="mb-5 flex flex-wrap items-center gap-3 text-sm">
        <p>Couldn’t refresh your inbox.</p><Button variant="outline" onClick={() => void inbox.refetch()}>Retry loading</Button>
      </div>}
      {inbox.isPending ? <div aria-label="Loading inbox" className="space-y-4"><Skeleton className="h-6 w-2/3" /><Skeleton className="h-24 w-full" /><Skeleton className="h-10 w-1/2" /></div> : inbox.data && <>
        {inbox.data.items.length === 0 ? <div className="py-16 text-center">
          <Inbox className="mx-auto mb-4 size-6 text-muted-foreground" aria-hidden="true" />
          <h2 className="text-base font-medium">{view === "pending" ? "No questions waiting" : "No archived questions"}</h2>
          <p className="mt-2 text-sm text-muted-foreground">{view === "pending" ? "When an agent needs your feedback, its question will appear here." : "Answered, dismissed, and unanswered questions will appear here."}</p>
        </div> : inbox.data.items.map((q) => <Question key={`${q.id}:${q.revision}`} question={q} onResult={setNotice} />)}
        {(offset > 0 || inbox.data.has_more) && <nav aria-label="Inbox pages" className="mt-6 flex justify-between gap-3">
          <Button variant="outline" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>Previous</Button>
          <Button variant="outline" disabled={!inbox.data.has_more} onClick={() => setOffset(offset + 50)}>Next</Button>
        </nav>}
      </>}
    </div>
  </main>;
}

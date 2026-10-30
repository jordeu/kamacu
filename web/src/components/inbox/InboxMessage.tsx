import { useState } from "react";
import { useNavigate } from "react-router";
import { ArrowUpRight, ChevronRight } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { completionActions, completionClosesSession, feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, type FeedbackQuestion } from "@/api/inbox";

function Context({ text }: { text: string }) {
  return <div className="prose prose-invert prose-sm max-w-none break-words prose-headings:text-sm prose-a:text-blue-400 prose-pre:overflow-x-auto">
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: () => null }}>{text}</ReactMarkdown>
  </div>;
}
export function InboxMessage({ question: q, onResult, expanded, dialog = false, onNavigate }: { question: FeedbackQuestion; onNavigate?: () => void; expanded?: boolean; dialog?: boolean; onResult: (message: string, keepOpen?: boolean) => void }) {
  const [answer, setAnswer] = useState("");
  const action = useFeedbackAction();
  const navigate = useNavigate();
  const completion = q.kind === "completion";
  const choices = completion ? completionActions(q) : q.options;
  const pending = q.status === "pending";
  function navigateToTask() {
    onNavigate?.();
    navigate(feedbackTaskURL(q));
  }
  async function submit(kind: "answer" | "dismiss", option_id?: string, openTask = false) {
    try {
      // Await the mutation directly: polling can unmount this card before it finishes.
      const result = await action.mutateAsync({ question: q, action: kind, option_id, answer: kind === "answer" && !option_id ? answer : undefined });
      if (openTask && result.outcome !== "stale_revision" && result.question.status !== "pending") navigateToTask();
      onResult(feedbackOutcomeMessage(result), result.outcome === "stale_revision");
    } catch {
      // The mutation exposes its error to the inline alert.
    }
  }
  const status = q.status === "unanswered" ? "Unanswered" : q.status === "dismissed" ? "Dismissed" : "Answered";
  const timestamp = pending ? q.created_at : q.updated_at;
  const date = new Date(timestamp);
  const shortDate = date.toDateString() === new Date().toDateString()
    ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString([], { month: "short", day: "numeric" });
  const content = <article aria-labelledby={`question-${q.id}`} className="border-t border-border px-4 py-6 sm:px-12">
    <div className="min-w-0">
    <div className="flex flex-col items-start justify-between gap-3 sm:flex-row">
      <div className="min-w-0 flex-1">
        <p className="break-words text-sm text-muted-foreground">{q.task_id === 0 ? "Scratchpad" : `${q.project_name} / ${q.task_title}`}</p>
        <h2 id={`question-${q.id}`} className="mt-2 whitespace-pre-wrap break-words text-base font-medium">{q.question}</h2>
      </div>
      <Button variant="ghost" size="sm" disabled={action.isPending} onClick={() => pending && (q.origin!=="observer" || completion) ? submit("dismiss", undefined, true) : navigateToTask()} aria-label={q.task_id === 0 ? "Open Scratchpad" : `Open task: ${q.task_title}`}>{q.task_id === 0 ? "Open Scratchpad" : "Open task"} <ArrowUpRight className="size-4" /></Button>
    </div>
    {pending ? <div className="mt-6 space-y-6">
      <div><h3 className="mb-2 text-sm font-medium text-muted-foreground">Task summary</h3><p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{q.summary}</p></div>
      <div><h3 className="mb-2 text-sm font-medium text-muted-foreground">{completion ? "What was done" : "Decision context"}</h3><Context text={q.context} /></div>
    </div> : <>
      <p className="mt-3 text-sm text-muted-foreground">
        {status} · <time dateTime={new Date(q.updated_at).toISOString()}>{new Date(q.updated_at).toLocaleString()}</time>
        {q.origin==="observer" && q.status==="answered" && (completionClosesSession(q) ? " · Session exit requested" : ` · ${q.delivery_status === "uncertain" ? "Delivery uncertain — review the terminal" : q.delivery_status || "Delivery pending"}`)}
 {q.origin!=="observer" && q.status === "answered" && (completion && q.reason === "exit" ? " · Session exit requested" : q.answer_source === "session" ? " · Answered in terminal" : q.acknowledged_at ? " · Received by agent" : " · Awaiting agent")}
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
      <p className="text-sm font-medium">{completion ? "What next?" : "Your answer"} {choices.length > 0 && (completion || (q.delivery_mode !== "terminal" && !q.hidden)) && <span className="ml-1 font-normal text-muted-foreground">{completion ? "Choose what happens next." : "Choose an option to send it immediately."}</span>}</p>
      {choices.length > 0 && (completion || (q.delivery_mode !== "terminal" && !q.hidden)) && <div className="flex flex-wrap gap-2" role="group" aria-label={completion ? "Completion actions" : "Choose an answer to send immediately"}>
        {choices.map((option) => <Button key={option.id} variant="outline" className="h-auto min-h-9 max-w-full whitespace-normal break-words text-left" disabled={action.isPending} onClick={() => submit("answer", option.id)}>{option.label}</Button>)}
      </div>}
      {!completion && q.delivery_mode!=="terminal" && !q.hidden && <form onSubmit={(event) => { event.preventDefault(); if (answer.trim()) submit("answer"); }}>
        <label htmlFor={`answer-${q.id}`} className="mb-2 block text-sm text-muted-foreground">Write your own answer</label>
        <Textarea id={`answer-${q.id}`} value={answer} onChange={(event) => setAnswer(event.target.value)} maxLength={16000} rows={3} disabled={action.isPending} className="resize-y" />
        <div className="mt-3 flex items-center justify-between gap-3">
          <Button type="button" variant="ghost" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>
          <Button type="submit" disabled={action.isPending || !answer.trim()}>{action.isPending ? "Sending…" : "Send answer"}</Button>
        </div>
      </form>}
      {!completion && (q.delivery_mode==="terminal" || q.hidden) && <p className="text-sm">Open the terminal to respond.</p>}
 {(completion || q.delivery_mode==="terminal") && <Button variant="ghost" disabled={action.isPending} onClick={() => submit("dismiss")}>Dismiss</Button>}
      {action.isError && <p role="alert" className="text-sm text-destructive">{action.error.message} {completion ? "Review the task before retrying." : "Your answer has been kept. Check the terminal before retrying."}</p>}
    </div>}
    </div>
  </article>;
  if (dialog) return content;
  return <details open={expanded || undefined} id={`message-${q.id}`} className="group/message border-b border-border last:border-0 open:bg-muted/20">
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
    {content}
  </details>;
}

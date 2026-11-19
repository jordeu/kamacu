import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router";
import { Dialog as DialogPrimitive } from "radix-ui";
import { CheckCheck, ChevronLeft, Inbox, X } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { setTerminalViewObscured } from "@/presence/activeSessionStore";
import { CatchUpContext, CatchUpOpenContext } from "@/lib/useCatchUp";
import { useCatchUpQueue, useCatchUpContext, catchUpSourceText, catchUpHeadline, catchUpReadingText, catchUpActionSucceeded, latestCatchUpMessage } from "@/api/catchUp";
import { completionActions, feedbackOutcomeMessage, feedbackTaskURL, useFeedbackAction, useInboxMessage, type FeedbackQuestion } from "@/api/inbox";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogPortal, DialogTitle } from "@/components/ui/dialog";
import { ActionRunDetails } from "./ActionRunDetails";
import { RuleImprovement } from "./RuleImprovement";
import { InboxText } from "./InboxText";
import { CatchUpHistory, CatchUpPRs, CatchUpReferences, CatchUpTasks, contextCard, contextProse } from "./CatchUpContext";

export function CatchUpProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    setTerminalViewObscured(open);
    return () => setTerminalViewObscured(false);
  }, [open]);
  function launch() {
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setOpen(true);
  }
  return <CatchUpContext.Provider value={launch}><CatchUpOpenContext.Provider value={open}><Dialog open={open} onOpenChange={setOpen}>
    {children}
    {open && <DialogPortal><DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-background" />
      <DialogPrimitive.Content aria-describedby={undefined} className="fixed inset-0 z-50 flex h-dvh min-h-0 w-full flex-col overflow-hidden bg-background text-foreground outline-none" onCloseAutoFocus={event => { event.preventDefault(); if (opener.current?.isConnected) opener.current.focus(); }}>
        <CatchUpSession onClose={() => setOpen(false)} />
      </DialogPrimitive.Content>
    </DialogPortal>}
  </Dialog></CatchUpOpenContext.Provider></CatchUpContext.Provider>;
}
function CatchUpHeader({ onClose, progress }: { onClose: () => void; progress?: string }) {
  return <header className="flex shrink-0 items-center justify-between gap-3 border-b border-border bg-card/40 px-4 py-4 sm:px-7">
    <div className="flex items-center gap-3"><Inbox className="size-5 text-orange-300" aria-hidden="true" /><DialogTitle>Catch up</DialogTitle>{progress && <span className="text-xs tabular-nums text-muted-foreground">{progress}</span>}</div>
    <Button variant="ghost" size="sm" onClick={onClose}>Close <X className="size-4" /></Button>
  </header>;
}
function CatchUpSession({ onClose }: { onClose: () => void }) {
  const queue = useCatchUpQueue();
  if (!queue.data) return <><CatchUpHeader onClose={onClose} /><div className="p-8">{queue.isError ? <div role="alert">Couldn’t load your catch-up pass. <Button variant="outline" onClick={() => void queue.refetch()}>Retry inbox</Button></div> : <p role="status">Loading your inbox…</p>}</div></>;
  return <CatchUpPass items={queue.data} onClose={onClose} />;
}
interface Draft { answer: string; optionID?: string; custom?: boolean; revision: number }
function CatchUpPass({ items, onClose }: { items: FeedbackQuestion[]; onClose: () => void }) {
  // This snapshot stays fixed until the overlay closes, including past 50 items.
  const [pass] = useState(items);
  const [index, setIndex] = useState(0);
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const q = pass[index];
  function advance() { setIndex(current => current + 1); }
  return <>
    <CatchUpHeader onClose={onClose} progress={q ? `${index + 1} / ${pass.length}` : "Pass complete"} />
    {q ? <CatchUpMessage key={q.id} initial={q} draft={drafts[q.id]} onDraft={draft => setDrafts(current => ({ ...current, [q.id]: draft }))} onClose={onClose} onPrevious={index ? () => setIndex(index - 1) : undefined} onAdvance={advance} /> : <div className="min-h-0 flex-1 overflow-y-auto p-6">
      <div className="mx-auto max-w-xl space-y-5 py-16 text-center"><CheckCheck className="mx-auto size-9 text-emerald-300" /><h2 className="text-2xl font-semibold">{pass.length ? "You’ve reached the end of this pass" : "You’re all caught up"}</h2><p className="text-sm text-muted-foreground">{pass.length ? "Skipped messages remain pending. New arrivals will be included the next time you open Catch up." : "New requests and completed work will appear here."}</p><div className="flex justify-center gap-3">{pass.length > 0 && <Button variant="outline" onClick={() => setIndex(pass.length - 1)}>Previous message</Button>}<Button onClick={onClose}>Back to your page</Button></div></div>
    </div>}
  </>;
}
function CatchUpMessage({ initial, draft, onDraft, onClose, onPrevious, onAdvance }: {
  initial: FeedbackQuestion; draft?: Draft; onDraft: (draft: Draft) => void;
  onClose: () => void;
  onPrevious?: () => void; onAdvance: () => void;
}) {
  const [notice, setNotice] = useState("");
  const selected = useInboxMessage(initial.id);
  const q = selected.data ?? initial;
  const context = useCatchUpContext(q);
  const action = useFeedbackAction();
  const client = useQueryClient();
  const history = useRef<HTMLDivElement>(null);
  const answerInput = useRef<HTMLTextAreaElement>(null);
  const completion = q.kind === "completion";
  const readingText = catchUpReadingText(q);
  const operational = q.reason === "operational_failure";
  const canChoose = !q.hidden && (completion || q.delivery_mode !== "terminal");
  const choices: FeedbackQuestion["options"] = canChoose ? (completion ? completionActions(q) : q.options) : [];
  const canWrite = !q.hidden && (completion || (q.delivery_mode !== "terminal" && q.delivery_mode !== "claude_question"));
  const customInTerminal = !canWrite && !q.hidden && q.origin !== "notification";
  const custom = canWrite && (draft?.custom ?? (choices.length === 0));
  const pending = q.status === "pending";
  const changed = !!draft && draft.revision !== q.revision;
  const blocked = action.isPending || selected.isError || selected.isPending || !pending || changed;
  const answer = custom ? (draft?.answer ?? "") : "";
  const answerTooLong = new TextEncoder().encode(answer.trim()).length > 16000;
  const optionID = !custom ? choices.find(o => o.id === draft?.optionID)?.id : undefined;
  function edit(update: Partial<Draft>) { onDraft({ answer: draft?.answer ?? "", optionID: draft?.optionID, custom: draft?.custom ?? (choices.length === 0), revision: draft?.revision ?? q.revision, ...update }); }
  async function submit(kind: "answer" | "dismiss") {
    if (blocked || (kind === "answer" && (answerTooLong || (!answer.trim() && !optionID)))) return;
    setNotice("");
    try {
      const result = await action.mutateAsync({ question: q, action: kind, option_id: kind === "answer" && !answer.trim() ? optionID : undefined, answer: kind === "answer" && answer.trim() ? answer : undefined });
      const latest = client.setQueryData<FeedbackQuestion>(["inbox", "message", q.id], current => latestCatchUpMessage(current, result.question));
      if (latest && latest.revision > result.question.revision) {
        setNotice("This message changed again while your response was being saved. Review its latest version before continuing.");
      } else if (catchUpActionSucceeded(result, kind)) onAdvance();
      else setNotice(feedbackOutcomeMessage(result));
    } catch { /* Keep this message and its draft visible, including uncertain delivery. */ }
  }
  return <>
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain" data-testid="catch-up-scroll">
      <div className="mx-auto w-full max-w-[1450px] px-4 py-6 sm:px-8 lg:px-10">
        {notice && <p role="status" className="mb-5 rounded-lg border border-border p-3 text-sm">{notice}</p>}
        {selected.isError && <p role="alert" className="mb-5 text-sm">Couldn’t refresh this message. Actions are paused. <Button variant="outline" size="sm" onClick={() => void selected.refetch()}>Retry message</Button></p>}
        <div className="grid items-start gap-7 border-b border-border pb-7 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] lg:gap-10">
          <section className="min-w-0" aria-label="Current message">
            <CatchUpReferences projects={context.projects.data ?? []} names={context.projectNames} prs={context.prs} scratchpad={q.task_id === 0 && q.origin !== "notification"} />
            <p className="mb-4 text-xs text-muted-foreground">{completion ? "Completed work" : operational ? "Operational issue" : "Needs your decision"} · <time dateTime={new Date(q.created_at).toISOString()}>{new Date(q.created_at).toLocaleString()}</time></p>
            <h1 className="break-words text-2xl font-semibold leading-snug tracking-tight sm:text-3xl"><InboxText text={catchUpHeadline(q)} /></h1>
            {readingText && <div className={`mt-5 ${contextProse}`} data-testid="catch-up-explanation"><InboxText text={readingText} paragraphs /></div>}
            {!!q.explanation?.trim() && <details className="mt-5 border-t border-border pt-3"><summary className="cursor-pointer text-xs text-muted-foreground">Read full context</summary><div className={`mt-4 ${contextProse}`}><InboxText text={catchUpSourceText(q)} paragraphs /></div></details>}
            <Button variant="outline" size="sm" className="mt-5" onClick={() => history.current?.scrollIntoView({ behavior: "smooth", block: "start" })}>How we got here ↓</Button>
          </section>
          <section className={`${contextCard} space-y-4`} aria-label="Your response">
            <h2 className="text-base font-semibold">{completion ? "What should happen next?" : operational ? "Recovery actions" : "Your response"}</h2>
            {completion ? <p className="text-xs text-muted-foreground">Choose what happens next.</p> : <div className="break-words text-base font-medium"><InboxText text={q.question} /></div>}
            {!pending && <p role="status" className="rounded-lg border border-border p-3 text-sm">This message is {q.status}. {q.delivery_status && `Delivery: ${q.delivery_status}.`} {q.answer && <InboxText text={q.answer} />}Skip to continue.</p>}
            {pending && <>
              {(choices.length > 0 || canWrite || customInTerminal) && <div role="group" aria-label="Choose a response" className="space-y-2">{choices.map(option => <div key={option.id}>
                <Button variant="outline" className={`h-auto w-full justify-start whitespace-normal p-3 text-left ${optionID === option.id ? "border-orange-300/60 bg-orange-300/10" : ""}`} aria-pressed={optionID === option.id} disabled={blocked} onClick={() => edit({ optionID: option.id, custom: false })}><span className="mr-2 shrink-0" aria-hidden="true">{optionID === option.id ? "◉" : "○"}</span><span className="min-w-0 break-words">{option.label}</span></Button>
                {option.action && <details className="px-2 pt-1 text-xs text-muted-foreground"><summary className="cursor-pointer">Direct action · saved instructions</summary><p className="mt-2">{option.action_summary}</p>{option.action.guards?.review_seed && <p className="mt-2 whitespace-pre-wrap">Review prompt: {option.action.guards.review_seed}</p>}{option.action.actions.filter(a => a.instructions).map(a => <p key={a.id} className="mt-2 whitespace-pre-wrap">{a.instructions}</p>)}</details>}
              </div>)}
                {canWrite && <Button variant="outline" className={`h-auto w-full justify-start whitespace-normal p-3 text-left ${custom ? "border-orange-300/60 bg-orange-300/10" : ""}`} aria-pressed={custom} aria-expanded={custom} aria-controls={`catch-up-answer-${q.id}`} disabled={blocked} onClick={() => { edit({ custom: true, optionID: undefined }); requestAnimationFrame(() => answerInput.current?.focus()); }}><span className="mr-2 shrink-0" aria-hidden="true">{custom ? "◉" : "○"}</span>Custom answer</Button>}
                {customInTerminal && <Button variant="outline" className="h-auto w-full justify-start whitespace-normal p-3 text-left" asChild><Link to={feedbackTaskURL(q)} onClick={onClose}>Custom answer in terminal ↗</Link></Button>}
              </div>}
              {custom && <div className="border-t border-border pt-3"><label htmlFor={`catch-up-answer-${q.id}`} className="block text-xs text-muted-foreground">Your answer{q.origin === "notification" && " · interpreted once before execution"}</label><Textarea ref={answerInput} id={`catch-up-answer-${q.id}`} value={answer} onChange={e => edit({ answer: e.target.value, custom: true })} placeholder="Write a short answer…" aria-invalid={answerTooLong} aria-describedby={answerTooLong ? `catch-up-answer-error-${q.id}` : undefined} maxLength={16000} rows={2} className="mt-2 resize-y" disabled={action.isPending} />{answerTooLong && <p id={`catch-up-answer-error-${q.id}`} role="alert" className="mt-2 text-xs text-destructive">Your answer is too long. Shorten it before sending.</p>}{completion && <p className="mt-2 text-xs text-muted-foreground">Send follow-up instructions to the agent and keep the session open.</p>}</div>}
              {!canChoose && <p className="text-sm text-muted-foreground">Respond in the originating agent terminal.</p>}
              {q.origin !== "notification" && <Button variant="outline" size="sm" asChild><Link to={feedbackTaskURL(q)} onClick={onClose}>{q.task_id === 0 ? "Open Scratchpad" : "Open task terminal"} ↗</Link></Button>}
              {changed && <div role="alert" className="space-y-2 rounded-lg border border-orange-300/40 p-3 text-sm"><p>This message changed while you were responding. Review the updated context before continuing. Your draft has been kept.</p><Button variant="outline" size="sm" onClick={() => { setNotice(""); edit({ revision: q.revision, optionID: undefined }); }}>I’ve reviewed the updated message</Button></div>}
              <Button variant="ghost" size="sm" disabled={blocked} onClick={() => void submit("dismiss")}>Dismiss message</Button>
            </>}
            {action.isError && <p role="alert" className="break-words text-sm text-destructive">{action.error.message} Your draft has been kept. Review the message and delivery status before retrying.</p>}
          </section>
        </div>
        <div ref={history} className="mt-7 grid scroll-mt-5 items-start gap-8 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] lg:gap-10">
          <CatchUpHistory question={q} /><CatchUpTasks context={context} onNavigate={onClose} />
        </div>
        <CatchUpPRs context={context} />
        {(q.source_url || q.origin === "notification") && <details className={`mt-7 ${contextCard}`}><summary className="cursor-pointer text-sm text-muted-foreground">Source & execution details</summary>
          {q.source_url && <a href={q.source_url} target="_blank" rel="noopener noreferrer" className="mt-4 inline-block text-xs text-blue-400 hover:underline">Open notification source ↗</a>}
          {q.origin === "notification" && <div className="mt-5 space-y-4">{q.attention_source_id && <ActionRunDetails id={q.attention_source_id} label="Original execution steps" />}{!operational && <RuleImprovement questionID={q.id} />}{q.status === "answered" && <ActionRunDetails id={q.id} />}{q.execution_result && <div className={contextProse}><InboxText text={q.execution_result} paragraphs /></div>}</div>}
        </details>}
      </div>
    </div>
    <footer className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border bg-card/60 px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-7">
      <Button variant="ghost" size="sm" disabled={!onPrevious || action.isPending} onClick={onPrevious}><ChevronLeft className="size-4" />Previous</Button>
      <span className="hidden text-xs text-muted-foreground lg:inline">Skip leaves this message pending.</span>
      <div className="flex gap-2"><Button variant="outline" size="sm" disabled={action.isPending} onClick={onAdvance}>Skip for now →</Button><Button size="sm" disabled={blocked || answerTooLong || (!answer.trim() && !optionID)} onClick={() => void submit("answer")}>{action.isPending ? "Sending…" : custom ? "Send reply & next" : optionID ? "Confirm & next" : "Choose a response"}</Button></div>
    </footer>
  </>;
}

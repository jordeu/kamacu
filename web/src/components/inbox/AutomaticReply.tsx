import type { FeedbackQuestion } from "@/api/inbox";
import { InboxText } from "./InboxText";

export function AutomaticReply({ question }: { question: FeedbackQuestion }) {
  const a = question.automation;
  if (!a) return null;
  return <section aria-label="Inbox agent response" className="my-4 space-y-2 rounded border border-border bg-muted/30 p-3 text-sm">
    <p className="font-medium">{a.status === "answered" ? "Automatically answered by Inbox agent" : "Automatic response needs attention"}</p>
    {a.answer && <div><span className="text-xs text-muted-foreground">{a.status === "answered" ? "Response" : "Attempted response"}</span><InboxText text={a.answer} /></div>}
    {a.reason && <InboxText text={a.reason} />}
    {a.rules?.length > 0 && <details><summary className="cursor-pointer text-xs">{a.rules.length === 1 ? "Triggering rule" : "Triggering rules"} · {a.rules.map(r => r.id).join(", ")} · revision {a.config_revision}</summary>{a.rules.map(r => <div key={r.id} className="mt-2"><p className="text-xs text-muted-foreground">{r.id}</p><InboxText text={r.text} /></div>)}</details>}
    {a.error && <p role="alert" className="whitespace-pre-wrap break-words text-destructive">{a.error}</p>}
    {question.delivery_status === "uncertain" && <p>Delivery is uncertain. Inspect the terminal and saved response before sending new instructions. The response will not be sent again automatically.</p>}
  </section>;
}

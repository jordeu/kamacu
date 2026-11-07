import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { ApiError, get, post } from "@/api/client";
import type { NotificationRuleSet } from "@/api/notificationRules";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

type Improvement = { diagnosis: string; cause: string; risks: string; changes: { rule_id: string; text: string; explanation: string }[]; draft?: NotificationRuleSet };
type Comparison = { activity_id: string; url: string; before: string; after: string; limited: boolean };

export function RuleImprovement({ questionID, recurring = false, disabled = false, disabledReason, onBusyChange }: { questionID?: string; recurring?: boolean; disabled?: boolean; disabledReason?: string; onBusyChange?: (busy: boolean) => void }) {
 const [open, setOpen] = useState(false);
 return <div className="mt-3 space-y-3"><Button size="sm" variant="outline" disabled={disabled} onClick={() => setOpen(!open)}>{recurring ? "Review recurring problems" : "Improve rules"}</Button>{disabled && disabledReason && <p className="text-xs text-muted-foreground">{disabledReason}</p>}{open && <ImprovementForm questionID={questionID} disabled={disabled} onBusyChange={onBusyChange} />}</div>;
}
function ImprovementForm({ questionID, disabled, onBusyChange }: { questionID?: string; disabled: boolean; onBusyChange?: (busy: boolean) => void }) {
 const qc = useQueryClient();
 const [kind, setKind] = useState("unclear_rule");
 const [instructions, setInstructions] = useState("");
 const rules = useQuery({ queryKey: ["notification-rules"], queryFn: () => get<NotificationRuleSet[]>("/api/notifications/rules") });
 const problems = useQuery({ queryKey: ["notification-rule-problems"], queryFn: () => get<{rule_id: string; reason: string; count: number}[]>("/api/notifications/rules/problems") });
 const suggest = useMutation({ onMutate: () => onBusyChange?.(true), onSettled: () => onBusyChange?.(false), onError: error => { if (error instanceof ApiError && error.status === 409) void qc.invalidateQueries({ queryKey: ["notification-rules"] }); }, mutationFn: () => post<Improvement>("/api/notifications/rules/improvements", { base_revision: rules.data?.[0]?.revision, question_id: questionID ?? "", kind, instructions }), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-rules"] }) });
 return <section className="space-y-3 rounded border border-border p-3" aria-label="Improve notification rules">
  <p className="text-xs text-muted-foreground">Explain the behavior you want. Diagnosis can identify missing source data or an engine problem without changing your preferences. Suggested edits create a draft; you must prepare, compare and activate it in Settings.</p>
  <label className="block text-sm">What went wrong? <select className="rounded border bg-background p-1" value={kind} disabled={disabled || suggest.isPending} onChange={e => setKind(e.target.value)}><option value="unclear_rule">Rule unclear</option><option value="already_handled">Already handled</option><option value="not_relevant">Not relevant</option><option value="wrong_task">Wrong task</option><option value="show_similar">Show similar updates</option></select></label>
  <Textarea aria-label="Desired notification behavior" placeholder="What should happen next time? Include any exceptions you want to preserve." maxLength={4000} disabled={disabled || suggest.isPending} value={instructions} onChange={e => setInstructions(e.target.value)} />
  {problems.data && problems.data.length > 0 && <details className="text-xs"><summary>Recurring uncertainties</summary>{problems.data.map((p,i) => <p key={i}>{p.rule_id}: {p.reason} · {p.count} recent decisions</p>)}</details>}
  <Button size="sm" disabled={disabled || !instructions.trim() || !rules.data?.length || suggest.isPending} onClick={() => suggest.mutate()}>{suggest.isPending ? "Diagnosing…" : "Diagnose and propose draft"}</Button>
  {(suggest.error || rules.error || problems.error) && <p role="alert">{(suggest.error || rules.error || problems.error)?.message}</p>}
  {suggest.data && <div className="space-y-2 text-sm"><p>{suggest.data.diagnosis}</p><p>Cause: {suggest.data.cause}</p><p>{suggest.data.risks}</p>{suggest.data.changes.map(c => <div key={c.rule_id}><strong>{c.rule_id}</strong><p>{c.text}</p><p className="text-xs text-muted-foreground">{c.explanation}</p></div>)}{suggest.data.draft && <p>Draft revision {suggest.data.draft.revision} saved. Active rules have not changed. <Link className="underline" to="/settings">Review in Settings</Link></p>}</div>}
 </section>;
}
export function RuleHistoryComparison({ setID }: { setID: string }) {
 const [open,setOpen] = useState(false);
 return <div><Button size="sm" variant="outline" onClick={() => setOpen(!open)}>Compare against processed history</Button>{open && <History setID={setID} />}</div>;
}
function History({setID}: {setID:string}) {
 const q = useQuery({queryKey:["notification-rule-history",setID],queryFn:()=>get<Comparison[]>(`/api/notifications/rules/history/${setID}`)});
 return <div className="mt-2 space-y-2 text-xs" aria-label="Historical rule comparison"><p>Uses recorded facts and semantic judgments from recent processed activities. No live source reads, messages or task actions. Compares rule eligibility; existing message receipts and task routing are not simulated. Missing evidence and LLM rules are marked incomplete, and unknowable outcomes are shown as unknown.</p>{q.isPending && <p>Comparing…</p>}{q.error && <p role="alert">{q.error.message}</p>}{q.data?.length === 0 && <p>No recorded decisions yet.</p>}{q.data?.map(c => <p key={c.activity_id}><a className="underline" href={c.url} target="_blank" rel="noopener noreferrer">Source</a>: {c.before} → {c.after}{c.limited && " · incomplete evidence"}</p>)}</div>;
}

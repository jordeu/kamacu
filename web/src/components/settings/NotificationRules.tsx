import { RuleImprovement, RuleHistoryComparison } from "@/components/inbox/RuleImprovement";
import { useState } from "react";
import { useNotificationRules, useRuleMutation, useRulePreview, type NotificationRuleSet, type NotificationRule, type RuleExpression } from "@/api/notificationRules";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

function describe(e: RuleExpression): string {
 if (e.op === "always") return "Always";
 if (e.op === "semantic") return e.text ?? "Semantic condition";
 if (e.op === "not") return `Not (${describe(e.args![0])})`;
 if (e.op === "all" || e.op === "any") return (e.args ?? []).map(describe).join(e.op === "all" ? " AND " : " OR ");
 return `${e.fact} ${e.op === "in" ? "is one of" : "equals"} ${JSON.stringify(e.value)}`;
}
export function NotificationRules() {
 const query = useNotificationRules();
 const [dirtyEditorID, setDirtyEditorID] = useState<string | null>(null);
 const [diagnosing, setDiagnosing] = useState(false);
 if (query.isPending) return <p>Loading notification rules…</p>;
 if (query.isError) return <p role="alert">Could not load rules. <button className="underline" onClick={() => void query.refetch()}>Retry</button></p>;
 const latest = query.data[0];
 const active = query.data.find(s => s.status === "active");
 return <div className="space-y-3"><p className="text-xs text-muted-foreground">{active ? `Active rule set: revision ${active.revision}.` : "No active rule set. Save, prepare and activate rules before processing notifications."} Edits stay in a draft until activated. Missing evidence that affects filtering or routing creates an Inbox clarification.</p><RuleImprovement recurring disabled={dirtyEditorID === (latest?.id ?? "new") || latest?.status === "compiling"} disabledReason={latest?.status === "compiling" ? "Wait for rule preparation to finish." : "Save your rule edits before requesting suggestions."} onBusyChange={setDiagnosing} /><RuleEditor key={latest?.id ?? "new"} set={latest} active={active} diagnosing={diagnosing} onDirtyChange={dirty => setDirtyEditorID(dirty ? latest?.id ?? "new" : null)} /></div>;
}
function RuleEditor({ set, active, diagnosing, onDirtyChange }: { set?: NotificationRuleSet; active?: NotificationRuleSet; diagnosing: boolean; onDirtyChange: (dirty: boolean) => void }) {
 const [editedRules, setEditedRules] = useState<NotificationRule[]>(set?.rules ?? []);
 const [dirty, setDirty] = useState(false);
 const rules = dirty ? editedRules : set?.rules ?? editedRules;
 const setRules = (value: NotificationRule[]) => { setEditedRules(value); setDirty(true); onDirtyChange(true); };
 const [previewID, setPreviewID] = useState("");
 const save = useRuleMutation("drafts"), compile = useRuleMutation("compile"), activate = useRuleMutation("activate"), preview = useRuleMutation("preview");
 const result = useRulePreview(previewID);
 const busy = diagnosing || save.isPending || compile.isPending || activate.isPending || set?.status === "compiling";
 const change = (i: number, patch: Partial<NotificationRule>) => setRules(rules.map((r, j) => j === i ? { ...r, ...patch } : r));
 return <section className="space-y-3 rounded border border-border p-3" aria-label="Notification rules">
  <h3 className="text-sm font-medium">Notification rules {set && <span className="text-muted-foreground">· revision {set.revision} · {set.status}</span>}</h3>
  <p className="text-xs text-muted-foreground">Write one instruction per rule. Preparation compiles rules where possible. Other rules are evaluated by the LLM during notification processing. Review each mode before activating. Rules propose work; only your inbox reply authorizes execution.</p>
  {(dirty ? rules : set?.rules ?? rules).map((rule, i) => <div key={rule.id || `new-${i}`} className="space-y-2 rounded bg-muted/40 p-3">
   <div className="flex items-center justify-between gap-2"><label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={rule.enabled} disabled={busy} onChange={e => change(i, { enabled: e.target.checked })} />{rule.id || "New rule"}</label><Button size="sm" variant="ghost" disabled={busy} onClick={() => setRules(rules.filter((_, j) => j !== i))}>Remove</Button></div>
   <p className="text-xs font-medium">{!rule.enabled ? "Disabled" : rule.mode === "llm" ? "LLM evaluated · not compiled" : rule.clauses?.length && rule.compiler_version ? "Compiled" : "Not compiled · awaiting preparation"}</p>
   <label className="text-xs">Execution mode <select className="ml-2 rounded border bg-background p-1" value={rule.mode === "llm" ? "llm" : "compiled"} disabled={busy} onChange={e => change(i, { mode: e.target.value as "compiled" | "llm", clauses: [], compiler_version: undefined, diagnostic: undefined, alternatives: [] })}><option value="compiled">Try compilation, fall back to LLM</option><option value="llm">Use LLM evaluation</option></select></label>
   <Textarea aria-label={`Rule ${i + 1}`} value={rule.text} disabled={busy} maxLength={8000} rows={3} onChange={e => change(i, { text: e.target.value, clauses: [], compiler_version: undefined, diagnostic: undefined, alternatives: [] })} />
   {rule.mode === "llm" && <p className="text-xs text-muted-foreground">This rule is interpreted from its full text for each activity. Missing or ambiguous evidence creates an Inbox clarification.</p>}
   {rule.diagnostic && <div className="space-y-1 text-xs" role="status"><p>Why this rule could not be compiled: {rule.diagnostic}</p>{!!rule.alternatives?.length && <><p>Suggested alternative rules (not applied automatically):</p><ul className="list-disc pl-4">{rule.alternatives.map((text, j) => <li key={j}>{text}</li>)}</ul></>}</div>}
   {rule.mode !== "llm" && rule.clauses?.length > 0 && <details className="text-xs"><summary className="cursor-pointer">Compiled interpretation</summary>{rule.clauses.map(c => <p key={c.id} className="mt-2 break-words"><strong>{c.id} · {c.category}</strong>: {describe(c.when)} → {c.effect.replaceAll("_", " ")}{c.argument && ` (${c.argument})`}. Missing evidence: {c.on_unknown === "hold" ? "Ask in Inbox" : "Continue"}. Priority: {c.priority}.</p>)}</details>}
  </div>)}
  <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={busy || rules.length >= 100} onClick={() => setRules([...rules, { id: "", revision: 0, text: "", enabled: true, clauses: [] }])}>Add rule</Button><Button size="sm" disabled={busy || rules.some(r => !r.text.trim()) || (!!set && !dirty)} onClick={() => save.mutate({ base_revision: set?.revision ?? 0, rules }, { onSuccess: () => onDirtyChange(false) })}>Save draft</Button>
   {set && ["draft", "invalid"].includes(set.status) && <Button size="sm" variant="outline" disabled={busy || dirty} onClick={() => compile.mutate({ id: set.id })}>Prepare rules</Button>}
   {set && ["ready", "active"].includes(set.status) && <Button size="sm" variant="outline" disabled={busy || dirty || preview.isPending} onClick={() => preview.mutate({ id: set.id }, { onSuccess: data => setPreviewID(data.id) })}>Test on recent notifications</Button>}
   {set && ["ready", "active"].includes(set.status) && !dirty && <RuleHistoryComparison setID={set.id} />}
   {set?.status === "ready" && <Button size="sm" disabled={busy || dirty} onClick={() => activate.mutate({ id: set.id })}>Activate revision {set.revision}</Button>}
  </div>
  {set?.status === "compiling" && <p role="status" className="text-xs">Preparing rules…</p>}
  {set?.diagnostics?.map((d, i) => <p key={i} role="alert" className="text-xs text-destructive">{d}</p>)}
  {[save, compile, activate, preview].map((m, i) => m.isError && <p key={i} role="alert" className="text-xs text-destructive">{m.error.message}</p>)}
  {active && set?.id !== active.id && <details className="text-xs"><summary>Currently active rules (revision {active.revision})</summary>{active.rules.map(r => <p key={r.id}>{r.enabled ? "Enabled" : "Disabled"} · {r.mode === "llm" ? "LLM evaluated" : "Compiled"} · {r.id}: {r.text}</p>)}</details>}
  {previewID && <div className="space-y-2 border-t border-border pt-3 text-xs" aria-label="Rule preview"><p>Preview: {result.data?.status ?? "loading"}. No inbox cards are published and no notifications are archived.</p>{(result.error || result.data?.error) && <p role="alert" className="text-destructive">{result.data?.error || result.error?.message}</p>}{result.data?.result.decisions?.map(d => <p key={d.activity_id}><strong>{d.outcome}</strong> · {String(d.facts["subject.url"]?.value ?? d.facts.repository?.value ?? "")} · {d.evaluations.map(e => `${e.clause_id}: ${e.result}${e.reason ? ` (${e.reason})` : ""}`).join("; ") || "Default event handling"}</p>)}{result.data?.result.cards?.map((c, i) => <div key={i} className="rounded bg-muted p-2"><strong>{c.question}</strong><p>{c.summary}</p>{c.options.map(o => <p key={o}>· {o}</p>)}</div>)}</div>}
 </section>;
}

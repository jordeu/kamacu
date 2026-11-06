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
 if (query.isPending) return <p>Loading notification rules…</p>;
 if (query.isError) return <p role="alert">Could not load rules. <button className="underline" onClick={() => void query.refetch()}>Retry</button></p>;
 const latest = query.data[0];
 const active = query.data.find(s => s.status === "active");
 return <div className="space-y-3"><p className="text-xs text-muted-foreground">{active ? `Active rule set: revision ${active.revision}.` : "No active rule set. Save, compile and activate rules before processing notifications."} Edits stay in a draft until activated. Rules define whether missing evidence holds activity outside the inbox.</p><RuleEditor key={latest?.id ?? "new"} set={latest} active={active} /></div>;
}
function RuleEditor({ set, active }: { set?: NotificationRuleSet; active?: NotificationRuleSet }) {
 const [rules, setRules] = useState<NotificationRule[]>(set?.rules ?? []);
 const [previewID, setPreviewID] = useState("");
 const save = useRuleMutation("drafts"), compile = useRuleMutation("compile"), activate = useRuleMutation("activate"), preview = useRuleMutation("preview");
 const result = useRulePreview(previewID);
 const dirty = JSON.stringify(rules.map(r => [r.id, r.text, r.enabled])) !== JSON.stringify((set?.rules ?? []).map(r => [r.id, r.text, r.enabled]));
 const busy = save.isPending || compile.isPending || activate.isPending || set?.status === "compiling";
 const change = (i: number, patch: Partial<NotificationRule>) => setRules(rules.map((r, j) => j === i ? { ...r, ...patch } : r));
 return <section className="space-y-3 rounded border border-border p-3" aria-label="Notification rules">
  <h3 className="text-sm font-medium">Notification rules {set && <span className="text-muted-foreground">· revision {set.revision} · {set.status}</span>}</h3>
  <p className="text-xs text-muted-foreground">Write one instruction per rule. Compilation identifies filtering, grouping, action and presentation clauses. Review their interpretation before activating. Rules propose work; only your inbox reply authorizes execution.</p>
  {(dirty ? rules : set?.rules ?? rules).map((rule, i) => <div key={rule.id || `new-${i}`} className="space-y-2 rounded bg-muted/40 p-3">
   <div className="flex items-center justify-between gap-2"><label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={rule.enabled} disabled={busy} onChange={e => change(i, { enabled: e.target.checked })} />{rule.id || "New rule"}</label><Button size="sm" variant="ghost" disabled={busy} onClick={() => setRules(rules.filter((_, j) => j !== i))}>Remove</Button></div>
   <Textarea aria-label={`Rule ${i + 1}`} value={rule.text} disabled={busy} maxLength={8000} rows={3} onChange={e => change(i, { text: e.target.value })} />
   {rule.clauses?.length > 0 && <details className="text-xs"><summary className="cursor-pointer">Compiled interpretation</summary>{rule.clauses.map(c => <p key={c.id} className="mt-2 break-words"><strong>{c.id} · {c.category}</strong>: {describe(c.when)} → {c.effect.replaceAll("_", " ")}{c.argument && ` (${c.argument})`}. Missing evidence: {c.on_unknown}. Priority: {c.priority}.</p>)}</details>}
  </div>)}
  <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={busy || rules.length >= 100} onClick={() => setRules([...rules, { id: "", revision: 0, text: "", enabled: true, clauses: [] }])}>Add rule</Button><Button size="sm" disabled={busy || rules.some(r => !r.text.trim()) || (!!set && !dirty)} onClick={() => save.mutate({ base_revision: set?.revision ?? 0, rules })}>Save draft</Button>
   {set && ["draft", "invalid"].includes(set.status) && <Button size="sm" variant="outline" disabled={busy || dirty} onClick={() => compile.mutate({ id: set.id })}>Compile rules</Button>}
   {set && ["ready", "active"].includes(set.status) && <Button size="sm" variant="outline" disabled={busy || dirty || preview.isPending} onClick={() => preview.mutate({ id: set.id }, { onSuccess: data => setPreviewID(data.id) })}>Test on recent notifications</Button>}
   {set?.status === "ready" && <Button size="sm" disabled={busy || dirty} onClick={() => activate.mutate({ id: set.id })}>Activate revision {set.revision}</Button>}
  </div>
  {set?.status === "compiling" && <p role="status" className="text-xs">Compiling rules…</p>}
  {set?.diagnostics?.map((d, i) => <p key={i} role="alert" className="text-xs text-destructive">{d}</p>)}
  {[save, compile, activate, preview].map((m, i) => m.isError && <p key={i} role="alert" className="text-xs text-destructive">{m.error.message}</p>)}
  {active && set?.id !== active.id && <details className="text-xs"><summary>Currently active rules (revision {active.revision})</summary>{active.rules.map(r => <p key={r.id}>{r.enabled ? "Enabled" : "Disabled"} · {r.id}: {r.text}</p>)}</details>}
  {previewID && <div className="space-y-2 border-t border-border pt-3 text-xs" aria-label="Rule preview"><p>Preview: {result.data?.status ?? "loading"}. No inbox cards are published and no notifications are archived.</p>{(result.error || result.data?.error) && <p role="alert" className="text-destructive">{result.data?.error || result.error?.message}</p>}{result.data?.result.decisions?.map(d => <p key={d.activity_id}><strong>{d.outcome}</strong> · {String(d.facts["subject.url"]?.value ?? d.facts.repository?.value ?? "")} · {d.evaluations.map(e => `${e.clause_id}: ${e.result}${e.reason ? ` (${e.reason})` : ""}`).join("; ") || "Default event handling"}</p>)}{result.data?.result.cards?.map((c, i) => <div key={i} className="rounded bg-muted p-2"><strong>{c.question}</strong><p>{c.summary}</p>{c.options.map(o => <p key={o}>· {o}</p>)}</div>)}</div>}
 </section>;
}

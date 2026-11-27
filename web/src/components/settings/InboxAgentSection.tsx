import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { get } from "@/api/client";
import { useInboxAgentConfig, useSaveInboxAgentConfig, useTestInboxAgent, type InboxAgentConfig } from "@/api/inboxAgent";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

export function InboxAgentSection() {
  const query = useInboxAgentConfig();
  const profiles = useQuery({ queryKey: ["model-profiles"], queryFn: () => get<{ profiles: { id: number; name: string; model: string; backend: string }[] }>("/api/monitor") });
  const save = useSaveInboxAgentConfig();
  const test = useTestInboxAgent();
  const [draft, setDraft] = useState<InboxAgentConfig | null>(null);
  const config = draft ?? query.data;
  const stale = !!draft && draft.revision !== query.data?.revision;
  if (!config || !profiles.data) return <p role={query.isError || profiles.isError ? "alert" : "status"}>{query.isError || profiles.isError ? "Could not load inbox agent settings." : "Loading inbox agent settings…"}<button className="ml-2 underline" onClick={() => { void query.refetch(); void profiles.refetch(); }}>Retry</button></p>;
  function change(value: InboxAgentConfig) { setDraft(value); test.reset(); }
  return <div className="space-y-3">
    <p className="text-xs text-muted-foreground">Checks each new monitoring or notification message before it appears in the Inbox. Only a clearly matching enabled rule authorizes an immediate reply or existing action button. Successful replies go directly to Archived with the response and rule. Unclear cases and failures remain in Pending.</p>
    <label className="block text-sm">Inbox model
      <select aria-label="Inbox model" className="ml-2 max-w-full rounded border bg-background p-2" value={config.profile_id ?? ""} disabled={save.isPending} onChange={e => change({ ...config, profile_id: e.target.value ? Number(e.target.value) : null })}>
        <option value="">Off — choose a model to enable</option>
        {profiles.data.profiles.map(p => <option key={p.id} value={p.id}>{p.name} · {p.model || p.backend}</option>)}
      </select>
    </label>
    <p className="text-xs text-muted-foreground">Independent of monitoring and notification model selections. A model and at least one enabled rule are required.</p>
    <Button size="sm" variant="outline" disabled={!query.data?.profile_id || !!draft || test.isPending} onClick={() => test.mutate()}>{test.isPending ? "Testing…" : "Test inbox model"}</Button>
    {test.data && <p role="status" className="text-xs">Inbox response contract verified.</p>}
    <div className="space-y-3 rounded border p-3">
      <h3 className="text-sm font-medium">Inbox rules · revision {config.revision}</h3>
      <p className="text-xs text-muted-foreground">Specify the exact trigger, repository/project, relevant user, required evidence and response or action. Missing or ambiguous facts leave the message for you. Rules apply only to new publications; saving does not process your existing queue.</p>
      {config.rules.length === 0 && <p className="text-xs text-muted-foreground">No rules yet. Examples: start a rereview only when you previously approved that PR; start a seqeralabs-renovate request in a named repository; approve a release request only for a named author and project.</p>}
      {config.rules.map((rule, i) => <div key={rule.id || `new-${i}`} className="space-y-2 rounded bg-muted/40 p-3">
        <div className="flex items-center justify-between gap-2">
          <label className="text-xs"><input type="checkbox" checked={rule.enabled} disabled={save.isPending} onChange={e => change({ ...config, rules: config.rules.map((r, j) => j === i ? { ...r, enabled: e.target.checked } : r) })} /> {rule.id || "New inbox rule"}</label>
          <Button size="sm" variant="ghost" disabled={save.isPending} onClick={() => change({ ...config, rules: config.rules.filter((_, j) => j !== i) })}>Remove</Button>
        </div>
        <Textarea aria-label={`Inbox rule ${rule.id || i + 1}`} value={rule.text} rows={4} maxLength={8000} disabled={save.isPending} onChange={e => change({ ...config, rules: config.rules.map((r, j) => j === i ? { ...r, text: e.target.value } : r) })} />
      </div>)}
      <Button size="sm" variant="outline" disabled={save.isPending || config.rules.length >= 100} onClick={() => change({ ...config, rules: [...config.rules, { id: "", text: "", enabled: true }] })}>Add inbox rule</Button>
    </div>
    <details className="space-y-2"><summary className="cursor-pointer text-xs">Inbox processing limits</summary><div className="grid gap-2 sm:grid-cols-3">
      {([ ["context_bytes", "Context budget (bytes)", 64000, 8000000], ["output_tokens", "API output token limit", 1024, 64000], ["timeout_seconds", "Assessment deadline (seconds)", 30, 900] ] as const).map(([key, label, min, max]) => <label key={key} className="text-xs">{label}<Input type="number" min={min} max={max} value={config[key]} onChange={e => change({ ...config, [key]: Number(e.target.value) })} /></label>)}
    </div></details>
    {stale && <p role="alert" className="text-sm">Settings changed while you were editing. Discard and reload before saving.</p>}
    <div className="flex gap-2"><Button size="sm" disabled={!draft || stale || save.isPending || config.rules.some(r => !r.text.trim())} onClick={() => save.mutate(config, { onSuccess: () => setDraft(null) })}>Save inbox agent</Button>{draft && <Button size="sm" variant="ghost" disabled={save.isPending} onClick={() => { setDraft(null); test.reset(); }}>Discard edits and reload</Button>}</div>
    {(save.error || test.error) && <p role="alert" className="text-sm text-destructive">{(save.error || test.error)?.message}</p>}
  </div>;
}

import { useState } from "react";
import { useAgents, useProjects } from "@/api/queries";
import { useSessions } from "@/api/sessions";
import { useUpdateTask } from "@/api/mutations";
import type { Task } from "@/api/types";
import { Button } from "@/components/ui/button";

interface Draft {
  savedAgentId: number | null;
  savedModel: string;
  agentId: number | null;
  model: string;
  modelAgentId: number | undefined;
}

export function TaskAgentSettings({ task, projectId }: { task: Task; projectId: number }) {
  const agents = useAgents();
  const projects = useProjects();
  const sessions = useSessions(task.id);
  const update = useUpdateTask(projectId);
  const [draft, setDraft] = useState<Draft | null>(null);
  const projectAgentId = projects.data?.find((project) => project.id === projectId)?.agent_id;
  const savedAgentId = task.agent_id ?? projectAgentId;
  const agentId = draft ? draft.agentId : task.agent_id ?? null;
  const effectiveAgentId = agentId ?? projectAgentId;
  const agent = agents.data?.find((entry) => entry.id === effectiveAgentId);
  const projectAgent = agents.data?.find((entry) => entry.id === projectAgentId);
  const candidate = draft ? draft.model : task.model ?? "";
  const savedModel = effectiveAgentId === savedAgentId && candidate === task.model;
  const model = candidate;
  const configuredModel = !!agent?.model_argument && !!agent.models?.includes(model);
  const unavailableModel = model !== "" &&
    ((draft !== null && draft.modelAgentId !== effectiveAgentId) || (!savedModel && !configuredModel));
  const changedElsewhere = draft !== null &&
    (draft.savedAgentId !== (task.agent_id ?? null) || draft.savedModel !== (task.model ?? ""));
  const running = sessions.data?.some((entry) => entry.kind === "agent" && entry.status === "running");
  const loadError = agents.isError || projects.isError || sessions.isError;
  const disabled = running || loadError || !agent || sessions.isPending || update.isPending;

  function edit(next: Omit<Draft, "savedAgentId" | "savedModel">) {
    update.reset();
    setDraft({ ...next, savedAgentId: draft ? draft.savedAgentId : task.agent_id ?? null, savedModel: draft ? draft.savedModel : task.model ?? "" });
  }

  function changeAgent(value: string) {
    const next = value === "" ? null : Number(value);
    const nextEffective = next ?? projectAgentId;
    edit({ agentId: next, model: nextEffective === effectiveAgentId ? model : "", modelAgentId: nextEffective });
  }

  function save() {
    if (disabled || unavailableModel || changedElsewhere || !agent || !draft) return;
    // A specific model belongs to the agent shown in this editor, even if
    // another tab changes the project default before the save reaches the server.
    update.mutate({ id: task.id, agent_id: model ? agent.id : agentId, model, expected_agent_id: draft.savedAgentId, expected_model: draft.savedModel }, { onSuccess: () => setDraft(null) });
  }

  return (
    <section aria-label="Task agent configuration" className="space-y-3 rounded-md border p-3">
      <h3 className="text-sm font-medium">Agent and model</h3>
      <p className="text-xs text-muted-foreground">
        {running ? "Stop the running agent before changing its agent or model." : "Changes apply the next time you start or resume the agent."}
      </p>
      <fieldset disabled={disabled} className="grid gap-3 sm:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-1.5">
          <label htmlFor="description-agent" className="text-xs font-medium">Agent</label>
          <select id="description-agent" className="h-9 min-w-0 rounded-md border bg-background px-2 text-sm disabled:opacity-50" value={agentId ?? ""} onChange={(event) => changeAgent(event.target.value)}>
            <option value="">Project default{projectAgent ? ` (${projectAgent.name})` : ""}</option>
            {agents.data?.map((entry) => <option key={entry.id} value={entry.id}>{entry.name}</option>)}
          </select>
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <label htmlFor="description-model" className="text-xs font-medium">Model</label>
          <select id="description-model" className="h-9 min-w-0 rounded-md border bg-background px-2 text-sm disabled:opacity-50" value={model} disabled={!agent?.model_argument && !model} onChange={(event) => {
            edit({ agentId, model: event.target.value, modelAgentId: effectiveAgentId });
          }}>
            <option value="">Agent default</option>
            {model && !configuredModel && <option value={model}>{model} ({savedModel ? "saved" : "unavailable"})</option>}
            {(agent?.model_argument ? agent.models : [])?.map((id) => <option key={id} value={id}>{id}</option>)}
          </select>
        </div>
      </fieldset>
      {agent && !agent.models?.length && <p className="text-xs text-muted-foreground">Add available models in Settings → Agents, or use Agent default.</p>}
      {loadError && <div className="flex items-center gap-2 text-xs">
        <p role="alert">Could not load agent configuration or session status.</p>
        <Button size="sm" variant="ghost" onClick={() => { void agents.refetch(); void projects.refetch(); void sessions.refetch(); }}>Retry</Button>
      </div>}
      {changedElsewhere && <p role="alert" className="text-xs text-destructive">Agent settings changed elsewhere. Cancel these edits to load the saved settings.</p>}
      {unavailableModel && <p role="alert" className="text-xs text-destructive">The selected model is no longer available for this agent. Choose a model or Agent default before saving.</p>}
      {draft && <div className="flex items-center gap-2">
        <Button size="sm" onClick={save} disabled={disabled || unavailableModel || changedElsewhere}>Save agent and model</Button>
        <Button size="sm" variant="ghost" disabled={update.isPending} onClick={() => { setDraft(null); update.reset(); }}>Cancel</Button>
      </div>}
      {update.isError && <p role="alert" className="text-xs text-destructive">Could not save agent and model. {update.error.message}</p>}
    </section>
  );
}

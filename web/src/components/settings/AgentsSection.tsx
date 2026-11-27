import { useState } from "react";
import { Plus } from "lucide-react";
import { useProjects, useAgents } from "@/api/queries";
import { useDeleteAgent, useSetDefaultAgent } from "@/api/mutations";
import type { Agent } from "@/api/types";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AgentNameDialog } from "@/components/settings/AgentNameDialog";

export function AgentsSection() {
  const agentsQuery = useAgents();
  const agents = agentsQuery.data ?? [];
  const { data: projects } = useProjects();
  const deleteAgent = useDeleteAgent();
  const setDefaultAgent = useSetDefaultAgent();
  const [defaultDraft, setDefaultDraft] = useState<number | null>(null);
  const [defaultNotice, setDefaultNotice] = useState("");
  const currentDefault = agents.find(agent => agent.is_default)?.id;
  const draft = agents.find(agent => agent.id === defaultDraft);
  const selectedDefault = draft?.id ?? currentDefault ?? "";

  const [nameOpen, setNameOpen] = useState(false);
  const [nameMode, setNameMode] = useState<"create" | "edit">("create");
  const [nameTarget, setNameTarget] = useState<Agent | undefined>(undefined);
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null);

  const countByAgent = new Map<number, number>();
  for (const p of projects ?? []) {
    if (p.agent_id == null) continue;
    countByAgent.set(p.agent_id, (countByAgent.get(p.agent_id) ?? 0) + 1);
  }

  function openCreate() {
    setNameMode("create");
    setNameTarget(undefined);
    setNameOpen(true);
  }

  function openEdit(a: Agent) {
    setNameMode("edit");
    setNameTarget(a);
    setNameOpen(true);
  }

  function handleDelete() {
    if (!deleteTarget || deleteAgent.isPending) return;
    deleteAgent.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) });
  }

  return (
    <>
      <div className="space-y-4">
        <div className="flex justify-end">
          <Button onClick={openCreate}><Plus /> New agent</Button>
        </div>
        {agentsQuery.isPending && <p role="status" className="text-sm text-muted-foreground">Loading coding agents…</p>}
        {agentsQuery.isError && (
          <div className="flex flex-wrap items-center gap-2">
            <p role="alert" className="text-sm text-destructive">Could not load coding agents.</p>
            <Button variant="outline" size="sm" onClick={() => agentsQuery.refetch()}>Retry</Button>
          </div>
        )}
        {agents.length > 0 && (
          <div className="space-y-2 rounded-lg border bg-muted/30 p-4">
            <label htmlFor="new-project-agent" className="text-sm font-medium">Default for new projects</label>
            <p id="new-project-agent-hint" className="text-sm text-muted-foreground">
              Used when you create a project. Existing project and task assignments keep their current agent.
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <select
                id="new-project-agent"
                aria-describedby="new-project-agent-hint"
                className="min-h-11 min-w-0 flex-1 rounded-md border border-input bg-background px-3 text-sm md:min-h-9"
                value={selectedDefault}
                disabled={setDefaultAgent.isPending}
                onChange={event => {
                  setDefaultDraft(Number(event.target.value));
                  setDefaultNotice("");
                  setDefaultAgent.reset();
                }}
              >
                {selectedDefault === "" && <option value="" disabled>Choose an agent</option>}
                {agents.map(agent => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
              </select>
              <Button
                disabled={!draft || draft.id === currentDefault || setDefaultAgent.isPending}
                onClick={() => {
                  if (!draft || setDefaultAgent.isPending) return;
                  setDefaultAgent.mutate(draft.id, {
                    onSuccess: agent => {
                      setDefaultDraft(null);
                      setDefaultNotice(`${agent.name} is now the default for new projects.`);
                    },
                  });
                }}
              >
                {setDefaultAgent.isPending ? "Saving…" : "Save default"}
              </Button>
            </div>
            <p role="status" className="text-sm text-muted-foreground empty:hidden">{defaultNotice}</p>
            {setDefaultAgent.isError && (
              <p role="alert" className="text-sm text-destructive">Could not save the default agent. {setDefaultAgent.error.message}</p>
            )}
          </div>
        )}
        {agents.length > 0 && (
          <ul aria-label="Configured coding agents" className="divide-y rounded-lg border">
            {agents.map(agent => {
              const count = countByAgent.get(agent.id) ?? 0;
              const models = agent.models?.length ?? 0;
              return (
                <li key={agent.id} className="flex flex-wrap items-start justify-between gap-3 p-3">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="break-all text-sm font-medium">{agent.name}</span>
                      <Badge variant="secondary">{agent.is_system ? "Built-in" : "Custom"}</Badge>
                    </div>
                    <p className="break-all text-xs text-muted-foreground">
                      <code>{agent.command}</code> · {models > 0 ? `${models} ${models === 1 ? "model" : "models"}` : "Uses CLI default"}
                    </p>
                    {!agent.is_system && count > 0 && (
                      <p className="text-xs text-muted-foreground">Reassign {count} {count === 1 ? "project" : "projects"} before deleting.</p>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    <Button variant="outline" size="sm" aria-label={`Configure ${agent.name}`} onClick={() => openEdit(agent)}>
                      Configure
                    </Button>
                    {!agent.is_system && count === 0 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={`Delete ${agent.name}`}
                        onClick={() => { deleteAgent.reset(); setDeleteTarget(agent); }}
                      >
                        Delete
                      </Button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <AgentNameDialog
        mode={nameMode}
        agent={nameTarget}
        open={nameOpen}
        onOpenChange={setNameOpen}
      />

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(next) => {
          if (!next) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete agent?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget
                ? `"${deleteTarget.name}" will be removed. Agents assigned to tasks, projects, workspaces, or the Scratchpad must be unassigned before deletion.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteAgent.isError && <p role="alert" className="text-sm text-destructive">Could not delete the agent. {deleteAgent.error.message}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={deleteAgent.isPending} onClick={(event) => { event.preventDefault(); handleDelete(); }}>
              Delete agent
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

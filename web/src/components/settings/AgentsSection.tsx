import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { AgentNameDialog } from "@/components/settings/AgentNameDialog";

const SECTION_HELP = `Agents available in project task tabs. The default is used for new projects.`;

export interface AgentsSectionProps {}

/**
 * The M001 agents management section — one more Settings <section> in the 640px
 * column, mirroring the ManageWorkspacesDialog hub but as a page section (per
 * D008, reuse the workspaces pattern; per the milestone scope this is name-only,
 * no icons). One row per agent with: the name + an engine badge (Claude/Custom),
 * a set-default radio, an Edit control, and a guarded delete. Plus a "New agent"
 * entry that opens AgentNameDialog in create mode.
 *
 * Delete guards are enforced server-side (is_system non-deletable; in-use
 * block-until-unassigned) + ON DELETE RESTRICT; here they are UX affordances
 * only, mirroring ManageWorkspacesDialog's count-by-parent derivation.
 */
export function AgentsSection({}: AgentsSectionProps) {
  const { data: agents } = useAgents();
  const { data: projects } = useProjects();
  const deleteAgent = useDeleteAgent();
  const setDefaultAgent = useSetDefaultAgent();

  const [nameOpen, setNameOpen] = useState(false);
  const [nameMode, setNameMode] = useState<"create" | "edit">("create");
  const [nameTarget, setNameTarget] = useState<Agent | undefined>(undefined);
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null);

  // Per-agent project count, keyed by agent_id (mirrors the workspaces'
  // countByWorkspace). Used to disable + tooltip the delete for in-use agents.
  const countByAgent = new Map<number, number>();
  for (const p of projects ?? []) {
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

  async function handleDelete() {
    if (!deleteTarget) return;
    await deleteAgent.mutateAsync(deleteTarget.id);
    setDeleteTarget(null);
  }

  return (
    <>
      <section className="flex flex-col gap-3">
        <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Agents`}</h2>
        <p className="text-xs text-muted-foreground">{SECTION_HELP}</p>

        <div className="flex flex-col gap-0.5">
          {(agents ?? []).map((a) => {
            const count = countByAgent.get(a.id) ?? 0;
            const deleteDisabled = a.is_system || count > 0;
            const deleteTooltip = a.is_system
              ? "The system agent can't be deleted."
              : `Reassign its ${count} project${count === 1 ? "" : "s"} first.`;

            return (
              <div
                key={a.id}
                className="flex items-center gap-2 rounded-md px-2 py-1.5"
              >
                {/* Set-default radio: a button styled as a radio, keyed off the
                    is_default flag. aria-pressed for screen readers. */}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={a.is_default ? `${a.name} is the default` : `Make ${a.name} the default`}
                      aria-pressed={a.is_default}
                      disabled={a.is_default}
                      onClick={() => setDefaultAgent.mutate(a.id)}
                      className={a.is_default ? "text-amber-500" : "text-muted-foreground"}
                    >
                      {/* a small star/star-off glyph reads as default-or-not */}
                      <span className="text-xs">{a.is_default ? "★" : "☆"}</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="top">
                    {a.is_default ? "Default agent" : "Set as default"}
                  </TooltipContent>
                </Tooltip>

                <span className="min-w-0 flex-1 truncate text-sm">{a.name}</span>
                <Badge variant="secondary" className="font-mono text-[10px]">
                  {a.engine === "claude" ? "Claude" : "Custom"}
                </Badge>

                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Edit ${a.name}`}
                  onClick={() => openEdit(a)}
                >
                  <Pencil className="size-4" />
                </Button>

                {deleteDisabled ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-disabled
                        aria-label={`Delete ${a.name}`}
                        className="text-muted-foreground opacity-60 hover:bg-transparent hover:text-muted-foreground"
                        onClick={(event) => event.preventDefault()}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top">{deleteTooltip}</TooltipContent>
                  </Tooltip>
                ) : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Delete ${a.name}`}
                    onClick={() => setDeleteTarget(a)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                )}
              </div>
            );
          })}
        </div>

        <Button variant="ghost" className="justify-start" onClick={openCreate}>
          <Plus />
          New agent
        </Button>
      </section>

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
                ? `"${deleteTarget.name}" will be removed. Its projects are unaffected — it has none.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleDelete}>
              Delete agent
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

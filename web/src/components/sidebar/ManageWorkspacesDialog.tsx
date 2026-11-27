import { useState } from "react";
import { Pencil, Plus, Trash2, Settings } from "lucide-react";
import { useAgents, useProjects, useWorkspaces } from "@/api/queries";
import { useDeleteWorkspace, useSetWorkspaceDefaultAgent } from "@/api/mutations";
import type { Workspace } from "@/api/types";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
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
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { MonitoringSection } from "@/components/settings/MonitoringSection";
import { WorkspaceNameDialog } from "@/components/sidebar/WorkspaceNameDialog";

export interface ManageWorkspacesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Manage-workspaces hub (D-03/D-05/D-06/D-07): one row per workspace with a
 * rename control and a guarded delete, plus a fast "New workspace" create entry.
 *
 * The delete guards are enforced server-side (Phase 01) + `ON DELETE RESTRICT`;
 * here they are UX affordances only (T-26-08). The non-empty guard's project
 * count is derived client-side from `useProjects()` by `workspace_id` (D-05, no
 * count endpoint). Disabled deletes stay focusable-with-tooltip (never a raw
 * `disabled` that swallows the tooltip) and are muted, never destructive-red.
 */
export function ManageWorkspacesDialog({
  open,
  onOpenChange,
}: ManageWorkspacesDialogProps) {
  const { data: workspaces } = useWorkspaces();
  const { data: projects } = useProjects();
  const deleteWorkspace = useDeleteWorkspace();

  // The name dialog is shared by the create entry and every row's rename.
  const [nameOpen, setNameOpen] = useState(false);
  const [nameMode, setNameMode] = useState<"create" | "rename">("create");
  const [nameTarget, setNameTarget] = useState<Workspace | undefined>(
    undefined,
  );
  // The delete confirm targets one (empty, non-default) workspace at a time.
  const [settingsId, setSettingsId] = useState<number | null>(null);
  const settingsWorkspace = workspaces?.find((workspace) => workspace.id === settingsId);
  const [deleteTarget, setDeleteTarget] = useState<Workspace | null>(null);

  // Per-workspace project count, keyed by workspace_id (D-05) — mirrors the
  // in-memory waitingByProject derivation in ProjectSidebar.
  const countByWorkspace = new Map<number, number>();
  for (const p of projects ?? []) {
    countByWorkspace.set(
      p.workspace_id,
      (countByWorkspace.get(p.workspace_id) ?? 0) + 1,
    );
  }

  // Name-sorted rows (the backend already sorts, but sort defensively so the
  // Manage list order never depends on fetch order).
  const sorted = [...(workspaces ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  function openCreate() {
    setNameMode("create");
    setNameTarget(undefined);
    setNameOpen(true);
  }

  function openRename(ws: Workspace) {
    setNameMode("rename");
    setNameTarget(ws);
    setNameOpen(true);
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    await deleteWorkspace.mutateAsync(deleteTarget.id);
    setDeleteTarget(null);
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>Manage workspaces</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-0.5">
            {sorted.map((ws) => {
              const count = countByWorkspace.get(ws.id) ?? 0;
              const deleteDisabled = ws.is_default || count > 0;
              const deleteTooltip = ws.is_default
                ? "The default workspace can't be deleted."
                : `Move or remove its ${count} project${count === 1 ? "" : "s"} first.`;

              return (
                <div
                  key={ws.id}
                  className="flex items-center gap-2 rounded-md px-2 py-1.5"
                >
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {ws.name}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Rename ${ws.name}`}
                    onClick={() => openRename(ws)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Settings for ${ws.name}`}
                    onClick={() => setSettingsId(ws.id)}
                  >
                    <Settings className="size-4" />
                  </Button>
                  {deleteDisabled ? (
                    // Keep the disabled delete focusable-with-tooltip: aria-disabled
                    // + a click no-op instead of a raw `disabled` (which would swallow
                    // hover/focus and hide the reason). Muted, never destructive-red.
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-disabled
                          aria-label={`Delete ${ws.name}`}
                          className="text-muted-foreground opacity-60 hover:bg-transparent hover:text-muted-foreground"
                          onClick={(event) => event.preventDefault()}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side="top">
                        {deleteTooltip}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Delete ${ws.name}`}
                      onClick={() => setDeleteTarget(ws)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  )}
                </div>
              );
            })}
          </div>

          <Button
            variant="ghost"
            className="justify-start"
            onClick={openCreate}
          >
            <Plus />
            New workspace
          </Button>
        </DialogContent>
      </Dialog>

      <Dialog
        open={settingsWorkspace !== undefined}
        onOpenChange={(open) => {
          if (!open) setSettingsId(null);
        }}
      >
        <DialogContent aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle>{settingsWorkspace?.name} settings</DialogTitle>
          </DialogHeader>
          {settingsWorkspace && (
            <>
              <WorkspaceDefaultAgent workspace={settingsWorkspace} />
              <MonitoringSection workspaceId={settingsWorkspace.id} />
            </>
          )}
        </DialogContent>
      </Dialog>
      <WorkspaceNameDialog
        mode={nameMode}
        workspace={nameTarget}
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
            <AlertDialogTitle>Delete workspace?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget
                ? `"${deleteTarget.name}" will be removed. Its projects are unaffected — it has none.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleDelete}>
              Delete workspace
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function WorkspaceDefaultAgent({ workspace }: { workspace: Workspace }) {
  const { data: agents } = useAgents();
  const update = useSetWorkspaceDefaultAgent();
  const globalDefault = agents?.find((agent) => agent.is_default);

  return (
    <div className="space-y-2">
      <label htmlFor="workspace-default-agent" className="text-xs font-medium">
        Default agent
      </label>
      <Select
        value={workspace.default_agent_id == null ? "global" : String(workspace.default_agent_id)}
        disabled={!agents || update.isPending}
        onValueChange={(value) => update.mutate({
          id: workspace.id,
          default_agent_id: value === "global" ? null : Number(value),
        })}
      >
        <SelectTrigger id="workspace-default-agent" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="global">
            Global default{globalDefault ? ` (${globalDefault.name})` : ""}
          </SelectItem>
          {agents?.map((agent) => (
            <SelectItem key={agent.id} value={String(agent.id)}>{agent.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        Used for new tasks in projects that choose Workspace default agent.
      </p>
      {update.isError && <p role="alert" className="text-xs text-destructive">{update.error.message}</p>}
    </div>
  );
}

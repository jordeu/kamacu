import { useState } from "react";
import { useNavigate } from "react-router";
import { ChevronsUpDown, Plus, Settings } from "lucide-react";
import { useProjects, useWorkspaces } from "@/api/queries";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { WorkspaceNameDialog } from "@/components/sidebar/WorkspaceNameDialog";
import { ManageWorkspacesDialog } from "@/components/sidebar/ManageWorkspacesDialog";

/**
 * Expanded-only workspace switcher (WSNAV-01, D-01/D-02/D-11). The trigger shows
 * the active workspace's name + a chevron; the dropdown lists every workspace
 * (active marked with a ✓ via DropdownMenuCheckboxItem), then a New-workspace and
 * a Manage-workspaces entry. Picking a workspace sets it active and navigates to
 * that workspace's first project (or "/" when the workspace is empty, letting the
 * index route render the workspace-scoped empty state — plan 05).
 *
 * The whole switcher is hidden in the collapsed icon rail via
 * `group-data-[collapsible=icon]:hidden` (the same brand-lockup idiom as
 * ProjectSidebar) — mounted into the sidebar in plan 06.
 */
export function WorkspaceSwitcher() {
  const { activeWorkspaceId, setActiveWorkspaceId } = useActiveWorkspace();
  const { data: workspaces } = useWorkspaces();
  const { data: projects } = useProjects();
  const navigate = useNavigate();

  const [nameOpen, setNameOpen] = useState(false);
  const [manageOpen, setManageOpen] = useState(false);

  const activeName =
    (workspaces ?? []).find((w) => w.id === activeWorkspaceId)?.name ?? "";

  // Name-sorted so the dropdown order is stable regardless of fetch order.
  const sorted = [...(workspaces ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  function handlePick(id: number) {
    setActiveWorkspaceId(id);
    // The projects list is already name-sorted by the backend, so [0] is the
    // first project by name (D-11). Empty workspace → index route empty state.
    const first = (projects ?? []).filter((p) => p.workspace_id === id)[0];
    navigate(first ? `/projects/${first.id}` : "/");
  }

  return (
    <div className="px-2 group-data-[collapsible=icon]:hidden">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={`Workspace: ${activeName}`}
            className="flex min-h-7 w-full items-center gap-2 rounded-md px-3 text-sm font-medium outline-none transition-colors hover:bg-sidebar-accent focus-visible:bg-sidebar-accent data-[state=open]:bg-sidebar-accent"
          >
            <span className="min-w-0 flex-1 truncate text-left">
              {activeName}
            </span>
            <ChevronsUpDown
              className="size-4 shrink-0 text-muted-foreground"
              aria-hidden
            />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="bottom" align="start">
          {sorted.map((ws) => (
            <DropdownMenuCheckboxItem
              key={ws.id}
              checked={ws.id === activeWorkspaceId}
              onSelect={() => handlePick(ws.id)}
            >
              {ws.name}
            </DropdownMenuCheckboxItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setNameOpen(true)}>
            <Plus />
            New workspace
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setManageOpen(true)}>
            <Settings />
            Manage workspaces…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <WorkspaceNameDialog
        mode="create"
        open={nameOpen}
        onOpenChange={setNameOpen}
      />
      <ManageWorkspacesDialog open={manageOpen} onOpenChange={setManageOpen} />
    </div>
  );
}

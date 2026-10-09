import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { MoreHorizontal } from "lucide-react";
import { ApiError } from "@/api/client";
import { useDeleteProject, useMoveProject } from "@/api/mutations";
import { useWorkspaces } from "@/api/queries";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";
import type { Project } from "@/api/types";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarMenuAction } from "@/components/ui/sidebar";
import { RenameProjectDialog } from "@/components/sidebar/RenameProjectDialog";
import { ProjectSettingsDialog } from "@/components/sidebar/ProjectSettingsDialog";

export interface ProjectMenuProps {
  project: Project;
}

export function ProjectMenu({ project }: ProjectMenuProps) {
  const [renameOpen, setRenameOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const deleteProject = useDeleteProject();
  const moveProject = useMoveProject();
  const { data: workspaces } = useWorkspaces();
  const { setActiveWorkspaceId } = useActiveWorkspace();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { projectId } = useParams();

  async function handleDelete() {
    const isCurrent = projectId === String(project.id);
    try {
      await deleteProject.mutateAsync(project.id);
    } catch {
      // Keep the confirmation open so the mutation error is visible.
      return;
    }
    setDeleteOpen(false);
    if (isCurrent) {
      // Refetch before navigating so the index redirect never targets the
      // just-deleted project from a stale cache.
      await queryClient.refetchQueries({ queryKey: ["projects"] });
      navigate("/");
    }
  }

  // Transfer + follow-the-project (WSPROJ-01, D-08/D-09). Move the project to the
  // target workspace; if it is the currently-open project, flip the active
  // workspace to the target so the view follows it (the project stays selected
  // and visible; the localStorage active-workspace key is updated by the setter).
  // A non-open project just drops out of the filtered sidebar list.
  function handleMove(targetWorkspaceId: number) {
    if (targetWorkspaceId === project.workspace_id) return;
    moveProject.mutate({ id: project.id, workspace_id: targetWorkspaceId });
    if (projectId === String(project.id)) {
      setActiveWorkspaceId(targetWorkspaceId);
    }
  }

  // Name-sorted so the submenu order is stable regardless of fetch order
  // (mirrors WorkspaceSwitcher).
  const sortedWorkspaces = [...(workspaces ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <SidebarMenuAction
            showOnHover
            aria-label={`Project menu: ${project.name}`}
          >
            <MoreHorizontal className="size-4" />
          </SidebarMenuAction>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="bottom" align="start">
          <DropdownMenuItem onSelect={() => setRenameOpen(true)}>
            Rename
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setSettingsOpen(true)}>
            Project settings
          </DropdownMenuItem>
          {/* WSPROJ-01 / D-08: transfer submenu — radio items for every
              workspace, the project's current workspace checked + disabled.
              Selecting another workspace moves the project (and, if it is the
              open one, the view follows it — D-09). */}
          <DropdownMenuSub>
            <DropdownMenuSubTrigger aria-label="Move to workspace">
              Move to
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuRadioGroup value={String(project.workspace_id)}>
                {sortedWorkspaces.map((ws) => (
                  <DropdownMenuRadioItem
                    key={ws.id}
                    value={String(ws.id)}
                    disabled={ws.id === project.workspace_id}
                    onSelect={() => handleMove(ws.id)}
                  >
                    {ws.name}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuItem
            variant="destructive"
            onSelect={() => {
              deleteProject.reset();
              setDeleteOpen(true);
            }}
          >
            Delete project
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <RenameProjectDialog
        project={project}
        open={renameOpen}
        onOpenChange={setRenameOpen}
      />

      <ProjectSettingsDialog
        project={project}
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
      />

      <AlertDialog open={deleteOpen} onOpenChange={(open) => {
        if (!deleteProject.isPending) setDeleteOpen(open);
      }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete project?</AlertDialogTitle>
            <AlertDialogDescription>
              {project.managed
                ? `"${project.name}" and all its tasks, managed checkout, and task worktrees will be removed. Uncommitted changes, unpushed commits, stashes, or running sessions can block deletion.`
                : `"${project.name}" and all its tasks will be removed from Kamacu. The repository on disk is untouched.`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteProject.error && (
            <div role="alert" className="text-sm text-destructive">
              <p>{deleteProject.error.message || "Could not delete the project. Please try again."}</p>
              {deleteProject.error instanceof ApiError && deleteProject.error.reasons?.length ? (
                <ul className="mt-2 list-disc pl-4">
                  {deleteProject.error.reasons.map((reason, index) => (
                    <li key={index}>
                      {reason.target}: {({ uncommitted: "uncommitted changes", unpushed: "unpushed commits or unavailable default branch", stash: "stashed changes", sessions: "running sessions" } as Record<string, string>)[reason.kind] ?? reason.kind}
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteProject.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={deleteProject.isPending} onClick={(event) => {
              event.preventDefault();
              void handleDelete();
            }}>
              {deleteProject.isPending ? "Deleting…" : "Delete project"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

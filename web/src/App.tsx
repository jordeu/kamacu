import { useEffect, useState, type ReactNode } from "react";
import { Navigate, Route, Routes, useParams } from "react-router";
import { AppLayout } from "@/components/layout/AppLayout";
import { useProjects, useWorkspaces } from "@/api/queries";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";
import { Button } from "@/components/ui/button";
import { AddProjectDialog } from "@/components/sidebar/AddProjectDialog";
import BoardPage from "@/pages/BoardPage";
import SettingsPage from "@/pages/SettingsPage";
import TaskPage from "@/pages/TaskPage";
import TerminalPage from "@/pages/TerminalPage";

function RedirectToFirstProject() {
  const { data: projects, isLoading } = useProjects();
  const { data: workspaces } = useWorkspaces();
  const { activeWorkspaceId } = useActiveWorkspace();
  const [addOpen, setAddOpen] = useState(false);

  // Wait until projects have loaded AND the active workspace has resolved
  // (activeWorkspaceId is null only while useWorkspaces() is still loading).
  if (isLoading || activeWorkspaceId === null) return null;

  // Only the active workspace's projects are candidates for the index redirect
  // (D-11/D-13) — do NOT redirect to a project in another workspace.
  const wsProjects = (projects ?? []).filter(
    (p) => p.workspace_id === activeWorkspaceId,
  );
  if (wsProjects.length > 0) {
    // First-by-name: the backend lists projects ORDER BY name COLLATE NOCASE,
    // so [0] is the alphabetically-first project in this workspace (D-11).
    return <Navigate to={`/projects/${wsProjects[0].id}`} replace />;
  }

  // Workspace-scoped empty state (D-13): never a global "No projects yet" while
  // other workspaces still hold projects. Name resolved from useWorkspaces().
  const workspaceName =
    workspaces?.find((w) => w.id === activeWorkspaceId)?.name ?? "";

  return (
    <div className="flex h-full items-center justify-center">
      <div className="flex flex-col items-center gap-4 py-8 text-center">
        <div className="flex flex-col items-center gap-1">
          <h1 className="text-xl font-medium">
            No projects in {workspaceName} yet
          </h1>
          <p className="text-muted-foreground">
            Add a project to this workspace to get a board.
          </p>
        </div>
        <Button onClick={() => setAddOpen(true)}>Add project</Button>
      </div>
      <AddProjectDialog open={addOpen} onOpenChange={setAddOpen} />
    </div>
  );
}

/**
 * URL-wins deep-link sync (D-14): the URL's project decides the active
 * workspace on reload/deep-link. When the URL project exists and belongs to a
 * different workspace than the (localStorage-restored) active one, flip the
 * active workspace to the project's own workspace so the viewed project is
 * always visible under its workspace's sidebar filter. Workspace never enters
 * the URL — this only mutates the localStorage-backed active-workspace state.
 * Renders the underlying page unchanged.
 */
function BoardWorkspaceSync({ children }: { children: ReactNode }) {
  const params = useParams();
  const projectId = Number(params.projectId);
  const { data: projects } = useProjects();
  const { activeWorkspaceId, setActiveWorkspaceId } = useActiveWorkspace();

  const projectWorkspaceId = projects?.find(
    (p) => p.id === projectId,
  )?.workspace_id;

  useEffect(() => {
    // Only reconcile once both sides have resolved: the URL project exists in
    // useProjects() (T-26-07 — never trust a workspace_id we can't see) and the
    // active workspace has settled (non-null). URL wins when they differ.
    if (
      projectWorkspaceId !== undefined &&
      activeWorkspaceId !== null &&
      projectWorkspaceId !== activeWorkspaceId
    ) {
      setActiveWorkspaceId(projectWorkspaceId);
    }
  }, [projectWorkspaceId, activeWorkspaceId, setActiveWorkspaceId]);

  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<RedirectToFirstProject />} />
        <Route
          path="/projects/:projectId"
          element={
            <BoardWorkspaceSync>
              <BoardPage />
            </BoardWorkspaceSync>
          }
        />
        <Route
          path="/projects/:projectId/tasks/:taskId"
          element={
            <BoardWorkspaceSync>
              <TaskPage />
            </BoardWorkspaceSync>
          }
        />
        <Route path="/settings" element={<SettingsPage />} />
        {/* Dev/debug surface for the terminal engine (D-12) — reached by URL */}
        <Route path="/terminal" element={<TerminalPage />} />
      </Route>
    </Routes>
  );
}

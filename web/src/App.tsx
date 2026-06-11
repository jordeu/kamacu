import { useState } from "react";
import { Navigate, Route, Routes } from "react-router";
import { AppLayout } from "@/components/layout/AppLayout";
import { useProjects } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { AddProjectDialog } from "@/components/sidebar/AddProjectDialog";
import BoardPage from "@/pages/BoardPage";
import SettingsPage from "@/pages/SettingsPage";
import TaskPage from "@/pages/TaskPage";
import TerminalPage from "@/pages/TerminalPage";

function RedirectToFirstProject() {
  const { data: projects, isLoading } = useProjects();
  const [addOpen, setAddOpen] = useState(false);

  if (isLoading) return null;
  if (projects && projects.length > 0) {
    return <Navigate to={`/projects/${projects[0].id}`} replace />;
  }

  return (
    <div className="flex h-full items-center justify-center">
      <div className="flex flex-col items-center gap-4 py-8 text-center">
        <div className="flex flex-col items-center gap-1">
          <h1 className="text-xl font-medium">No projects yet</h1>
          <p className="text-muted-foreground">
            Point Kangent at a local git repository to get a board.
          </p>
        </div>
        <Button onClick={() => setAddOpen(true)}>Add project</Button>
      </div>
      <AddProjectDialog open={addOpen} onOpenChange={setAddOpen} />
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<RedirectToFirstProject />} />
        <Route path="/projects/:projectId" element={<BoardPage />} />
        <Route
          path="/projects/:projectId/tasks/:taskId"
          element={<TaskPage />}
        />
        <Route path="/settings" element={<SettingsPage />} />
        {/* Dev/debug surface for the terminal engine (D-12) — reached by URL */}
        <Route path="/terminal" element={<TerminalPage />} />
      </Route>
    </Routes>
  );
}

import { Navigate, Route, Routes } from "react-router";
import { AppLayout } from "@/components/layout/AppLayout";
import { useProjects } from "@/api/queries";
import BoardPage from "@/pages/BoardPage";
import TaskPage from "@/pages/TaskPage";

function RedirectToFirstProject() {
  const { data: projects, isLoading } = useProjects();
  if (isLoading) return null;
  if (projects && projects.length > 0) {
    return <Navigate to={`/projects/${projects[0].id}`} replace />;
  }
  // No projects yet — plan 01-04 fills in the empty state.
  return null;
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
      </Route>
    </Routes>
  );
}

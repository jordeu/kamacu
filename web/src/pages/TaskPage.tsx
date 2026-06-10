import { useParams } from "react-router";

export default function TaskPage() {
  const { projectId, taskId } = useParams();
  // Plan 01-06 replaces this stub with the task view.
  return (
    <div className="p-4 text-muted-foreground">
      Task {taskId} in project {projectId}
    </div>
  );
}

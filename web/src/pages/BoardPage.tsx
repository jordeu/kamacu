import { useParams } from "react-router";

export default function BoardPage() {
  const { projectId } = useParams();
  // Plan 01-05 replaces this stub with the kanban board.
  return <div className="p-4 text-muted-foreground">Board {projectId}</div>;
}

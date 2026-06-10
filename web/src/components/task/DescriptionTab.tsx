import type { Task } from "@/api/types";

// Stub — replaced with the markdown view/edit toggle in Task 2 of plan 01-06.
export function DescriptionTab({ task }: { task: Task; projectId: number }) {
  return (
    <div className="text-muted-foreground whitespace-pre-wrap">
      {task.description || "No description."}
    </div>
  );
}

import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useNavigate } from "react-router";
import { cn } from "@/lib/utils";
import type { Task } from "@/api/types";

function CardShell({ task, className }: { task: Task; className?: string }) {
  return (
    <div
      className={cn(
        "rounded-md border border-border bg-card px-3 py-2",
        className,
      )}
    >
      <span className="line-clamp-2 text-sm font-medium">{task.title}</span>
    </div>
  );
}

/** Rendered inside DragOverlay — visual clone with lift feedback, no sortable wiring. */
export function TaskCardOverlay({ task }: { task: Task }) {
  return (
    <CardShell task={task} className="scale-[1.02] cursor-grabbing shadow-lg" />
  );
}

export function TaskCard({ task }: { task: Task }) {
  const navigate = useNavigate();
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: task.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      onClick={() =>
        navigate(`/projects/${task.project_id}/tasks/${task.id}`)
      }
      className={cn(
        "cursor-grab rounded-md border border-border bg-card px-3 py-2 hover:bg-[#27272a]",
        isDragging && "opacity-40",
      )}
    >
      <span className="line-clamp-2 text-sm font-medium">{task.title}</span>
    </div>
  );
}

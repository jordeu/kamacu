import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useNavigate } from "react-router";
import { cn } from "@/lib/utils";
import { useAgentStatuses } from "@/api/agents";
import type { AgentStatusEntry } from "@/api/agents";
import { StatusDot } from "@/components/StatusDot";
import type { Task } from "@/api/types";

/** Shared inner layout for CardShell, TaskCard, and TaskCardOverlay.
 *  No agent entry → title renders exactly as before: no dot, no reserved
 *  gutter (UI-SPEC: dotless cards must not shift layout). */
function CardRow({
  task,
  entry,
}: {
  task: Task;
  entry: AgentStatusEntry | undefined;
}) {
  return (
    <div className="flex gap-2">
      <span className="line-clamp-2 flex-1 text-sm font-medium">
        {task.title}
      </span>
      {entry && <StatusDot entry={entry} className="mt-[6px]" />}
    </div>
  );
}

function CardShell({ task, className }: { task: Task; className?: string }) {
  // D-48: AGENT entry only — bash sessions never appear in /api/agents/status.
  // React Query dedupes per-card hooks on the shared ["agent-statuses"] key.
  const { data } = useAgentStatuses();
  const entry = data?.find((e) => e.taskId === task.id);

  return (
    <div
      className={cn(
        "rounded-md border bg-card px-3 py-2",
        // D-44: waiting swaps the border hue, nothing else changes.
        entry?.status === "waiting" ? "border-amber-400/40" : "border-border",
        className,
      )}
    >
      <CardRow task={task} entry={entry} />
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
  const { data } = useAgentStatuses();
  const entry = data?.find((e) => e.taskId === task.id);
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
        "cursor-grab rounded-md border bg-card px-3 py-2 hover:bg-[#27272a]",
        entry?.status === "waiting" ? "border-amber-400/40" : "border-border",
        isDragging && "opacity-40",
      )}
    >
      <CardRow task={task} entry={entry} />
    </div>
  );
}

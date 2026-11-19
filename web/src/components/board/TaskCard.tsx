import type { ComponentProps, ReactNode } from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useNavigate } from "react-router";
import { cn } from "@/lib/utils";
import { useAgentStatuses } from "@/api/agents";
import { StatusDot } from "@/components/StatusDot";
import { TaskPRLinks } from "@/components/task/TaskPRLinks";
import { SnoozeTask } from "@/components/task/SnoozeTask";
import type { Task } from "@/api/types";

interface TaskCardProps {
  task: Task;
  footer?: ReactNode;
  sortable?: boolean;
  activePRsOnly?: boolean;
  "aria-label"?: string;
}

/** Shared appearance for project cards, overview cards and the drag overlay. */
function CardShell({
  task,
  footer,
  activePRsOnly = false,
  overlay = false,
  className,
  ...props
}: Omit<ComponentProps<"div">, "children"> & {
  task: Task;
  footer?: ReactNode;
  activePRsOnly?: boolean;
  overlay?: boolean;
}) {
  const { data } = useAgentStatuses();
  const entry = data?.find((agent) => agent.taskId === task.id);
  return (
    <div
      {...props}
      className={cn(
        "rounded-md border bg-card px-3 py-2",
        entry?.status === "waiting" ? "border-amber-400/40" : "border-border",
        className,
      )}
    >
      <div className="flex gap-2">
        <span className="line-clamp-2 min-w-0 flex-1 text-sm font-medium [overflow-wrap:anywhere]">
          {task.title}
        </span>
        {entry && <StatusDot entry={entry} className="mt-[6px]" />}
      </div>
      <TaskPRLinks
        task={task}
        className="mt-1"
        compact
        activeOnly={activePRsOnly}
      />
      {!overlay && task.snoozed_until && <SnoozeTask task={task} compact />}
      {footer && <div className="mt-2">{footer}</div>}
    </div>
  );
}

export function TaskCardOverlay({ task }: { task: Task }) {
  return (
    <CardShell
      task={task}
      overlay
      className="scale-[1.02] cursor-grabbing shadow-lg"
    />
  );
}

/** Overview cards open existing tasks; project boards retain their drag sensors. */
export function TaskCard({ sortable = true, ...props }: TaskCardProps) {
  return sortable ? (
    <SortableTaskCard {...props} />
  ) : (
    <StaticTaskCard {...props} />
  );
}

function StaticTaskCard(props: TaskCardProps) {
  const navigate = useNavigate();
  const open = () =>
    navigate(`/projects/${props.task.project_id}/tasks/${props.task.id}`);
  return (
    <CardShell
      {...props}
      role="button"
      tabIndex={0}
      aria-label={props["aria-label"] ?? props.task.title}
      onClick={open}
      onKeyDown={(event) => {
        if (
          event.target === event.currentTarget &&
          (event.key === "Enter" || event.key === " ")
        ) {
          event.preventDefault();
          open();
        }
      }}
      className="cursor-pointer hover:bg-[#27272a] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    />
  );
}

function SortableTaskCard(props: TaskCardProps) {
  const navigate = useNavigate();
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: props.task.id });
  return (
    <CardShell
      {...props}
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      {...attributes}
      {...listeners}
      aria-label={props["aria-label"] ?? props.task.title}
      onClick={() =>
        navigate(`/projects/${props.task.project_id}/tasks/${props.task.id}`)
      }
      className={cn(
        "cursor-grab hover:bg-[#27272a]",
        isDragging && "opacity-40",
      )}
    />
  );
}

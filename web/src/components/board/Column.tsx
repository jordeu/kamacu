import type { ReactNode } from "react";
import { useDroppable } from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { cn } from "@/lib/utils";
import { STATUS_LABELS, type Status, type Task } from "@/api/types";
import { TaskCard } from "./TaskCard";

interface ColumnProps {
  status: Status;
  tasks: Task[];
  /** Optional content rendered above the cards (To Do's quick-add plugs in here). */
  topSlot?: ReactNode;
}

export function Column({ status, tasks, topSlot }: ColumnProps) {
  // Namespaced droppable id: empty columns accept drops and column ids never
  // collide with task ids.
  const { setNodeRef, isOver } = useDroppable({ id: `column:${status}` });

  return (
    <div className="flex min-h-0 min-w-[260px] flex-1 flex-col">
      <div className="flex items-baseline gap-2 px-3 pb-2">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {STATUS_LABELS[status]}
        </span>
        <span className="text-xs font-medium text-muted-foreground">
          {tasks.length}
        </span>
      </div>
      <SortableContext
        items={tasks.map((t) => t.id)}
        strategy={verticalListSortingStrategy}
      >
        <div
          ref={setNodeRef}
          className={cn(
            "flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto rounded-md bg-[#101013] p-3",
            isOver && "ring-1 ring-blue-500",
          )}
        >
          {topSlot}
          {tasks.map((task) => (
            <TaskCard key={task.id} task={task} />
          ))}
        </div>
      </SortableContext>
    </div>
  );
}

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
  /** Optional content rendered above the cards. */
  topSlot?: ReactNode;
}

export function Column({ status, tasks, topSlot }: ColumnProps) {
  // Namespaced droppable id: empty columns accept drops and column ids never
  // collide with task ids.
  const { setNodeRef, isOver } = useDroppable({ id: `column:${status}` });

  const recurrent = status === "todo" ? tasks.filter((t) => t.recurrent) : [];
  const ordinary = status === "todo" ? tasks.filter((t) => !t.recurrent) : tasks;

  return (
    <div className="flex min-h-0 min-w-[260px] flex-1 flex-col">
      <div className="flex items-baseline gap-2 px-3 pb-2">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {STATUS_LABELS[status]}
        </span>
        <span className="text-xs font-medium text-muted-foreground">
          {ordinary.length}
        </span>
      </div>
      <SortableContext
        items={[...ordinary, ...recurrent].map((t) => t.id)}
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
          {ordinary.map((task) => (
            <TaskCard key={task.id} task={task} />
          ))}
          {recurrent.length > 0 && (
            <>
              <div className="mt-2 flex items-baseline gap-2 border-t border-border pt-2">
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Recurrent tasks
                </span>
                <span className="text-xs font-medium text-muted-foreground">
                  {recurrent.length}
                </span>
              </div>
              {recurrent.map((task) => (
                <TaskCard key={task.id} task={task} />
              ))}
            </>
          )}
        </div>
      </SortableContext>
    </div>
  );
}

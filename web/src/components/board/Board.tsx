import { useEffect, useState } from "react";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { sortableKeyboardCoordinates } from "@dnd-kit/sortable";
import { STATUSES, type Status, type Task } from "@/api/types";
import { Column } from "./Column";
import { TaskCardOverlay } from "./TaskCard";

function groupTasks(tasks: Task[]): Record<Status, Task[]> {
  const columns: Record<Status, Task[]> = {
    todo: [],
    in_progress: [],
    in_review: [],
    done: [],
  };
  for (const task of [...tasks].sort((a, b) => a.position - b.position)) {
    columns[task.status].push(task);
  }
  return columns;
}

interface BoardProps {
  tasks: Task[];
  projectId: number;
}

export function Board({ tasks, projectId }: BoardProps) {
  void projectId; // used by Task 3 (quick-add wiring)

  const sensors = useSensors(
    // CRITICAL: 5px activation distance lets plain clicks navigate to the task
    // route while press-and-move starts a drag (Pitfall 4).
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const [activeTask, setActiveTask] = useState<Task | null>(null);
  // Local mirror of board order — dnd-kit needs synchronous reorders.
  const [columns, setColumns] = useState<Record<Status, Task[]>>(() =>
    groupTasks(tasks),
  );

  // Single derivation path: columns derive ONLY from query data, and the
  // derivation is FROZEN while a drag is active so a query invalidation
  // mid-drag never clobbers the local order (Pitfall 6).
  useEffect(() => {
    if (activeTask === null) {
      setColumns(groupTasks(tasks));
    }
  }, [tasks, activeTask]);

  function handleDragStart({ active }: DragStartEvent) {
    const task = tasks.find((t) => t.id === Number(active.id)) ?? null;
    setActiveTask(task);
  }

  function handleDragOver(event: DragOverEvent) {
    // Body completed in Task 2 (cross-column local preview).
    void event;
  }

  function handleDragEnd(event: DragEndEvent) {
    // Body completed in Task 2 (afterId computation + move mutation).
    void event;
    setActiveTask(null);
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
    >
      <div className="flex h-full min-h-0 flex-1 gap-4 overflow-x-auto px-6 pb-6">
        {STATUSES.map((status) => (
          <Column key={status} status={status} tasks={columns[status]} />
        ))}
      </div>
      <DragOverlay>
        {activeTask ? <TaskCardOverlay task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}

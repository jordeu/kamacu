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
import { arrayMove, sortableKeyboardCoordinates } from "@dnd-kit/sortable";
import { useMoveTask } from "@/api/mutations";
import { STATUSES, type Status, type Task } from "@/api/types";
import { CleanupWorktreeDialog } from "@/components/task/CleanupWorktreeDialog";
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
  /** useTasks(projectId) query data, passed down from BoardPage — the single derivation source for columns. */
  tasks: Task[];
  projectId: number;
}

export function Board({ tasks, projectId }: BoardProps) {
  const moveTask = useMoveTask(projectId);

  const sensors = useSensors(
    // CRITICAL: 5px activation distance lets plain clicks navigate to the task
    // route while press-and-move starts a drag (Pitfall 4).
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const [activeTask, setActiveTask] = useState<Task | null>(null);
  // D-31: a task that just landed in Done with a live worktree gets the
  // cleanup offer — set from the move's onSuccess, AFTER the move persists.
  const [cleanupTask, setCleanupTask] = useState<Task | null>(null);
  // Drag-origin slot, recorded at drag start for same-position no-op detection.
  const [origin, setOrigin] = useState<{
    status: Status;
    index: number;
  } | null>(null);
  // Local mirror of board order — dnd-kit needs synchronous reorders.
  const [columns, setColumns] = useState<Record<Status, Task[]>>(() =>
    groupTasks(tasks),
  );

  // Single derivation path: columns derive ONLY from query data, and the
  // derivation is FROZEN while a drag is active so a query invalidation
  // mid-drag never clobbers the local order (Pitfall 6). The optimistic cache
  // write in useMoveTask.onMutate keeps the post-drop order stable until
  // onSettled invalidation brings server truth; on error the mutation's
  // snapshot rollback restores the cache and the derived columns revert.
  useEffect(() => {
    if (activeTask === null && !moveTask.isPending) {
      setColumns(groupTasks(tasks));
    }
  }, [tasks, activeTask, moveTask.isPending]);

  function findColumnOf(taskId: number): Status | null {
    for (const status of STATUSES) {
      if (columns[status].some((t) => t.id === taskId)) return status;
    }
    return null;
  }

  /** Resolve an `over` id to a target column: `column:*` ids directly, task ids via their current local column. */
  function resolveTargetColumn(overId: string | number): Status | null {
    if (typeof overId === "string" && overId.startsWith("column:")) {
      return overId.slice("column:".length) as Status;
    }
    return findColumnOf(Number(overId));
  }

  function handleDragStart({ active }: DragStartEvent) {
    const activeId = Number(active.id);
    const status = findColumnOf(activeId);
    if (status === null) return;
    const index = columns[status].findIndex((t) => t.id === activeId);
    setActiveTask(columns[status][index] ?? null);
    setOrigin({ status, index });
  }

  // LOCAL state only — no network. Gives live cross-column preview.
  function handleDragOver({ active, over }: DragOverEvent) {
    if (!over) return;
    const activeId = Number(active.id);
    const from = findColumnOf(activeId);
    const to = resolveTargetColumn(over.id);
    if (from === null || to === null || from === to) return;

    setColumns((prev) => {
      const fromTasks = [...prev[from]];
      const index = fromTasks.findIndex((t) => t.id === activeId);
      if (index === -1) return prev;
      const [moved] = fromTasks.splice(index, 1);

      const toTasks = [...prev[to]];
      // Insert at the hovered card's index, or at the end for bare-column hover.
      let insertAt = toTasks.length;
      if (!(typeof over.id === "string" && over.id.startsWith("column:"))) {
        const overIndex = toTasks.findIndex((t) => t.id === Number(over.id));
        if (overIndex !== -1) insertAt = overIndex;
      }
      toTasks.splice(insertAt, 0, { ...moved, status: to });

      return { ...prev, [from]: fromTasks, [to]: toTasks };
    });
  }

  function handleDragEnd({ active, over }: DragEndEvent) {
    const activeId = Number(active.id);

    const finish = () => {
      // Clearing activeTask unfreezes derivation.
      setActiveTask(null);
      setOrigin(null);
    };

    // Dropped outside any droppable — derivation snaps local state back to
    // query truth.
    if (!over) {
      finish();
      return;
    }

    const status = findColumnOf(activeId);
    if (status === null) {
      finish();
      return;
    }

    // Final within-column index: cross-column placement already happened in
    // handleDragOver; same-column reorders resolve here via arrayMove.
    const columnTasks = columns[status];
    const oldIndex = columnTasks.findIndex((t) => t.id === activeId);
    let newIndex = oldIndex;
    if (!(typeof over.id === "string" && over.id.startsWith("column:"))) {
      const overIndex = columnTasks.findIndex((t) => t.id === Number(over.id));
      if (overIndex !== -1) newIndex = overIndex;
    }
    const finalTasks = arrayMove(columnTasks, oldIndex, newIndex);
    const finalIndex = finalTasks.findIndex((t) => t.id === activeId);
    setColumns((prev) => ({ ...prev, [status]: finalTasks }));

    // Same-position no-op (same column AND same index as drag origin) — skip
    // the mutation entirely.
    if (origin !== null && origin.status === status && origin.index === finalIndex) {
      finish();
      return;
    }

    // afterId = card directly ABOVE the final index; null at the top. Never
    // the moving task itself (it sits at finalIndex, not finalIndex - 1).
    const afterId = finalIndex === 0 ? null : finalTasks[finalIndex - 1].id;
    moveTask.mutate(
      { id: activeId, status, afterId },
      {
        // Call-site callback (the shared hook stays untouched): the cleanup
        // offer fires AFTER the move persists — the server response Task
        // carries the worktree fields, so no stale-cache predicate. It never
        // blocks or rolls back the move; declining keeps the worktree and
        // re-entering Done prompts again (no suppression state).
        onSuccess: (t) => {
          if (status === "done" && t.worktree_path) setCleanupTask(t);
        },
      },
    );
    finish();
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
          <Column
            key={status}
            status={status}
            tasks={columns[status]}
            projectId={projectId}
          />
        ))}
      </div>
      <DragOverlay>
        {activeTask ? <TaskCardOverlay task={activeTask} /> : null}
      </DragOverlay>
      {cleanupTask && (
        <CleanupWorktreeDialog
          open
          onOpenChange={(o) => {
            if (!o) setCleanupTask(null);
          }}
          taskId={cleanupTask.id}
          taskTitle={cleanupTask.title}
          projectId={projectId}
          trigger="done"
        />
      )}
    </DndContext>
  );
}

import type { Status, Task } from "@/api/types";

export function taskSection(task: Task): "ordinary" | "recurrent" | "snoozed" {
  return task.snoozed_until ? "snoozed" : task.recurrent ? "recurrent" : "ordinary";
}

/** Keep TODO sections contiguous; snoozed tasks wake soonest first. */
export function orderColumnTasks(status: Status, tasks: Task[]): Task[] {
  if (status !== "todo") return tasks;
  return [
    ...tasks.filter((task) => taskSection(task) === "ordinary"),
    ...tasks.filter((task) => taskSection(task) === "recurrent"),
    ...tasks.filter((task) => taskSection(task) === "snoozed")
      .sort((a, b) => Date.parse(a.snoozed_until!) - Date.parse(b.snoozed_until!) || a.id - b.id),
  ];
}

/** Dragging within TODO does not change recurrence or snooze state. */
export function sameTaskSection(status: Status, a: Task, b: Task): boolean {
  return status !== "todo" || taskSection(a) === taskSection(b);
}

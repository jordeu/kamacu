import type { Status, Task } from "@/api/types";

/** Keep TODO's sections contiguous without changing order within either one. */
export function orderColumnTasks(status: Status, tasks: Task[]): Task[] {
  if (status !== "todo") return tasks;
  return [...tasks.filter((t) => !t.recurrent), ...tasks.filter((t) => t.recurrent)];
}

/** A drop on the other TODO section never changes a task's recurrence. */
export function sameTaskSection(status: Status, a: Task, b: Task): boolean {
  return status !== "todo" || a.recurrent === b.recurrent;
}

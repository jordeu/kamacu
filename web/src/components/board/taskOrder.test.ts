import { describe, expect, it } from "vitest";
import type { Task } from "@/api/types";
import { orderColumnTasks, sameTaskSection } from "./taskOrder";

function task(id: number, recurrent: boolean): Task {
  return {
    id, recurrent, project_id: 1, title: `Task ${id}`, description: "",
    status: "todo", position: id, created_at: "", updated_at: "",
    branch: null, worktree_path: null, worktree_error: null,
    source: "manual", pr_number: null, pr_base_ref: null,
  };
}

describe("TODO recurrence sections", () => {
  it("groups interleaved server positions while preserving each section's order", () => {
    const tasks = [task(4, true), task(3, false), task(2, true), task(1, false)];
    expect(orderColumnTasks("todo", tasks).map((t) => t.id)).toEqual([3, 1, 4, 2]);
    expect(orderColumnTasks("done", tasks)).toEqual(tasks);
  });

  it("keeps a cross-column drop in its recurrence section", () => {
    const ordinary = task(1, false);
    const recurring = task(2, true);
    const moved = task(3, true);
    expect(orderColumnTasks("todo", [moved, ordinary, recurring]).map((t) => t.id)).toEqual([1, 3, 2]);
    expect(sameTaskSection("todo", moved, ordinary)).toBe(false);
    expect(sameTaskSection("todo", moved, recurring)).toBe(true);
    expect(sameTaskSection("in_progress", moved, ordinary)).toBe(true);
    expect(moved.recurrent).toBe(true);
  });
});

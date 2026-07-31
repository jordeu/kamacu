import { Link } from "react-router";
import type { ActivityTask } from "@/api/types";
import { formatAgo } from "@/lib/time";

/**
 * Phase 11 — the tasks-done list (CONTEXT D-13, D-15; TASKS-02/03).
 *
 * Tasks arrive already sorted `doneAt DESC` from the server SQL (verified —
 * activity_test.go:180-257), so this component does NOT re-sort (Phase 11
 * RESEARCH Pitfall 5 only applies to reviews). It buckets the list by
 * projectId via `groupByProject` and renders each group under a project-name
 * subheading (NOT uppercase — the uppercase tracking-wide treatment is
 * reserved for top-level section headings per UI-SPEC).
 *
 * Clicking an entry navigates to that task's view (D-15) — `activityTask`
 * carries both `id` and `projectId`, so no lookup is needed.
 */

interface ProjectGroup<T> {
  projectId: number;
  projectName: string;
  entries: T[];
}

/**
 * Bucket rows by projectId, preserving arrival order within each group.
 * Used by both ActivityList (tasks) and ReviewsList (reviews, post-sort).
 */
export function groupByProject<
  T extends { projectId: number; projectName: string },
>(rows: T[]): ProjectGroup<T>[] {
  const map = new Map<number, ProjectGroup<T>>();
  for (const r of rows) {
    let g = map.get(r.projectId);
    if (!g) {
      g = { projectId: r.projectId, projectName: r.projectName, entries: [] };
      map.set(r.projectId, g);
    }
    g.entries.push(r);
  }
  return [...map.values()];
}

export function ActivityList({ tasks }: { tasks: ActivityTask[] }) {
  const now = Date.now();
  const groups = groupByProject(tasks);

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Tasks completed`}</h2>
      {groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">{`No tasks completed in this window.`}</p>
      ) : (
        <div className="flex flex-col gap-3">
          {groups.map((group) => (
            <div key={group.projectId} className="flex flex-col gap-1">
              <div className="text-xs font-medium text-muted-foreground">
                {group.projectName}
              </div>
              {group.entries.map((task) => (
                <Link
                  key={task.id}
                  to={`/projects/${task.projectId}/tasks/${task.id}`}
                  className="flex items-baseline gap-2 text-sm hover:text-foreground"
                >
                  <span className="flex-1 truncate text-muted-foreground hover:text-foreground">
                    {task.title}
                  </span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {formatAgo(task.doneAt, now)}
                  </span>
                </Link>
              ))}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

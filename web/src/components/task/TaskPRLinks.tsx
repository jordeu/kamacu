import { ExternalLink } from "lucide-react";
import { useProjectPRStatuses } from "@/api/prStatus";
import type { Task, TaskPRLink } from "@/api/types";
import { cn } from "@/lib/utils";
import { PRLifecycleIcon, PRStatusOverview } from "./PRStatusOverview";

/** Secondary task metadata, shared by the board and task header. */
export function TaskPRLinks({ task, className }: { task: Task; className?: string }) {
  const links = task.pr_links?.filter((link) => link.role !== "incidental") ?? [];
  if (!links.length) return null;
  return <LinkedTaskPRLinks task={task} links={links} className={className} />;
}

function LinkedTaskPRLinks({ task, links, className }: {
  task: Task; links: TaskPRLink[]; className?: string;
}) {
  const query = useProjectPRStatuses(task.project_id);
  const multipleRepos = new Set(links.map((link) => link.repository)).size > 1;

  return (
    <div className={cn("flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground", className)}>
      {links.map((link) => {
        const found = query.data?.statuses?.find((status) =>
          status.repository.toLowerCase() === link.repository.toLowerCase() && status.number === link.number);
        const snapshot = found && query.isError
          ? { ...found, stale: true, error: "Could not refresh PR status" } : found;
        return (
          <span key={`${link.repository}/${link.number}`} className="inline-flex min-w-0 max-w-full items-center gap-1">
            <a
              href={link.url}
              target="_blank"
              rel="noopener noreferrer"
              title={`${link.repository} PR #${link.number}${snapshot ? ` · ${snapshot.lifecycle}` : ""} (opens in a new tab)`}
              aria-label={`Open ${link.repository} PR #${link.number} on GitHub (opens in a new tab)${snapshot ? ` · ${snapshot.lifecycle}` : ""}`}
              onClick={(event) => event.stopPropagation()}
              onPointerDown={(event) => event.stopPropagation()}
              // Mouse and touch sensors listen separately from pointerdown.
              onMouseDown={(event) => event.stopPropagation()}
              onTouchStart={(event) => event.stopPropagation()}
              onKeyDown={(event) => event.stopPropagation()}
              className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-sm font-medium hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <PRLifecycleIcon state={snapshot?.lifecycle} />
              <span className="truncate">{multipleRepos ? `${link.repository} ` : ""}PR #{link.number}</span>
              <ExternalLink aria-hidden="true" className="size-3 shrink-0" />
            </a>
            <PRStatusOverview snapshot={snapshot} repository={link.repository} number={link.number} />
          </span>
        );
      })}
    </div>
  );
}

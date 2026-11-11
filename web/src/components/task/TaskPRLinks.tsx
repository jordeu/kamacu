import { ExternalLink } from "lucide-react";
import type { Task } from "@/api/types";
import { cn } from "@/lib/utils";

/** Secondary task metadata, shared by the board and task header. */
export function TaskPRLinks({ task, className }: { task: Task; className?: string }) {
  const links = task.pr_links?.filter((link) => link.role !== "incidental") ?? [];
  if (!links.length) return null;
  const multipleRepos = new Set(links.map((link) => link.repository)).size > 1;

  return (
    <div className={cn("flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground", className)}>
      {links.map((link) => (
        <a
          key={`${link.repository}/${link.number}`}
          href={link.url}
          target="_blank"
          rel="noopener noreferrer"
          title={`${link.repository} PR #${link.number} (opens in a new tab)`}
          aria-label={`Open ${link.repository} PR #${link.number} on GitHub (opens in a new tab)`}
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          // The board uses MouseSensor and TouchSensor, which listen to these
          // events separately from pointerdown. Keep native link behavior.
          onMouseDown={(event) => event.stopPropagation()}
          onTouchStart={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
          className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-sm font-medium hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="truncate">{multipleRepos ? `${link.repository} ` : ""}PR #{link.number}</span>
          <ExternalLink aria-hidden="true" className="size-3 shrink-0" />
        </a>
      ))}
    </div>
  );
}

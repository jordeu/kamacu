import { useState } from "react";
import { ChevronDown, ExternalLink } from "lucide-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { useProjectPRStatuses } from "@/api/prStatus";
import type { Task, TaskPRLink } from "@/api/types";
import { cn } from "@/lib/utils";
import { PRLifecycleIcon, PRStatusOverview } from "./PRStatusOverview";

/** Secondary task metadata, shared by the board and task header. */
export function TaskPRLinks({ task, className, compact = false }: { task: Task; className?: string; compact?: boolean }) {
  const links = task.pr_links?.filter((link) => link.role !== "incidental") ?? [];
  if (!links.length) return null;
  return <LinkedTaskPRLinks task={task} links={links} className={className} compact={compact} />;
}

function LinkedTaskPRLinks({ task, links, className, compact }: {
  task: Task; links: TaskPRLink[]; className?: string; compact: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const query = useProjectPRStatuses(task.project_id);
  const multipleRepos = new Set(links.map((link) => link.repository)).size > 1;

  const renderLink = (link: TaskPRLink) => {
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
  };

  if (compact && links.length > 1) {
    // Evidence refreshes must not change which association was linked last.
    // Keep the last API entry as a deterministic fallback for undated links.
    const latest = links.reduce((last, link) => {
      const at = Date.parse(link.linked_at ?? link.observed_at ?? "") || 0;
      const lastAt = Date.parse(last.linked_at ?? last.observed_at ?? "") || 0;
      return at >= lastAt ? link : last;
    });
    const remaining = links.filter((link) => link !== latest);
    return <Collapsible open={expanded} onOpenChange={setExpanded}
      className={cn("min-w-0 text-xs text-muted-foreground", className)}
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onMouseDown={(event) => event.stopPropagation()}
      onTouchStart={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <div className="flex min-w-0 items-center justify-between gap-1">
        {renderLink(latest)}
        <CollapsibleTrigger
          aria-label={expanded ? "Hide additional linked pull requests" : `Show ${remaining.length} more linked pull ${remaining.length === 1 ? "request" : "requests"}`}
          className="inline-flex h-7 shrink-0 cursor-pointer items-center gap-0.5 rounded-sm px-1 hover:bg-zinc-700/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {expanded ? "Less" : `+${remaining.length}`}
          <ChevronDown aria-hidden="true" className={cn("size-3", expanded && "rotate-180")} />
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent>
        <div className="mt-1 flex min-w-0 flex-col items-start gap-1">{remaining.map(renderLink)}</div>
      </CollapsibleContent>
    </Collapsible>;
  }

  return <div className={cn("flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground", className)}>
    {links.map(renderLink)}
  </div>;
}

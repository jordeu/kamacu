import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { ExternalLink } from "lucide-react";
import { useOpenReview } from "@/api/pullRequests";
import type { AgentStatusEntry } from "@/api/agents";
import type { Project, Task } from "@/api/types";
import type { OverviewPR } from "@/lib/workspaceOverview";
import { formatDateTime, formatDuration } from "@/lib/time";
import { cn } from "@/lib/utils";
import { ProjectAvatar } from "@/components/ui/ProjectAvatar";
import { StatusDot } from "@/components/StatusDot";

function Updated({ value, now }: { value: string; now: number }) {
  const seconds = Math.max(0, (now - Date.parse(value)) / 1000);
  return (
    <time
      dateTime={value}
      title={`Updated ${formatDateTime(value)}`}
      className="shrink-0 text-xs text-muted-foreground"
    >
      Updated {formatDuration(seconds)} ago
    </time>
  );
}

export function TaskRow({
  task,
  project,
  agent,
  now,
}: {
  task: Task;
  project: Project;
  agent?: AgentStatusEntry;
  now: number;
}) {
  const links =
    task.pr_links?.filter((link) => link.role !== "incidental") ?? [];
  return (
    <li className="border-t">
      <Link
        to={`/projects/${project.id}/tasks/${task.id}`}
        className="flex min-w-0 items-center gap-3 px-3 py-3 transition-colors hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring md:px-4"
      >
        <ProjectAvatar
          size="rail"
          letters={project.icon_letters}
          color={project.icon_color}
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-2">
            <span
              className="line-clamp-2 text-sm font-medium [overflow-wrap:anywhere]"
              title={task.title}
            >
              {task.title}
            </span>
            {agent && <StatusDot entry={agent} className="mt-1.5" />}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-2 text-xs text-muted-foreground [overflow-wrap:anywhere]">
            <span>{project.name}</span>
            <span>·</span>
            <span>Task {task.id}</span>
            {links.length > 0 && (
              <>
                <span>·</span>
                <span>
                  {links.length} linked {links.length === 1 ? "PR" : "PRs"}
                </span>
              </>
            )}
          </div>
          <span className="mt-1 block md:hidden">
            <Updated value={task.updated_at} now={now} />
          </span>
        </div>
        <span className="hidden md:inline">
          <Updated value={task.updated_at} now={now} />
        </span>
      </Link>
    </li>
  );
}

const checks = {
  pass: { label: "Checks pass", className: "text-green-400" },
  fail: { label: "Checks fail", className: "text-red-400" },
  pending: { label: "Checks pending", className: "text-amber-400" },
  none: { label: "No checks", className: "text-muted-foreground" },
};

export function ReviewRow({ pr, now }: { pr: OverviewPR; now: number }) {
  const review = useOpenReview(pr.project.id);
  const navigate = useNavigate();
  const check = checks[pr.checks];
  function open() {
    if (review.isPending) return;
    review.mutate(pr.number, {
      onSuccess: (result) =>
        navigate(`/projects/${pr.project.id}/tasks/${result.task.id}`),
    });
  }
  return (
    <li className="border-t">
      <div className="flex items-center gap-2 pr-3 md:pr-4">
        <button
          type="button"
          onClick={open}
          disabled={review.isPending}
          aria-label={`Open review for ${pr.project.github_repo} PR ${pr.number}: ${pr.title}`}
          className="flex min-w-0 flex-1 items-center gap-3 px-3 py-3 text-left transition-colors hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring disabled:opacity-60 md:px-4"
        >
          <ProjectAvatar
            size="rail"
            letters={pr.project.icon_letters}
            color={pr.project.icon_color}
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1">
            <span
              className="line-clamp-2 text-sm font-medium [overflow-wrap:anywhere]"
              title={pr.title}
            >
              {pr.title}
            </span>
            <div className="mt-1 flex flex-wrap gap-x-2 text-xs text-muted-foreground [overflow-wrap:anywhere]">
              <span>{pr.project.name}</span>
              <span>·</span>
              <span>PR {pr.number}</span>
              <span>·</span>
              <span>@{pr.author}</span>
              {pr.stale && <span className="text-amber-400">Cached</span>}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 md:hidden">
              <span className={cn("text-xs", check.className)}>
                {check.label}
              </span>
              <Updated value={pr.updatedAt} now={now} />
            </div>
            {review.isPending && (
              <p role="status" className="mt-1 text-xs text-muted-foreground">
                Setting up the review worktree…
              </p>
            )}
          </div>
          <span
            className={cn(
              "hidden shrink-0 rounded bg-muted/40 px-2 py-1 text-xs md:inline",
              check.className,
            )}
          >
            {check.label}
          </span>
          <span className="hidden md:inline">
            <Updated value={pr.updatedAt} now={now} />
          </span>
        </button>
        <a
          href={pr.url}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={`Open ${pr.project.github_repo} PR ${pr.number} on GitHub (opens in a new tab)`}
          className="flex size-9 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ExternalLink className="size-4" aria-hidden="true" />
        </a>
      </div>
      {review.isError && (
        <p role="alert" className="break-words px-4 pb-3 text-xs text-red-400">
          Couldn't open this review. {review.error.message}
        </p>
      )}
    </li>
  );
}

export function QueueSection({
  id,
  title,
  count,
  color,
  hint,
  children,
  remaining,
  expanded,
  toggle,
}: {
  id: string;
  title: string;
  count: string | number;
  color: string;
  hint: string;
  children: ReactNode;
  remaining: number;
  expanded: boolean;
  toggle: () => void;
}) {
  return (
    <section
      aria-labelledby={`${id}-heading`}
      className="overflow-hidden rounded-md border"
    >
      <header className="flex flex-wrap items-center gap-2 bg-card/40 px-3 py-3 md:px-4">
        <span
          className={cn("size-2 shrink-0 rounded-full", color)}
          aria-hidden="true"
        />
        <h2 id={`${id}-heading`} className="text-sm font-medium">
          {title}
        </h2>
        <span
          className="rounded border bg-muted/30 px-1.5 text-xs tabular-nums text-muted-foreground"
          aria-label={`${title}: ${count}`}
        >
          {count}
        </span>
        <p className="ml-auto hidden text-xs text-muted-foreground xl:block">
          {hint}
        </p>
      </header>
      {children}
      {(remaining > 0 || expanded) && (
        <button
          type="button"
          onClick={toggle}
          aria-expanded={expanded}
          className="min-h-10 w-full border-t px-4 py-2 text-left text-xs text-muted-foreground hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        >
          {expanded ? "Show fewer" : `Show all · ${remaining} more`}
        </button>
      )}
    </section>
  );
}

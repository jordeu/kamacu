import { useLayoutEffect, useRef, type ReactNode } from "react";
import type { Project, Status, Task } from "@/api/types";
import type { OverviewPR } from "@/lib/workspaceOverview";
import { TaskCard } from "@/components/board/TaskCard";
import { PRCard } from "@/components/board/PRCard";
import { ProjectAvatar } from "@/components/ui/ProjectAvatar";
import { cn } from "@/lib/utils";
import { TaskStatusDropdown } from "./TaskStatusDropdown";

const positions = new Map<
  string,
  { left: number; lanes: Record<string, number> }
>();

function ProjectBadge({
  project,
  onSelect,
}: {
  project: Project;
  onSelect: (id: number) => void;
}) {
  return (
    <button
      type="button"
      title={`Project: ${project.name}`}
      aria-label={`Filter to ${project.name}`}
      onClick={(event) => {
        event.stopPropagation();
        onSelect(project.id);
      }}
      onKeyDown={(event) => event.stopPropagation()}
      className="inline-flex max-w-full items-center gap-1 rounded border bg-muted/30 py-0.5 pr-1.5 pl-0.5 text-[10px] leading-4 text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ProjectAvatar
        size="inline"
        letters={project.icon_letters}
        color={project.icon_color}
        className="size-3.5 text-[8px]"
        aria-hidden="true"
      />
      <span className="min-w-0 truncate">{project.name}</span>
    </button>
  );
}

export function WorkspaceBoard({
  progress,
  review,
  prs,
  projects,
  taskCountReady,
  prCount,
  taskPlaceholder,
  prPlaceholder,
  prNotice,
  onProjectSelect,
  scrollKey,
  tasksReady,
  reviewsReady,
  movePending,
  onTaskMove,
}: {
  progress: Task[];
  review: Task[];
  prs: OverviewPR[];
  projects: Project[];
  taskCountReady: boolean;
  prCount: number | string;
  taskPlaceholder: (kind: "progress" | "review") => ReactNode;
  prPlaceholder: ReactNode;
  prNotice?: ReactNode;
  onProjectSelect: (id: number) => void;
  scrollKey: string;
  tasksReady: boolean;
  reviewsReady: boolean;
  movePending: boolean;
  onTaskMove: (task: Task, status: Status) => void;
}) {
  const board = useRef<HTMLDivElement>(null);
  const lanes = useRef<Record<string, HTMLDivElement | null>>({});
  const projectMap = new Map(projects.map((project) => [project.id, project]));
  useLayoutEffect(() => {
    const position = positions.get(scrollKey);
    if (board.current) board.current.scrollLeft = position?.left ?? 0;
    for (const kind of ["progress", "review", "prs"]) {
      const lane = lanes.current[kind];
      if (lane && (kind === "prs" ? reviewsReady : tasksReady))
        lane.scrollTop = position?.lanes[kind] ?? 0;
    }
  }, [scrollKey, tasksReady, reviewsReady]);
  function remember(kind: string, value: number) {
    const position = positions.get(scrollKey) ?? { left: 0, lanes: {} };
    if (kind === "board") position.left = value;
    else position.lanes[kind] = value;
    positions.delete(scrollKey);
    positions.set(scrollKey, position);
    if (positions.size > 30) positions.delete(positions.keys().next().value!);
  }
  return (
    <div
      ref={board}
      aria-label="Workspace board"
      onScroll={(event) => {
        if (event.target === event.currentTarget)
          remember("board", event.currentTarget.scrollLeft);
      }}
      className="flex min-h-0 flex-1 snap-x snap-proximity gap-4 overflow-x-auto px-3 pb-3 scroll-px-3 md:px-6 md:pb-4 md:scroll-px-6"
    >
      {(["progress", "review", "prs"] as const).map((kind) => {
        const title =
          kind === "progress"
            ? "Tasks in progress"
            : kind === "review"
              ? "Tasks in review"
              : "PRs awaiting your review";
        const group = kind === "progress" ? progress : review;
        const count =
          kind === "prs" ? prCount : taskCountReady ? group.length : "—";
        return (
          <section
            key={kind}
            aria-labelledby={`overview-${kind}-heading`}
            className="flex min-h-0 w-[calc(100vw-3rem)] min-w-0 shrink-0 snap-start flex-col md:w-auto md:min-w-[260px] md:flex-1"
          >
            <header className="flex items-center gap-2 px-3 pb-2">
              <span
                aria-hidden="true"
                className={cn(
                  "size-1.5 shrink-0 rounded-full",
                  kind === "progress"
                    ? "bg-blue-400"
                    : kind === "review"
                      ? "bg-purple-400"
                      : "bg-green-400",
                )}
              />
              <h2
                id={`overview-${kind}-heading`}
                className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
              >
                {title}
              </h2>
              <span
                className="shrink-0 text-xs tabular-nums text-muted-foreground"
                aria-label={`${title}: ${count}`}
              >
                {count}
              </span>
            </header>
            <div
              ref={(node) => {
                lanes.current[kind] = node;
              }}
              tabIndex={0}
              aria-label={`${title} cards`}
              onScroll={(event) => {
                if (event.target === event.currentTarget)
                  remember(kind, event.currentTarget.scrollTop);
              }}
              className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto overscroll-y-contain rounded-md bg-[#101013] p-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
            >
              {kind === "prs" ? (
                <>
                  {prs.map((pr) => (
                    <PRCard
                      key={`${pr.project.id}/${pr.project.github_repo}/${pr.number}`}
                      pr={pr}
                      projectId={pr.project.id}
                      allowHiding
                      aria-label={`Open review for ${pr.project.name} PR #${pr.number}`}
                      footer={
                        <div className="flex items-center justify-between gap-2">
                          <ProjectBadge
                            project={pr.project}
                            onSelect={onProjectSelect}
                          />
                          {pr.stale && (
                            <span className="shrink-0 text-[10px] text-amber-400">
                              Cached
                            </span>
                          )}
                        </div>
                      }
                    />
                  ))}
                  {!prs.length && prPlaceholder}
                  {prNotice}
                </>
              ) : (
                <>
                  {group.map((task) => (
                    <TaskCard
                      key={task.id}
                      task={task}
                      aria-label={`${task.title} · ${projectMap.get(task.project_id)!.name}`}
                      sortable={false}
                      activePRsOnly
                      footer={
                        <div className="flex items-center justify-between gap-2">
                          <ProjectBadge
                            project={projectMap.get(task.project_id)!}
                            onSelect={onProjectSelect}
                          />
                          <TaskStatusDropdown
                            task={task}
                            projectName={projectMap.get(task.project_id)!.name}
                            disabled={movePending}
                            onMove={onTaskMove}
                          />
                        </div>
                      }
                    />
                  ))}
                  {!group.length && taskPlaceholder(kind)}
                </>
              )}
            </div>
          </section>
        );
      })}
    </div>
  );
}

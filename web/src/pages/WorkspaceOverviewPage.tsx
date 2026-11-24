import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { RefreshCw, Search, X } from "lucide-react";
import { useProjects, useWorkspaces } from "@/api/queries";
import { useSettings } from "@/api/settings";
import { useRefreshActiveProjectPRStatuses } from "@/api/prStatus";
import {
  useWorkspaceReviews,
  useWorkspaceTasks,
} from "@/api/workspaceOverview";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";
import {
  filterOverview,
  overviewReviews,
  isReviewQueueIncomplete,
} from "@/lib/workspaceOverview";
import { formatDateTime } from "@/lib/time";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { WorkspaceBoard } from "@/components/overview/WorkspaceBoard";
import { useMoveWorkspaceTask } from "@/api/workspaceTaskMove";
import { STATUS_LABELS, type Status, type Task } from "@/api/types";
import { ApiError, get } from "@/api/client";
import { CleanupWorktreeDialog } from "@/components/task/CleanupWorktreeDialog";

export default function WorkspaceOverviewPage() {
  const { workspaceId: rawId } = useParams();
  const workspaceId = Number(rawId);
  const { data: workspaces, isError, refetch } = useWorkspaces();
  const { setActiveWorkspaceId } = useActiveWorkspace();
  const workspace = workspaces?.find(
    (candidate) => candidate.id === workspaceId,
  );

  // The URL owns the scope on deep links, just as project routes do. Switching
  // workspaces navigates to a new overview URL rather than reusing old data.
  const resolvedWorkspaceId = workspace?.id;
  useEffect(() => {
    if (resolvedWorkspaceId !== undefined)
      setActiveWorkspaceId(resolvedWorkspaceId);
  }, [resolvedWorkspaceId, setActiveWorkspaceId]);

  if (isError && !workspaces)
    return (
      <LoadError
        message="Couldn't load workspaces."
        retry={() => void refetch()}
      />
    );
  if (!workspaces)
    return (
      <div className="p-6">
        <Skeleton className="h-8 w-64" />
      </div>
    );
  if (!workspace)
    return (
      <div className="p-6">
        <h1 className="text-base font-medium">Workspace not found</h1>
        <Link className="mt-3 inline-block text-sm underline" to="/">
          Go to your workspace
        </Link>
      </div>
    );
  return (
    <WorkspaceOverview
      key={workspaceId}
      workspaceId={workspaceId}
      workspaceName={workspace.name}
    />
  );
}

function LoadError({ message, retry }: { message: string; retry: () => void }) {
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-3 p-6 text-sm text-muted-foreground"
    >
      <p>{message}</p>
      <Button variant="outline" size="sm" onClick={retry}>
        Retry loading
      </Button>
    </div>
  );
}

function WorkspaceOverview({
  workspaceId,
  workspaceName,
}: {
  workspaceId: number;
  workspaceName: string;
}) {
  const projectsQuery = useProjects();
  const settingsQuery = useSettings();
  const statusRefresh = useRefreshActiveProjectPRStatuses();
  const move = useMoveWorkspaceTask(workspaceId);
  const moving = useRef(false);
  const [lastMove, setLastMove] = useState<{ task: Task; from: Status } | null>(
    null,
  );
  const [moveError, setMoveError] = useState<string | null>(null);
  const [undoPending, setUndoPending] = useState(false);
  const [cleanupTask, setCleanupTask] = useState<Task | null>(null);
  function moveTask(task: Task, status: Status) {
    if (moving.current || task.status === status) return;
    moving.current = true;
    setLastMove(null);
    setMoveError(null);
    move.mutate(
      { task, status },
      {
        onSuccess: (result) => {
          setLastMove({ task: result, from: task.status });
          if (status === "done" && result.worktree_path) setCleanupTask(result);
        },
        onError: (error) =>
          setMoveError(`Couldn't move “${task.title}”: ${error.message}`),
        onSettled: () => {
          moving.current = false;
        },
      },
    );
  }
  async function undoMove() {
    if (!lastMove || moving.current) return;
    moving.current = true;
    setUndoPending(true);
    setMoveError(null);
    try {
      // Do not overwrite a status change made since the original move.
      const current = await get<Task>(`/api/tasks/${lastMove.task.id}`);
      if (current.status !== lastMove.task.status) {
        setLastMove(null);
        throw new Error(
          "This task has changed since your move. Refresh to see its current status.",
        );
      }
      await move.mutateAsync({ task: current, status: lastMove.from });
      setLastMove(null);
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) setLastMove(null);
      setMoveError(
        `Couldn't undo the move: ${error instanceof Error ? error.message : "Try again."}`,
      );
    } finally {
      moving.current = false;
      setUndoPending(false);
    }
  }
  const { data: settings, isError: settingsError } = settingsQuery;
  const projects = (projectsQuery.data ?? []).filter(
    (project) => project.workspace_id === workspaceId,
  );
  const tasks = useWorkspaceTasks(
    workspaceId,
    projectsQuery.data !== undefined && !move.isPending,
  );
  const githubEnabled = settings?.github_integration?.value === "on";
  const reviews = useWorkspaceReviews(
    workspaceId,
    projects,
    githubEnabled && projectsQuery.data !== undefined,
  );
  const [params, setParams] = useSearchParams();
  const pendingParams = useRef(params);
  useLayoutEffect(() => {
    pendingParams.current = params;
  }, [params]);
  const projectId = projects.some(
    (project) => String(project.id) === params.get("project"),
  )
    ? params.get("project")!
    : "all";
  const search = params.get("q") ?? "";
  const sort = params.get("sort") === "old" ? "old" : "recent";
  const reviewData = githubEnabled ? (reviews.data ?? []) : [];
  const list = filterOverview(
    tasks.data ?? [],
    overviewReviews(projects, reviewData),
    projects,
    projectId,
    search,
    sort,
  );
  const visibleQueues = reviewData.filter(
    (entry) => projectId === "all" || String(entry.projectId) === projectId,
  );
  const unavailable = visibleQueues.filter(isReviewQueueIncomplete);
  const reviewsReady =
    githubEnabled &&
    projectsQuery.data !== undefined &&
    reviews.data !== undefined;
  const reviewCount =
    !reviewsReady || reviews.isError
      ? "—"
      : unavailable.length
        ? list.prs.length > 0
          ? `${list.prs.length} shown`
          : "—"
        : list.prs.length;
  const projectMap = new Map(projects.map((project) => [project.id, project]));
  const filtered = search.trim() !== "" || projectId !== "all";
  const linkedProjects = projects.filter((project) =>
    project.github_repo?.trim(),
  );
  const reviewEmpty = !githubEnabled
    ? settingsError
      ? "Couldn't load GitHub settings."
      : settings
        ? "GitHub integration is turned off."
        : "Loading GitHub settings…"
    : reviews.isError && !reviews.data
      ? "Couldn't load PR review requests. Try refreshing."
      : !reviewsReady
        ? "Loading PR review requests…"
        : unavailable.length
          ? "Some review requests are unavailable. See repository status below."
          : linkedProjects.length === 0
            ? "Link a GitHub repository in project settings to see review requests."
            : filtered
              ? "No PR reviews match these filters."
              : "No PRs are waiting for your review.";
  function update(key: string, value: string) {
    // Router search-param callbacks do not queue like state updates. Merge
    // against the latest requested filters when typing and selecting quickly.
    const next = new URLSearchParams(pendingParams.current);
    if (!value || value === "all" || (key === "sort" && value === "recent"))
      next.delete(key);
    else next.set(key, value);
    next.delete("show"); // Old action-queue links still open the board.
    pendingParams.current = next;
    setParams(next, { replace: true });
  }
  function refresh() {
    void settingsQuery.refetch();
    void tasks.refetch();
    if (githubEnabled) {
      void reviews.refresh();
      statusRefresh.mutate(projects.map((project) => project.id));
    }
  }
  if (projectsQuery.isError && !projectsQuery.data)
    return (
      <LoadError
        message="Couldn't load projects."
        retry={() => void projectsQuery.refetch()}
      />
    );
  return (
    <div
      className="relative flex h-full min-h-0 flex-col"
      data-testid="workspace-overview"
    >
      <header className="shrink-0 px-3 pt-5 pb-4 md:px-6 md:pt-6">
        <p className="mb-2 break-words text-xs text-muted-foreground">
          {workspaceName} / Workspace
        </p>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-xl font-medium tracking-tight">
            Workspace overview
          </h1>
          <Button
            size="sm"
            variant="ghost"
            onClick={refresh}
            disabled={
              tasks.isFetching ||
              reviews.isFetching ||
              statusRefresh.isPending ||
              move.isPending ||
              undoPending
            }
            aria-label="Refresh workspace overview"
          >
            <RefreshCw className="size-4" aria-hidden="true" />
            <span className="hidden sm:inline">Refresh</span>
          </Button>
        </div>
        <p className="mt-1 break-words text-sm text-muted-foreground">
          {projectsQuery.data
            ? `${projects.length} ${projects.length === 1 ? "project" : "projects"}`
            : "Loading projects…"}{" "}
          · Your project boards, together.
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <label className="relative w-full sm:max-w-72">
            <Search
              className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
              aria-hidden="true"
            />
            <span className="sr-only">Search work</span>
            <input
              type="search"
              value={search}
              onChange={(event) => update("q", event.target.value)}
              placeholder="Search work…"
              className="h-9 w-full rounded-md border bg-card/40 pr-3 pl-9 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
          <label className="min-w-0 flex-1 sm:max-w-64">
            <span className="sr-only">Filter project</span>
            <select
              value={projectId}
              onChange={(event) => update("project", event.target.value)}
              className="h-9 w-full rounded-md border bg-background px-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="all">All projects</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
          </label>
          <label className="min-w-0 flex-1 sm:ml-auto sm:max-w-52">
            <span className="sr-only">Sort items</span>
            <select
              value={sort}
              onChange={(event) => update("sort", event.target.value)}
              className="h-9 w-full rounded-md border bg-background px-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="recent">Recently updated</option>
              <option value="old">Least recently updated</option>
            </select>
          </label>
        </div>
        {projectsQuery.data && projects.length === 0 && (
          <p className="mt-4 text-sm text-muted-foreground">
            No projects in this workspace yet. Use Add project to get started.
          </p>
        )}
        {statusRefresh.isError && (
          <p role="alert" className="mt-3 text-sm text-amber-400">
            {statusRefresh.error.message}
          </p>
        )}
        {(tasks.isError || projectsQuery.isError) && (
          <div
            role="alert"
            className="mt-3 flex flex-wrap items-center gap-3 text-sm text-amber-400"
          >
            <span>
              {tasks.isError
                ? tasks.data
                  ? "Couldn't refresh tasks. Showing the last loaded tasks."
                  : "Couldn't load tasks."
                : "Couldn't refresh projects. Showing the last loaded projects."}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void projectsQuery.refetch();
                void tasks.refetch();
              }}
            >
              Retry loading
            </Button>
          </div>
        )}
      </header>
      <WorkspaceBoard
        progress={list.progress}
        review={list.review}
        prs={list.prs}
        projects={projects}
        onProjectSelect={(id) => update("project", String(id))}
        onTaskMove={moveTask}
        movePending={move.isPending || undoPending}
        scrollKey={JSON.stringify([workspaceId, projectId, search, sort])}
        tasksReady={tasks.data !== undefined}
        reviewsReady={reviewsReady || reviews.isError || !githubEnabled}
        taskCountReady={tasks.data !== undefined}
        prCount={reviewCount}
        taskPlaceholder={(kind) =>
          !tasks.data && !tasks.isError ? (
            <div className="space-y-2" aria-label="Loading tasks">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : (
            <p className="py-5 text-sm text-muted-foreground">
              {tasks.isError && !tasks.data
                ? "Tasks are unavailable. Try again above."
                : filtered
                  ? "No tasks match these filters."
                  : `No tasks ${kind === "progress" ? "in progress" : "in review"}.`}
            </p>
          )
        }
        prPlaceholder={
          <p className="py-5 text-sm text-muted-foreground">{reviewEmpty}</p>
        }
        prNotice={
          <>
            {unavailable.length > 0 && (
              <p className="mt-1 text-xs text-amber-400">
                PR count is partial. Cached requests may have changed.
              </p>
            )}
            {reviews.isError && reviews.data && (
              <p role="alert" className="text-xs text-amber-400">
                Couldn't refresh review requests. Showing the last loaded
                results.
              </p>
            )}
            {unavailable.length > 0 && (
              <ul className="space-y-2 border-t pt-2 text-xs text-amber-400">
                {unavailable.map((entry) => (
                  <li key={entry.projectId} className="break-words">
                    {projectMap.get(entry.projectId)?.name}:{" "}
                    {entry.failed
                      ? "couldn't refresh"
                      : entry.data?.state === "auth_required"
                        ? "GitHub authentication required"
                        : entry.data?.state === "no_gh"
                          ? "GitHub CLI unavailable"
                          : entry.data?.state === "disabled"
                            ? "GitHub integration unavailable"
                            : entry.data?.state !== "ok"
                              ? "couldn't reach GitHub"
                              : !entry.data.fetchedAt
                                ? "review requests not ready"
                                : "cached results"}
                    {entry.data?.fetchedAt
                      ? ` · last fetched ${formatDateTime(entry.data.fetchedAt)}`
                      : " · no successful fetch"}
                  </li>
                ))}
              </ul>
            )}
          </>
        }
      />
      <p className="shrink-0 px-3 pb-3 text-xs text-muted-foreground md:px-6">
        Click a project badge to filter. Hover or select a linked PR status for
        details.
      </p>
      {(lastMove || moveError) && (
        <div className="absolute right-3 bottom-10 left-3 z-20 flex items-center gap-3 rounded-md border bg-popover p-3 text-sm shadow-lg sm:left-auto sm:max-w-md">
          <div className="min-w-0 flex-1">
            {lastMove && (
              <p role="status" className="break-words">
                Moved “{lastMove.task.title}” to{" "}
                {STATUS_LABELS[lastMove.task.status]}.
              </p>
            )}
            {moveError && (
              <p role="alert" className="break-words text-destructive">
                {moveError}
              </p>
            )}
          </div>
          {lastMove && (
            <Button
              size="sm"
              variant="outline"
              disabled={move.isPending || undoPending}
              onClick={() => void undoMove()}
            >
              {undoPending ? "Undoing…" : "Undo"}
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="Dismiss task move notification"
            disabled={move.isPending || undoPending}
            onClick={() => {
              setLastMove(null);
              setMoveError(null);
            }}
          >
            <X className="size-4" aria-hidden="true" />
          </Button>
        </div>
      )}
      {cleanupTask && (
        <CleanupWorktreeDialog
          open
          taskId={cleanupTask.id}
          taskTitle={cleanupTask.title}
          projectId={cleanupTask.project_id}
          trigger="done"
          onOpenChange={(open) => {
            if (!open) setCleanupTask(null);
          }}
          onCleanedUp={() => setLastMove(null)}
        />
      )}
    </div>
  );
}

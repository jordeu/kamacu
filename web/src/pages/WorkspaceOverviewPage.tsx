import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { RefreshCw, Search } from "lucide-react";
import { useProjects, useWorkspaces } from "@/api/queries";
import { useSettings } from "@/api/settings";
import { useAgentStatuses } from "@/api/agents";
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
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  TaskRow,
  ReviewRow,
  QueueSection,
} from "@/components/overview/OverviewQueue";

const scrollPositions = new Map<string, number>();

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
  const { data: settings, isError: settingsError } = settingsQuery;
  const { data: agents } = useAgentStatuses();
  const projects = (projectsQuery.data ?? []).filter(
    (project) => project.workspace_id === workspaceId,
  );
  const tasks = useWorkspaceTasks(
    workspaceId,
    projectsQuery.data !== undefined,
  );
  const githubEnabled = settings?.github_integration?.value === "on";
  const reviews = useWorkspaceReviews(
    workspaceId,
    projects,
    githubEnabled && projectsQuery.data !== undefined,
  );
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const [params, setParams] = useSearchParams();
  const projectId = projects.some(
    (project) => String(project.id) === params.get("project"),
  )
    ? params.get("project")!
    : "all";
  const search = params.get("q") ?? "";
  const sort = params.get("sort") === "old" ? "old" : "recent";
  const expanded = (params.get("show") ?? "").split(",");
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
  const incomplete = visibleQueues.some(isReviewQueueIncomplete);
  const reviewsReady = githubEnabled && reviews.data !== undefined;
  const reviewCount =
    !reviewsReady || reviews.isError
      ? "—"
      : incomplete
        ? list.prs.length > 0
          ? `${list.prs.length} shown`
          : "—"
        : list.prs.length;
  const projectMap = new Map(projects.map((project) => [project.id, project]));
  const scrollRef = useRef<HTMLDivElement>(null);
  // Expanding a group changes the URL but should preserve the viewport.
  const scrollKey = JSON.stringify([workspaceId, projectId, search, sort]);
  const ready = tasks.data !== undefined;
  useLayoutEffect(() => {
    if (!ready) return;
    const node = scrollRef.current;
    if (!node) return;
    node.scrollTop = scrollPositions.get(scrollKey) ?? 0;
  }, [scrollKey, ready]);
  function rememberScroll(position: number) {
    // Record before navigation removes the scroll container from the DOM.
    scrollPositions.delete(scrollKey);
    scrollPositions.set(scrollKey, position);
    if (scrollPositions.size > 30)
      scrollPositions.delete(scrollPositions.keys().next().value!);
  }

  function update(key: string, value: string) {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (!value || value === "all" || (key === "sort" && value === "recent"))
          next.delete(key);
        else next.set(key, value);
        return next;
      },
      { replace: true },
    );
  }
  function toggle(kind: string) {
    update(
      "show",
      expanded.includes(kind)
        ? expanded.filter((entry) => entry !== kind).join(",")
        : [...expanded.filter(Boolean), kind].join(","),
    );
  }
  function retry() {
    void settingsQuery.refetch();
    void tasks.refetch();
    if (githubEnabled) void reviews.refresh();
  }
  const filtered = search.trim() !== "" || projectId !== "all";
  const taskLoading = tasks.data === undefined && !tasks.isError;
  const taskError = tasks.isError;
  const linkedProjects = projects.filter((project) =>
    project.github_repo?.trim(),
  );
  const reviewUnavailable = !reviewsReady || incomplete || reviews.isError;
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
        : incomplete
          ? "Some review requests are unavailable. See repository status below."
          : linkedProjects.length === 0
            ? "Link a GitHub repository in project settings to see review requests."
            : filtered
              ? "No PR reviews match these filters."
              : "No PRs are waiting for your review.";

  if (projectsQuery.isError && !projectsQuery.data)
    return (
      <LoadError
        message="Couldn't load projects."
        retry={() => void projectsQuery.refetch()}
      />
    );
  return (
    <div
      ref={scrollRef}
      onScroll={(event) => rememberScroll(event.currentTarget.scrollTop)}
      className="h-full overflow-y-auto"
      data-testid="workspace-overview"
    >
      <div className="mx-auto max-w-[1440px] px-3 py-5 md:px-6 md:py-6">
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
            onClick={retry}
            disabled={tasks.isFetching || reviews.isFetching}
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
          · Ongoing work and reviews across {workspaceName}.
        </p>
        <div className="mt-5 flex flex-wrap gap-2" aria-label="Queue counts">
          {[
            {
              label: "In progress",
              count: tasks.data ? list.progress.length : "—",
              color: "bg-blue-400",
            },
            {
              label: "In review",
              count: tasks.data ? list.review.length : "—",
              color: "bg-purple-400",
            },
            { label: "PR reviews", count: reviewCount, color: "bg-green-400" },
          ].map((item) => (
            <div
              key={item.label}
              className="flex items-center gap-2 rounded-md border px-3 py-2 text-xs text-muted-foreground"
            >
              <span
                className={cn("size-1.5 rounded-full", item.color)}
                aria-hidden="true"
              />
              <strong className="text-base font-medium tabular-nums text-foreground">
                {item.count}
              </strong>
              {item.label}
            </div>
          ))}
        </div>
        <div className="mt-5 flex flex-wrap items-center gap-2 border-b pb-4">
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
          <p className="mt-5 text-sm text-muted-foreground">
            No projects in this workspace yet. Use Add project to get started.
          </p>
        )}
        {(taskError || projectsQuery.isError) && (
          <div
            role="alert"
            className="mt-4 flex flex-wrap items-center gap-3 text-sm text-amber-400"
          >
            <span>
              {taskError
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
        <div className="mt-5 flex flex-col gap-4">
          {(["progress", "review"] as const).map((kind) => {
            const group = list[kind];
            const all = expanded.includes(kind);
            return (
              <QueueSection
                key={kind}
                id={`overview-${kind}`}
                title={
                  kind === "progress" ? "Tasks in progress" : "Tasks in review"
                }
                count={tasks.data ? group.length : "—"}
                color={kind === "progress" ? "bg-blue-400" : "bg-purple-400"}
                hint={`Kamacu board status · ${kind === "progress" ? "in progress" : "in review"}`}
                remaining={Math.max(0, group.length - 5)}
                expanded={all && group.length > 5}
                toggle={() => toggle(kind)}
              >
                {taskLoading ? (
                  <div
                    className="space-y-2 border-t p-4"
                    aria-label="Loading tasks"
                  >
                    <Skeleton className="h-12 w-full" />
                    <Skeleton className="h-12 w-full" />
                  </div>
                ) : group.length ? (
                  <ul>
                    {group.slice(0, all ? group.length : 5).map((task) => (
                      <TaskRow
                        key={task.id}
                        now={now}
                        task={task}
                        project={projectMap.get(task.project_id)!}
                        agent={agents?.find(
                          (agent) => agent.taskId === task.id,
                        )}
                      />
                    ))}
                  </ul>
                ) : (
                  <p className="border-t p-4 text-sm text-muted-foreground">
                    {taskError && !tasks.data
                      ? "Tasks are unavailable. Try again above."
                      : filtered
                        ? "No tasks match these filters."
                        : `No tasks ${kind === "progress" ? "in progress" : "in review"}.`}
                  </p>
                )}
              </QueueSection>
            );
          })}
          <QueueSection
            id="overview-prs"
            title="PRs awaiting your review"
            count={reviewCount}
            color="bg-green-400"
            hint="Open PRs · review requested from you"
            remaining={Math.max(0, list.prs.length - 5)}
            expanded={expanded.includes("prs") && list.prs.length > 5}
            toggle={() => toggle("prs")}
          >
            {list.prs.length ? (
              <ul>
                {list.prs
                  .slice(0, expanded.includes("prs") ? list.prs.length : 5)
                  .map((pr) => (
                    <ReviewRow
                      key={`${pr.project.github_repo}/${pr.number}`}
                      now={now}
                      pr={pr}
                    />
                  ))}
              </ul>
            ) : (
              <p
                className="border-t p-4 text-sm text-muted-foreground"
                role={
                  reviewUnavailable && reviews.isError ? "alert" : undefined
                }
              >
                {reviewEmpty}
              </p>
            )}
            {incomplete && (
              <p className="border-t px-4 py-3 text-xs text-amber-400">
                PR count is partial. Cached requests may have changed.
              </p>
            )}
            {reviews.isError && reviews.data && (
              <p
                role="alert"
                className="border-t px-4 py-3 text-xs text-amber-400"
              >
                Couldn't refresh review requests. Showing the last loaded
                results.
              </p>
            )}
            {visibleQueues.some(isReviewQueueIncomplete) && (
              <div className="border-t px-4 py-3">
                <ul className="space-y-1 text-xs text-amber-400">
                  {visibleQueues
                    .filter(isReviewQueueIncomplete)
                    .map((entry) => (
                      <li key={entry.projectId}>
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
              </div>
            )}
          </QueueSection>
        </div>
        <p className="mt-4 text-xs text-muted-foreground">
          Counts describe each queue. Updated times show the last change to an
          item.
          {reviews.dataUpdatedAt > 0 && githubEnabled
            ? ` PR queues checked ${new Date(reviews.dataUpdatedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}.`
            : ""}
        </p>
      </div>
    </div>
  );
}

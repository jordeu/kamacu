import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, Ellipsis, ExternalLink, Plus } from "lucide-react";
import { ApiError } from "@/api/client";
import { prDetailKey, type PRDetailWire } from "@/api/pullRequests";
import { useTask } from "@/api/queries";
import { useUpdateTask } from "@/api/mutations";
import { useCreateWorktree } from "@/api/worktrees";
import { useAgentStatuses } from "@/api/agents";
import {
  useDeleteSession,
  useReattachTmux,
  useSessions,
  useSpawnSession,
  useStopSession,
} from "@/api/sessions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { StatusDot } from "@/components/StatusDot";
import { QuotaIndicator } from "@/components/quota/QuotaIndicator";
import { AgentTab } from "@/components/task/AgentTab";
import { CleanupWorktreeDialog } from "@/components/task/CleanupWorktreeDialog";
import { DeleteTaskDialog } from "@/components/task/DeleteTaskDialog";
import { DescriptionTab } from "@/components/task/DescriptionTab";
import { DiffTab } from "@/components/task/DiffTab";
import { TaskTabs, type TabDef } from "@/components/task/TaskTabs";
import { WorktreeMetaLine } from "@/components/task/WorktreeMetaLine";
import { TerminalPane } from "@/components/terminal/TerminalPane";

function isTypingTarget(el: Element | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  // Covers the xterm helper textarea too — focused terminals keep Esc.
  return (
    el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable
  );
}

export default function TaskPage() {
  const { projectId: projectIdParam, taskId: taskIdParam } = useParams();
  const projectId = Number(projectIdParam);
  const taskId = Number(taskIdParam);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Fetch by id so deep links work without the board cache (TASK-05).
  const { data: task, isPending, isError } = useTask(taskId);
  const updateTask = useUpdateTask(projectId);
  const createWorktree = useCreateWorktree(taskId, projectId);

  // Bash session lifecycle (D-27..D-30) — the server is tab truth (D-28).
  const { data: sessions } = useSessions(taskId);
  const spawn = useSpawnSession(taskId);
  const stopSession = useStopSession();
  const deleteSession = useDeleteSession();
  // Restored tmux survivors auto-reattach invisibly (TMUX-05, D-88).
  const reattach = useReattachTmux(taskId);

  // Agent tab dot (D-50) — same query/component as the board card dot.
  const agentEntry = (useAgentStatuses().data ?? []).find(
    (e) => e.taskId === taskId,
  );

  // Newest agent session, running or exited (server list is newest-first).
  // An exited agent stays in the pane with the Start-again banner until a
  // fresh spawn replaces it.
  const agentSession = (sessions ?? []).find((s) => s.kind === "agent");

  // D-39: opening a task ALWAYS lands on the Agent tab — no smart selection.
  const [activeTab, setActiveTab] = useState("agent");
  // Sessions the user × -closed: muted, removed once they leave running.
  const [closingIds, setClosingIds] = useState<Set<string>>(new Set());
  // Ids seen RUNNING during this mount (RESEARCH OQ2): a session that exits
  // on its own keeps a muted tab until banner-Close within this visit; a
  // later revisit shows running sessions only.
  const [keepExitedIds, setKeepExitedIds] = useState<Set<string>>(new Set());

  const [titleDraft, setTitleDraft] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const cancelTitleEditRef = useRef(false);
  // tmux names already reattached this mount (TMUX-05): the 5s poll keeps
  // surfacing an orphaned ghost until the live session replaces it, and
  // StrictMode double-mounts, so this ref makes the reattach spawn fire exactly
  // once per name. The backend is attach-or-create idempotent — this only
  // avoids redundant POSTs.
  const reattachedRef = useRef<Set<string>>(new Set());

  // Every session currently running for this task joins keepExitedIds.
  useEffect(() => {
    if (!sessions) return;
    setKeepExitedIds((prev) => {
      let next: Set<string> | null = null;
      for (const s of sessions) {
        if (s.status === "running" && !prev.has(s.id)) {
          next ??= new Set(prev);
          next.add(s.id);
        }
      }
      return next ?? prev;
    });
  }, [sessions]);

  // Auto-reattach restored tmux survivors (TMUX-05, D-88): each orphaned bash
  // ghost fires ONE reattach spawn against its persisted tmux name. The ghost
  // never renders as its own tab (filtered out of visibleSessions below); once
  // the reattach succeeds the real session appears and connects via the normal
  // WS path — indistinguishable from a tab that was always there (D-77). No
  // Resume button, no banner: the reattach is automatic and invisible.
  useEffect(() => {
    if (!sessions) return;
    for (const s of sessions) {
      if (!s.orphaned || s.kind === "agent" || !s.tmuxName) continue;
      if (reattachedRef.current.has(s.tmuxName)) continue;
      reattachedRef.current.add(s.tmuxName);
      reattach.mutate(s.tmuxName);
    }
  }, [sessions, reattach]);

  // Visible bash tabs: running sessions, plus self-exited ones kept muted
  // within this visit. User-initiated closes (closingIds) drop on exit.
  // Spawn order left-to-right: createdAt ascending, label tiebreak (the
  // server list is newest-first).
  const visibleSessions = useMemo(() => {
    return (sessions ?? [])
      .filter(
        (s) =>
          // Agents never render as closable bash tabs (D-38) — the Agent
          // tab owns them.
          s.kind !== "agent" &&
          // Restored tmux ghosts (TMUX-05) are transient reattach triggers, not
          // tabs: the real session replaces them on the next poll (D-88).
          !s.orphaned &&
          (s.status === "running" ||
            (s.status === "exited" &&
              keepExitedIds.has(s.id) &&
              !closingIds.has(s.id))),
      )
      .sort((a, b) =>
        a.createdAt === b.createdAt
          ? a.label.localeCompare(b.label)
          : a.createdAt.localeCompare(b.createdAt),
      );
  }, [sessions, keepExitedIds, closingIds]);

  // "diff" joins tabIds ONLY when a worktree exists. The disabled Diff tab
  // stays OUT of tabIds so the existing Pitfall-7 layout effect handles a
  // mid-view worktree removal for free — "diff" drops out, the effect
  // reactivates the Agent tab (the universal fallback) before paint.
  const tabIds = useMemo(
    () => [
      "agent",
      "description",
      ...(task?.worktree_path ? ["diff"] : []),
      ...visibleSessions.map((s) => s.id),
    ],
    [task?.worktree_path, visibleSessions],
  );

  // Pitfall 7: a Radix Tabs value pointing at a removed tab renders blank.
  // When the active tab disappears (closing session exited via poll or 'x'
  // frame), activate its left neighbor from the previous order, else the
  // Agent tab (which can never disappear) — before paint, no blank frame.
  const prevTabIdsRef = useRef<string[]>(tabIds);
  useLayoutEffect(() => {
    const prev = prevTabIdsRef.current;
    prevTabIdsRef.current = tabIds;
    if (tabIds.includes(activeTab)) return;
    let next = "agent";
    for (let i = prev.indexOf(activeTab) - 1; i >= 0; i--) {
      if (tabIds.includes(prev[i])) {
        next = prev[i];
        break;
      }
    }
    setActiveTab(next);
  }, [tabIds, activeTab]);

  // Esc returns to the board (D-07) — always the explicit board route, never
  // history-back, because deep-linked tabs have no history. Suppressed while
  // typing in inputs/textareas/contenteditable (the xterm helper textarea IS
  // a textarea, so focused terminals keep Esc) or while any Radix
  // dialog/menu is open.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      if (isTypingTarget(document.activeElement)) return;
      if (e.target instanceof Element && isTypingTarget(e.target)) return;
      if (
        document.querySelector(
          '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"]',
        )
      ) {
        return;
      }
      navigate(`/projects/${projectId}`);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [navigate, projectId]);

  if (isPending) {
    return (
      <div className="mx-auto w-full max-w-[860px] space-y-4 p-4">
        <Skeleton className="h-7 w-2/3" />
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-3/4" />
      </div>
    );
  }

  if (isError || !task) {
    return (
      <div className="mx-auto w-full max-w-[860px] space-y-3 p-4">
        <p className="text-muted-foreground">Task not found.</p>
        <Link
          to={`/projects/${projectId}`}
          className="text-sm underline underline-offset-4 hover:text-foreground"
        >
          Back to board
        </Link>
      </div>
    );
  }

  // PR review branch (Phase 12): a source='github_pr' task renders the
  // read-only review identity. The live PR detail (title/body/author/url/base)
  // was stashed by useOpenReview at open time (prDetailKey) and survives
  // reattach in the cache; fall back to the task's own pr_* fields if absent
  // (e.g. a hard refresh on a deep link — header degrades, never breaks).
  const isPR = task.source === "github_pr";
  const prDetail = isPR
    ? queryClient.getQueryData<PRDetailWire>(prDetailKey(task.id))
    : undefined;
  const prTitle = prDetail?.title ?? task.title;
  // D-07 seed: prefilled-not-sent on agent Start, interpolating the live PR.
  const seed = isPR
    ? `Review PR #${task.pr_number} "${prTitle}". Summarize the changes, then flag bugs, risky changes, and missing tests.`
    : undefined;

  function commitTitle(value: string) {
    const cancelled = cancelTitleEditRef.current;
    cancelTitleEditRef.current = false;
    setTitleDraft(null);
    if (cancelled || !task) return;
    const trimmed = value.trim();
    // Trimmed-empty reverts without saving (TASK-04).
    if (!trimmed || trimmed === task.title) return;
    updateTask.mutate({ id: task.id, title: trimmed });
  }

  // Removes a tab NOW (banner-Close on an exited session) — next active tab
  // is computed in the same update (Pitfall 7).
  function removeTab(id: string, opts?: { deleteServerSide?: boolean }) {
    setActiveTab((current) => {
      if (current !== id) return current;
      const ids = prevTabIdsRef.current;
      const idx = ids.indexOf(id);
      return idx > 0 ? ids[idx - 1] : "agent";
    });
    setKeepExitedIds((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
    setClosingIds((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
    // Banner-Close frees the server-side ring buffer (mirrors TerminalPage).
    if (opts?.deleteServerSide) deleteSession.mutate(id);
  }

  // × on a running tab (D-29): stop fires immediately, tab enters the
  // closing state (muted); removal happens when the session leaves running.
  function handleCloseTab(id: string) {
    setClosingIds((prev) => new Set(prev).add(id));
    stopSession.mutate(id);
  }

  // Spawning happens ONLY from click handlers — never from effects
  // (StrictMode double-mount would double-spawn). The hook's setQueryData
  // makes the tab render immediately; activate it in the same success.
  function handleSpawn() {
    spawn.mutate(undefined, {
      onSuccess: (s) => {
        setKeepExitedIds((prev) => new Set(prev).add(s.id));
        setActiveTab(s.id);
      },
    });
  }

  const tabs: TabDef[] = [
    {
      id: "agent",
      label: "Agent",
      // D-50: same StatusDot/state source as the board card; no dot pre-start.
      leading: agentEntry ? <StatusDot entry={agentEntry} /> : undefined,
      // Switching tabs never detaches the agent WS.
      keepMounted: true,
      // NO onClose — permanent tab (D-38): stopped, never closed.
      content: (
        <div className="flex h-full min-h-[320px] w-full flex-col">
          <AgentTab
            task={task}
            agentSession={agentSession}
            projectId={projectId}
            seed={seed}
          />
        </div>
      ),
    },
    {
      id: "description",
      label: "Description",
      content: (
        <div className="h-full max-w-[860px] overflow-y-auto">
          <DescriptionTab
            task={task}
            projectId={projectId}
            readOnlySource={isPR ? (prDetail?.body ?? "") : undefined}
          />
        </div>
      ),
    },
    // D-64: Agent, Description, Diff, Bash 1..N. Disabled (zinc-600 label +
    // tooltip) when the task has no worktree. NOT keepMounted — mounting on
    // activation IS the D-61 fetch-on-open; re-activating remounts and
    // refetches. Never gets onClose (no ×) or leading (no dot): the label and
    // active indicator stay stock accent blue, explicitly NOT diff-colored.
    {
      id: "diff",
      label: "Diff",
      disabled: !task.worktree_path,
      disabledTooltip: `The diff needs a worktree`,
      content: (
        <div className="flex h-full min-h-[320px] w-full flex-col">
          <DiffTab taskId={task.id} />
        </div>
      ),
    },
    ...visibleSessions.map((s): TabDef => {
      const closing = closingIds.has(s.id);
      return {
        id: s.id,
        label: s.label,
        muted: closing || s.status === "exited",
        onClose:
          s.status === "running" && !closing
            ? () => handleCloseTab(s.id)
            : undefined,
        keepMounted: true,
        // Bash content is exempt from the 860px constraint — terminal real
        // estate fills the main area below the strip (min 320px).
        content: (
          <div className="flex h-full min-h-[320px] w-full flex-col">
            <TerminalPane
              key={s.id}
              sessionId={s.id}
              label={s.label}
              status={s.status}
              exitCode={s.exitCode}
              onSessionExit={() =>
                queryClient.invalidateQueries({
                  queryKey: ["sessions", taskId],
                })
              }
              onClosed={() => removeTab(s.id, { deleteServerSide: true })}
              onNewTerminal={handleSpawn}
            />
          </div>
        ),
      };
    }),
  ];

  const canSpawn = Boolean(task.worktree_path) && !spawn.isPending;

  const trailing = (
    <div className="flex items-center gap-2">
      <Tooltip>
        <TooltipTrigger asChild>
          {/* Disabled buttons swallow pointer events — the span keeps the
              explanation tooltip firing (D-30). */}
          <span className="inline-flex">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="New bash session"
              disabled={!canSpawn}
              onClick={handleSpawn}
            >
              <Plus className="size-4" />
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {task.worktree_path
            ? "New bash session"
            : "Bash sessions need a worktree"}
        </TooltipContent>
      </Tooltip>
      {spawn.isError && !spawn.isPending && (
        <span className="text-xs whitespace-nowrap text-red-500">
          {/* 409s are deliberate, human-readable conflict copy from the
              server (D-84 et al.) — render verbatim; 500s stay generic. */}
          {spawn.error instanceof ApiError && spawn.error.status === 409
            ? spawn.error.message
            : "Couldn't start a session. Try again."}
        </span>
      )}
    </div>
  );

  return (
    // Full-width working surface (Layout Contract): terminals get the whole
    // main area; header/meta span full width (UI-01) while Description prose
    // keeps its 860px island.
    <div className="flex h-full w-full flex-col gap-6 p-4">
      <div className="w-full shrink-0 space-y-2">
        <header className="flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Back to board"
                onClick={() => navigate(`/projects/${projectId}`)}
              >
                <ArrowLeft className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Back to board (Esc)</TooltipContent>
          </Tooltip>

          {isPR ? (
            // Read-only PR title (D-08): same box/typography as the editable
            // title so there is zero layout shift, but NOT a button — no hover
            // surface, no onClick, no Edit affordance. Reads as a label.
            <span
              className="min-w-0 flex-1 truncate px-1 py-0.5 text-left text-base font-medium"
              title={prTitle}
            >
              {prTitle}
            </span>
          ) : titleDraft === null ? (
            <button
              type="button"
              className="min-w-0 flex-1 truncate rounded-md px-1 py-0.5 text-left text-base font-medium hover:bg-muted/50"
              title="Edit title"
              onClick={() => setTitleDraft(task.title)}
            >
              {task.title}
            </button>
          ) : (
            <Input
              autoFocus
              value={titleDraft}
              className="h-8 flex-1 text-base font-medium"
              onChange={(e) => setTitleDraft(e.target.value)}
              onBlur={(e) => commitTitle(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.currentTarget.blur();
                } else if (e.key === "Escape") {
                  cancelTitleEditRef.current = true;
                  e.currentTarget.blur();
                }
              }}
            />
          )}

          <QuotaIndicator />

          {/* ⋯ menu OMITTED for a PR review (D-10): no Delete-task, no
              Clean-up-worktree yet (Phase 13 re-adds cleanup). ↗ open-on-GitHub
              lives in the PR meta line instead. */}
          {!isPR && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" aria-label="Task actions">
                  <Ellipsis className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {/* D-31 menu path: neutral item — it opens a gated dialog;
                    `Delete task` stays the menu's only red item. */}
                {task.worktree_path && (
                  <>
                    <DropdownMenuItem onSelect={() => setCleanupOpen(true)}>
                      Clean up worktree
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                  </>
                )}
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => setDeleteOpen(true)}
                >
                  Delete task
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </header>

        {isPR ? (
          // PR meta line (D-09): #num · @author · base · ↗. The worktree always
          // exists by render time, so no Create/Retry affordance applies — the
          // PR identity is the only meta this review needs.
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <span>#{task.pr_number}</span>
            <span>·</span>
            <span>@{prDetail?.author}</span>
            <span>·</span>
            <span className="font-mono">
              {prDetail?.baseRefName ?? task.pr_base_ref}
            </span>
            <span>·</span>
            <a
              href={prDetail?.url}
              target="_blank"
              rel="noreferrer"
              aria-label={`Open PR #${task.pr_number} on GitHub`}
              className="text-muted-foreground hover:text-foreground"
            >
              <ExternalLink className="size-3.5" />
            </a>
          </div>
        ) : (
          <WorktreeMetaLine
            task={task}
            onCreate={() => createWorktree.mutate()}
            creating={createWorktree.isPending}
          />
        )}
      </div>

      <TaskTabs
        tabs={tabs}
        value={activeTab}
        onValueChange={setActiveTab}
        trailing={trailing}
      />

      <CleanupWorktreeDialog
        open={cleanupOpen}
        onOpenChange={setCleanupOpen}
        taskId={task.id}
        taskTitle={task.title}
        projectId={projectId}
        trigger="menu"
      />

      <DeleteTaskDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        taskId={task.id}
        taskTitle={task.title}
        projectId={projectId}
      />
    </div>
  );
}

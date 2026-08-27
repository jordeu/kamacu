import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { Plus } from "lucide-react";
import { ApiError } from "@/api/client";
import { useGlobal } from "@/api/global";
import { useAgentStatuses } from "@/api/agents";
import {
  useDeleteSession,
  useGlobalSessions,
  useReattachGlobalTmux,
  useRenameGlobalSession,
  useSpawnGlobalSession,
  useStopSession,
} from "@/api/sessions";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { StatusDot } from "@/components/StatusDot";
import { QuotaIndicator } from "@/components/quota/QuotaIndicator";
import { AgentTab } from "@/components/task/AgentTab";
import { TaskTabs, type TabDef } from "@/components/task/TaskTabs";
import { TerminalPane } from "@/components/terminal/TerminalPane";

/**
 * The /global Scratchpad view (GVIEW-01 + GVIEW-04, 16-02): a copy-then-trim
 * of TaskPage — NEVER a shared-shell refactor (research Anti-Patterns;
 * TaskPage is the hottest page in the app). The trimmed shell renders the
 * Agent tab plus bash tabs ONLY, with the whole TaskPage bash-tab lifecycle
 * copied wholesale (states, reattach guard, neighbor reactivation) and every
 * task-only branch deleted: no title editing, no worktree meta, no cleanup/
 * delete dialogs, no PR branches, no Esc-to-board handler (no parent board
 * exists), no taskId-derived lookups.
 *
 * The 7-state view matrix (UI-SPEC LOCKED) is driven by the ONE shared
 * ["global"] query (D-17 one round-trip), evaluated top-down: pending
 * skeleton → load error → unconfigured hero (D-39) → vanished-root hero
 * (D-40) → shell + warn banner (D-42) / shell + isolation banner (D-38) /
 * shell bare. Server truth only — never a client-side root fallback
 * (D-28..D-30 honesty posture).
 */
export default function GlobalTaskPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // The ONE config query (D-17) — shared with the Settings Scratchpad
  // section; root_exists and the live counts are server-derived per request.
  const { data: config, isPending, isError, refetch } = useGlobal();

  // Bash session lifecycle (task parity, scope-aware spellings) — the server
  // is tab truth (D-28).
  const { data: sessions } = useGlobalSessions();
  const spawn = useSpawnGlobalSession();
  const stopSession = useStopSession();
  const deleteSession = useDeleteSession();
  // Tab rename (TABS-01/02, D-03): PATCHes the session label; wired per-bash-tab
  // below (offered on every bash tab regardless of shell mode — the backend
  // handles the plain-bash/tmux persistence distinction).
  const renameSession = useRenameGlobalSession();
  // Restored tmux survivors auto-reattach invisibly (TMUX-05, D-88).
  const reattach = useReattachGlobalTmux();

  // Agent tab dot — the server's synthesized source === "global" status
  // entry (Phase 15; taskId 0 never collides with a real task).
  const agentEntry = (useAgentStatuses().data ?? []).find(
    (e) => e.source === "global",
  );

  // Newest agent session, running or exited (server list is newest-first).
  // An exited agent stays in the pane with the Start-again banner until a
  // fresh spawn replaces it.
  const agentSession = (sessions ?? []).find((s) => s.kind === "agent");

  // Opening the view ALWAYS lands on the Agent tab — no smart selection
  // (D-39 task parity).
  const [activeTab, setActiveTab] = useState("agent");
  // Sessions the user ×-closed: muted, removed once they leave running.
  const [closingIds, setClosingIds] = useState<Set<string>>(new Set());
  // Ids seen RUNNING during this mount: a session that exits on its own
  // keeps a muted tab until banner-Close within this visit; a later revisit
  // shows running sessions only.
  const [keepExitedIds, setKeepExitedIds] = useState<Set<string>>(new Set());
  // tmux names already reattached this mount (TMUX-05): the 5s poll keeps
  // surfacing an orphaned ghost until the live session replaces it, and
  // StrictMode double-mounts, so this ref makes the reattach spawn fire exactly
  // once per name. The backend is attach-or-create idempotent — this only
  // avoids redundant POSTs.
  const reattachedRef = useRef<Set<string>>(new Set());

  // Every session currently running joins keepExitedIds (RESEARCH OQ2):
  // accumulated DURING render via React's adjust-state-on-changed-data
  // pattern (the AddProjectDialog prevOpen-tracker idiom — research Pitfall
  // 8), never inside an effect. The guard makes it converge: once every
  // running id is in the set the branch stops firing, and the 5s poll
  // re-evaluates it on every sessions change.
  if (sessions) {
    let missingRunning = false;
    for (const s of sessions) {
      if (s.status === "running" && !keepExitedIds.has(s.id)) {
        missingRunning = true;
        break;
      }
    }
    if (missingRunning) {
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
    }
  }

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

  // The strip is EXACTLY [agent, ...bash tabs] — nothing else (GVIEW-01).
  const tabIds = useMemo(
    () => ["agent", ...visibleSessions.map((s) => s.id)],
    [visibleSessions],
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

  // State 1 — query pending: the TaskPage skeleton idiom.
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

  // State 2 — query error: the SettingsPage error/retry idiom.
  if (isError || !config) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-muted-foreground">{`Couldn't load the scratchpad.`}</p>
        <Button variant="outline" onClick={() => refetch()}>
          {`Retry loading`}
        </Button>
      </div>
    );
  }

  // State 3 (D-39) — unconfigured: the full-page hero pointing at Settings.
  // NEVER a silent $HOME/cwd fallback (standing honesty posture).
  if (config.root_path === "") {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-4 py-8 text-center">
          <div className="flex flex-col items-center gap-1">
            <h1 className="text-xl font-medium">
              {`Scratchpad isn't configured yet`}
            </h1>
            <p className="max-w-[480px] text-muted-foreground">
              {`The root folder and default agent live in Settings.`}
            </p>
          </div>
          <Button onClick={() => navigate("/settings")}>{`Open Settings`}</Button>
        </div>
      </div>
    );
  }

  const rootExists = config.root_exists;
  const anyLive =
    config.live.agent + config.live.bash + config.live.tmux > 0;

  // State 4 (D-40) — vanished root, nothing live: the DISTINCT vanished-root
  // hero naming root_path verbatim in a mono span — never the generic
  // unconfigured state above (deliberate misconfiguration-vs-disk-rot
  // distinction).
  if (!rootExists && !anyLive) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-4 py-8 text-center">
          <div className="flex flex-col items-center gap-1">
            <h1 className="text-xl font-medium">
              {`The Scratchpad root no longer exists`}
            </h1>
            <p className="max-w-[480px] text-muted-foreground">
              {`The configured root no longer exists on disk: `}
              <span className="font-mono">{config.root_path}</span>
              {`. Point Scratchpad at a new root in Settings.`}
            </p>
          </div>
          <Button onClick={() => navigate("/settings")}>{`Open Settings`}</Button>
        </div>
      </div>
    );
  }

  // States 5/6/7 — the full task-parity shell. Reaching here with a missing
  // root means live sessions exist (state 4 returned above): warn, don't
  // disturb — attached terminals keep streaming, nothing is killed or
  // detached. The banner slot renders EXACTLY ONE of {D-42 warn, D-38
  // isolation} and nothing for a managed clone (state 7 — the clone
  // namespace IS the isolation boundary).
  const banner = !rootExists ? (
    // State 5 (D-42): advisory, non-blocking — the server 409s any spawn
    // against the vanished root and the message surfaces verbatim inline.
    <div
      role="status"
      className="flex flex-wrap items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-xs text-amber-700 dark:text-amber-400"
    >
      <span>
        {`The configured root no longer exists on disk: `}
        <span className="font-mono">{config.root_path}</span>
        {`. Running sessions are unaffected.`}
      </span>
    </div>
  ) : config.github_repo === null ? (
    // State 6 (D-38): persistent, one line, NOT dismissible — the folder-root
    // isolation warning (the agent runs un-isolated in the user's checkout).
    <div
      role="status"
      className="flex flex-wrap items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-xs text-amber-700 dark:text-amber-400"
    >
      <span>
        {`Agent runs directly in `}
        <span className="font-mono">{config.root_path}</span>
        {` — no worktree isolation.`}
      </span>
    </div>
  ) : null;

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
      // Same StatusDot/state source as a task card; no dot pre-start.
      leading: agentEntry ? <StatusDot entry={agentEntry} /> : undefined,
      // Switching tabs never detaches the agent WS.
      keepMounted: true,
      // NO onClose — permanent tab (D-38): stopped, never closed.
      content: (
        <div className="flex h-full min-h-[320px] w-full flex-col">
          <AgentTab scope={{ kind: "global" }} agentSession={agentSession} />
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
        // Rename affordance on EVERY bash tab (D-03), but only while running:
        // an exited/closing session is gone, so a rename can't persist
        // meaningfully — mirror the onClose gate (Claude's discretion, D-02).
        onRename:
          s.status === "running" && !closing
            ? (label: string) => renameSession.mutate({ id: s.id, label })
            : undefined,
        keepMounted: true,
        // Bash content is exempt from any max-width constraint — terminal
        // real estate fills the main area below the strip (min 320px).
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
                  queryKey: ["sessions", "global"],
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

  // Warn-don't-block (Pitfall 9): states 3/4 are full-page replacements, so
  // within the shell the + stays enabled everywhere it renders — including
  // state 5, where a click the server refuses surfaces its 409 verbatim.
  const canSpawn = !spawn.isPending;

  const trailing = (
    <div className="flex items-center gap-2">
      <Tooltip>
        <TooltipTrigger asChild>
          {/* Disabled buttons swallow pointer events — the span keeps the
              tooltip firing while a spawn is pending. */}
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
        <TooltipContent>{`New bash session`}</TooltipContent>
      </Tooltip>
      {spawn.isError && !spawn.isPending && (
        <span className="text-xs whitespace-nowrap text-red-500">
          {/* 409s are deliberate, human-readable conflict copy from the
              server (D-34 et al.) — render verbatim; 500s stay generic. */}
          {spawn.error instanceof ApiError && spawn.error.status === 409
            ? spawn.error.message
            : `Couldn't start a session. Try again.`}
        </span>
      )}
    </div>
  );

  // D-37 locked outcome: the quota chip renders iff the global DEFAULT
  // agent's engine is claude — keyed off the ["global"] query, NOT TaskPage's
  // pattern-based gate (the stale types.ts union misses "opencode"; research
  // Pitfall 2).
  const isClaudeEngine = config.agent.engine === "claude";

  return (
    // Full-width working surface (Layout Contract): terminals get the whole
    // main area; the header and banner span full width.
    <div className="flex h-full w-full flex-col gap-6 p-4">
      <div className="w-full shrink-0 space-y-2">
        <header className="flex items-center gap-2">
          {/* D-35: static label — no back-arrow (top-level destination like
              /settings), no edit affordance, no page-level ⋯ menu. */}
          <h1 className="text-base font-medium">{`Scratchpad`}</h1>
          {isClaudeEngine && (
            <div className="ml-auto">
              <QuotaIndicator />
            </div>
          )}
        </header>
        {banner}
      </div>

      <TaskTabs
        tabs={tabs}
        value={activeTab}
        onValueChange={setActiveTab}
        trailing={trailing}
      />
    </div>
  );
}

import { useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Ellipsis } from "lucide-react";
import { useTask } from "@/api/queries";
import { useGlobal } from "@/api/global";
import { useAgentStatuses, type AgentStatusEntry } from "@/api/agents";
import {
  useResumeAgent,
  useResumeGlobalAgent,
  useSpawnAgent,
  useSpawnGlobalAgent,
  useStopSession,
  type TermSession,
} from "@/api/sessions";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TerminalPane } from "@/components/terminal/TerminalPane";

/**
 * Scope discriminator (16-02, research Pattern 4): the ONE prop that selects
 * task vs global behavior. "task" keeps the historical worktree-gated flow;
 * "global" (the Phase 16 Scratchpad, /global) resolves the scope-aware hooks
 * and the D-36/D-41 divergences. Scope is transparent to the terminal
 * engine — sessions are cwd-driven.
 */
export type AgentTabScope =
  | { kind: "task"; taskId: number }
  | { kind: "global" };

/**
 * The permanent Agent tab's content (D-38..D-41, revised at checkpoint
 * 2026-06-11): pre-start empty state, running TerminalPane with the
 * Insert-description header action (task scope only — the global scope's ⋯
 * menu renders Stop alone, D-36), and the code-free exited banner
 * ("Agent session ended.") over a dimmed terminal whose single
 * "Reset session" action spawns a FRESH claude session. The pane NEVER
 * spawns — all spawning happens here from click handlers (StrictMode
 * double-spawn guard).
 */
export function AgentTab({
  scope,
  agentSession,
  description,
  seed,
}: {
  scope: AgentTabScope;
  agentSession: TermSession | undefined;
  // Task-scope Insert text (D-35/36/37): the task's description. The global
  // scope passes none — no description exists there, so that menu item
  // degrades away (D-36).
  description?: string;
  // PR-review seed (Phase 12 D-06/D-07): when set, prefill (never send) this
  // prompt into the agent once, after the session connects. Task scope only.
  seed?: string;
}) {
  const isTask = scope.kind === "task";
  // Both hook sets are declared unconditionally (rules-of-hooks); the set a
  // given scope doesn't use never fires — mutations only hit the wire on
  // .mutate(). NaN disables the task query on the global branch (useTask's
  // !isNaN guard): the global pre-start reads its metadata from useGlobal().
  const taskId = isTask ? scope.taskId : Number.NaN;
  const taskSpawn = useSpawnAgent(taskId);
  const taskResume = useResumeAgent(taskId);
  const globalSpawn = useSpawnGlobalAgent();
  const globalResume = useResumeGlobalAgent();
  const spawn = isTask ? taskSpawn : globalSpawn;
  const resume = isTask ? taskResume : globalResume;
  // Stop is scope-blind: it targets a session id either way.
  const stopSession = useStopSession();
  const queryClient = useQueryClient();
  const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null);

  // Scope metadata from the already-shared caches — never a new round-trip:
  // the task scope reads ["task", id] (TaskPage resolved it before mounting
  // this tab — it early-returns on pending), the global scope reads the ONE
  // ["global"] query the /global shell drives (D-17; both cached app-wide).
  const { data: task } = useTask(taskId);
  const { data: globalConfig } = useGlobal();

  // The server's resumable flag is the SINGLE driver for which pair renders —
  // the UI never infers resumability client-side (UI-SPEC). Read it from the
  // already-polled shared query (Phase 4 dedupe pattern; no new props): the
  // task scope finds its task's entry, the global scope the server's
  // synthesized source === "global" entry (Phase 15; taskId 0).
  const statusEntries = useAgentStatuses().data ?? [];
  const resumable =
    (isTask
      ? statusEntries.find((e) => e.taskId === scope.taskId)
      : statusEntries.find((e) => e.source === "global")
    )?.resumable ?? false;
  // Mutual exclusion: while either Resume or Reset is in flight both buttons
  // disable, preventing a double-spawn racing the one-per-task 409 gate.
  const busy = spawn.isPending || resume.isPending;

  // State A — pre-start (task D-40 / global D-41): no agent session has run.
  if (!agentSession) {
    // D-54b resumable pre-start: post-restart the manager has no session but
    // the server reports a resumable transcript. Same envelope as the plain
    // pre-start (centered block, py-8, gap-2; body max-w-[480px]) — offers the
    // Reset/Resume pair instead of a single Start. The server only reports
    // resumable when a worktree exists, so no worktree check is needed here.
    if (resumable) {
      return (
        <div className="flex flex-col items-center gap-2 py-8 text-center">
          <h2 className="text-xl font-medium">{`Agent session ended.`}</h2>
          <p className="max-w-[480px] text-sm text-muted-foreground">
            {`A previous conversation can be picked up where it left off. Reset starts a fresh session instead.`}
          </p>
          <div className="flex items-center gap-2">
            <Button variant="ghost" disabled={busy} onClick={() => spawn.mutate()}>
              {spawn.isPending ? `Resetting…` : `Reset session`}
            </Button>
            <Button disabled={busy} onClick={() => resume.mutate()}>
              {resume.isPending ? `Resuming…` : `Resume session`}
            </Button>
          </div>
          {(spawn.isError || resume.isError) && !busy && (
            <span className="text-xs text-red-500">
              {`Couldn't start a session. Try again.`}
            </span>
          )}
        </div>
      );
    }

    if (isTask) {
      // Task branch — byte-for-byte the historical worktree-gated pre-start.
      // `task` is the same ["task", id] cache entry TaskPage rendered from,
      // so the gate and branch copy resolve identically.
      const hasWorktree = Boolean(task?.worktree_path);
      const branch = task?.branch ?? "";
      const cta = (
        <Button disabled={!hasWorktree || spawn.isPending} onClick={() => spawn.mutate()}>
          {spawn.isPending ? `Starting…` : `Start agent`}
        </Button>
      );
      return (
        <div className="flex flex-col items-center gap-2 py-8 text-center">
          <h2 className="text-xl font-medium">{`No agent session`}</h2>
          {hasWorktree ? (
            <p className="text-sm text-muted-foreground">
              {`Runs the agent in this task's worktree on `}
              <span className="font-mono">{branch}</span>
              {`.`}
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              {`The agent needs a worktree. Use Create worktree above.`}
            </p>
          )}
          {hasWorktree ? (
            cta
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                {/* Disabled buttons swallow pointer events — the span keeps
                    the explanation tooltip firing. */}
                <span className="inline-flex">{cta}</span>
              </TooltipTrigger>
              <TooltipContent>{`Agent sessions need a worktree`}</TooltipContent>
            </Tooltip>
          )}
          {spawn.isError && !spawn.isPending && (
            <span className="text-xs text-red-500">
              {`Couldn't start a session. Try again.`}
            </span>
          )}
        </div>
      );
    }

    // Global branch (D-41): the /global shell only renders this tab in view
    // states 5/6/7, so the CTA is enabled unconditionally — there is no
    // worktree concept to gate on. The body names the configured root (mono
    // span) from the shared ["global"] query.
    return (
      <div className="flex flex-col items-center gap-2 py-8 text-center">
        <h2 className="text-xl font-medium">{`No agent session`}</h2>
        <p className="text-sm text-muted-foreground">
          {`The agent runs directly in `}
          <span className="font-mono">{globalConfig?.root_path ?? ""}</span>
          {`.`}
        </p>
        <Button disabled={spawn.isPending} onClick={() => spawn.mutate()}>
          {spawn.isPending ? `Starting…` : `Start agent`}
        </Button>
        {spawn.isError && !spawn.isPending && (
          <span className="text-xs text-red-500">
            {`Couldn't start a session. Try again.`}
          </span>
        )}
      </div>
    );
  }

  // Agent ⋯ menu (D-06/D-07): folds the old standalone Insert-description
  // header button, the manual PR-review-prompt insert (D-09, no longer
  // auto-pasted on connect), and Stop into a single dropdown that supersedes
  // the pane's inline Stop button (headerMenu, D-07). Built ONLY while running;
  // when not running, agentMenu is undefined so TerminalPane's default
  // exited/not-found handling applies.
  //
  // Global scope (D-36): both Insert affordances are forced false — the menu
  // renders exactly ONE item (Stop, destructive) with no separator before a
  // lone item, and no Settings shortcut joins it.
  //
  // Both insert items reuse the SAME bracketed-paste mechanism (pasteApiRef →
  // term.paste, wrapped by xterm in \x1b[200~..\x1b[201~ because claude enables
  // mode 2004): the text is prefilled, NEVER auto-submitted — the user reviews
  // and presses Enter. Repeatable.
  const running = agentSession.status === "running";
  // Insert text items: task scope only. `description` is undefined and `seed`
  // is undefined on the global scope, so both flags are false there.
  const desc = description ?? "";
  const showInsertDescription = isTask && desc !== "" && running;
  const showInsertSeed = isTask && Boolean(seed) && running;
  const agentMenu = running ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Agent actions">
          <Ellipsis className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {showInsertDescription && (
          <DropdownMenuItem onSelect={() => pasteApiRef.current?.paste(desc)}>
            {`Insert description`}
          </DropdownMenuItem>
        )}
        {showInsertSeed && (
          <DropdownMenuItem
            onSelect={() => pasteApiRef.current?.paste(seed as string)}
          >
            {`Insert review prompt`}
          </DropdownMenuItem>
        )}
        {(showInsertDescription || showInsertSeed) && <DropdownMenuSeparator />}
        <DropdownMenuItem
          variant="destructive"
          onSelect={() => stopSession.mutate(agentSession.id)}
        >
          {`Stop`}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  ) : undefined;

  // D-45 optimistic clear: the open client reflects the waiting→idle
  // transition immediately on attach; the server clears authoritatively in
  // the WS attach path, other surfaces converge next poll. (D-09 removed the
  // connect-time PR-review-seed auto-paste — the seed now enters only via the
  // Insert review prompt menu item above.) The status entry lookup is
  // scope-aware: by taskId (task) or by source === "global" (global).
  const handleConnect = () => {
    queryClient.setQueryData<AgentStatusEntry[]>(["agent-statuses"], (old) =>
      old?.map((e) => {
        if (e.status !== "waiting") return e;
        const mine = isTask
          ? e.taskId === scope.taskId
          : e.source === "global";
        return mine ? { ...e, status: "idle" } : e;
      }),
    );
  };

  // State B — session exists (running or exited). Exited output stays
  // frozen, readable, copyable under the banner (D-15) but renders dimmed
  // so the dead agent looks disabled (checkpoint revision).
  //
  // D-54a within-run exited banner: when resumable, replace the default
  // single Reset action with the Reset (ghost, left) / Resume (primary,
  // rightmost) pair — same two mutations as the pre-start variant. When NOT
  // resumable, exitedActions stays undefined and the Phase 4 contract renders
  // verbatim (single Reset via exitedPrimaryLabel below). D-56: a failed
  // resume attaches normally, claude prints its error and exits — the next
  // poll reports resumable:false and the banner returns Reset-only. No masking
  // UI of any kind, no silent fallback to a fresh session.
  const resumePair = resumable ? (
    <>
      <Button variant="ghost" size="sm" disabled={busy} onClick={() => spawn.mutate()}>
        {spawn.isPending ? `Resetting…` : `Reset session`}
      </Button>
      <Button size="sm" disabled={busy} onClick={() => resume.mutate()}>
        {resume.isPending ? `Resuming…` : `Resume session`}
      </Button>
      {(spawn.isError || resume.isError) && !busy && (
        <span className="text-xs text-red-500">
          {`Couldn't start a session. Try again.`}
        </span>
      )}
    </>
  ) : undefined;

  return (
    <TerminalPane
      key={agentSession.id} // a fresh session id (from Reset or Resume) remounts the pane
      sessionId={agentSession.id}
      label={`Agent`}
      status={agentSession.status}
      exitCode={agentSession.exitCode}
      exitedPrimaryLabel={`Reset session`} // D-41 revised: "reset" wording, still a fresh claude
      exitedMessage={`Agent session ended.`} // code-free — users don't know exit codes; codes still drive dot colors
      exitedActions={resumePair} // D-54a: Reset/Resume pair when resumable; undefined → Phase 4 single-Reset contract
      dimWhenExited // exited agent terminal renders visually disabled; bash panes unchanged
      showExitedClose={false} // D-38: permanent tab, no Close
      onNewTerminal={() => spawn.mutate()} // "Reset session" — FRESH spawn
      onSessionExit={() =>
        // Scoped key either way — both spellings live under the ["sessions"]
        // prefix the scope-blind hooks invalidate (16-01 hook contract).
        queryClient.invalidateQueries({
          queryKey: isTask
            ? ["sessions", scope.taskId]
            : ["sessions", "global"],
        })
      }
      onReady={(api) => {
        pasteApiRef.current = api;
      }}
      onConnect={handleConnect}
      headerMenu={agentMenu}
    />
  );
}

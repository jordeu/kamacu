import { useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Ellipsis } from "lucide-react";
import type { Task } from "@/api/types";
import { useAgentStatuses, type AgentStatusEntry } from "@/api/agents";
import {
  useResumeAgent,
  useSpawnAgent,
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
 * The permanent Agent tab's content (D-38..D-41, revised at checkpoint
 * 2026-06-11): pre-start empty state, running TerminalPane with the
 * Insert-description header action, and the code-free exited banner
 * ("Agent session ended.") over a dimmed terminal whose single
 * "Reset session" action spawns a FRESH claude session (Phase 5 owns
 * recovery/resume). The pane NEVER spawns — all spawning happens here
 * from click handlers (StrictMode double-spawn guard).
 */
export function AgentTab({
  task,
  agentSession,
  seed,
}: {
  task: Task;
  agentSession: TermSession | undefined;
  projectId: number;
  // PR-review seed (Phase 12 D-06/D-07): when set, prefill (never send) this
  // prompt into the agent once, after the session connects.
  seed?: string;
}) {
  const spawn = useSpawnAgent(task.id);
  const resume = useResumeAgent(task.id);
  const stopSession = useStopSession();
  const queryClient = useQueryClient();
  const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null);

  // The server's resumable flag is the SINGLE driver for which pair renders —
  // the UI never infers resumability client-side (UI-SPEC). Read it from the
  // already-polled shared query (Phase 4 dedupe pattern; no new props).
  const resumable =
    (useAgentStatuses().data ?? []).find((e) => e.taskId === task.id)
      ?.resumable ?? false;
  // Mutual exclusion: while either Resume or Reset is in flight both buttons
  // disable, preventing a double-spawn racing the one-per-task 409 gate.
  const busy = spawn.isPending || resume.isPending;

  // State A — pre-start (D-40): no agent session has ever run.
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

    const hasWorktree = Boolean(task.worktree_path);
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
            <span className="font-mono">{task.branch}</span>
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

  // Agent ⋯ menu (D-06/D-07): folds the old standalone Insert-description
  // header button, the manual PR-review-prompt insert (D-09, no longer
  // auto-pasted on connect), and Stop into a single dropdown that supersedes
  // the pane's inline Stop button (headerMenu, D-07). Built ONLY while running;
  // when not running, agentMenu is undefined so TerminalPane's default
  // exited/not-found handling applies.
  //
  // Both insert items reuse the SAME bracketed-paste mechanism (pasteApiRef →
  // term.paste, wrapped by xterm in \x1b[200~..\x1b[201~ because claude enables
  // mode 2004): the text is prefilled, NEVER auto-submitted — the user reviews
  // and presses Enter. Repeatable.
  const running = agentSession.status === "running";
  // Insert description (D-35/36/37): only with a non-empty description.
  const showInsertDescription = task.description !== "" && running;
  // Insert review prompt (D-08): `seed` is already undefined for non-PR tasks
  // and blank templates (derived upstream in TaskPage), so gate on it directly.
  const showInsertSeed = Boolean(seed) && running;
  const agentMenu = running ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Agent actions">
          <Ellipsis className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {showInsertDescription && (
          <DropdownMenuItem
            onSelect={() => pasteApiRef.current?.paste(task.description)}
          >
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
  // Insert review prompt menu item above.)
  const handleConnect = () => {
    queryClient.setQueryData<AgentStatusEntry[]>(["agent-statuses"], (old) =>
      old?.map((e) =>
        e.taskId === task.id && e.status === "waiting"
          ? { ...e, status: "idle" }
          : e,
      ),
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
        queryClient.invalidateQueries({ queryKey: ["sessions", task.id] })
      }
      onReady={(api) => {
        pasteApiRef.current = api;
      }}
      onConnect={handleConnect}
      headerMenu={agentMenu}
    />
  );
}

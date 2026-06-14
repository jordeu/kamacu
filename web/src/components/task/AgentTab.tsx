import { useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Task } from "@/api/types";
import { useAgentStatuses, type AgentStatusEntry } from "@/api/agents";
import {
  useResumeAgent,
  useSpawnAgent,
  type TermSession,
} from "@/api/sessions";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { TerminalPane } from "@/components/terminal/TerminalPane";

// Agent session ids whose PR-review seed has already been injected. Module
// scope (not a ref) so reopening/remounting the review never re-injects —
// the guard is the SESSION, which is stable across mounts (12-07 fix #4).
const seededSessionIds = new Set<string>();

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
            {`Runs the claude CLI in this task's worktree on `}
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

  // Insert description (D-35/36/37): ONLY while running with a non-empty
  // description — otherwise hidden (not disabled). Pure frontend: xterm
  // wraps the paste in \x1b[200~..\x1b[201~ (claude enables mode 2004);
  // never submits, repeatable.
  const insertAction =
    task.description !== "" && agentSession.status === "running" ? (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => pasteApiRef.current?.paste(task.description)}
          >
            {`Insert description`}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {`Types the description into the prompt without sending`}
        </TooltipContent>
      </Tooltip>
    ) : undefined;

  // D-45 optimistic clear: the open client reflects the waiting→idle
  // transition immediately on attach; the server clears authoritatively in
  // the WS attach path, other surfaces converge next poll.
  const handleConnect = () => {
    queryClient.setQueryData<AgentStatusEntry[]>(["agent-statuses"], (old) =>
      old?.map((e) =>
        e.taskId === task.id && e.status === "waiting"
          ? { ...e, status: "idle" }
          : e,
      ),
    );

    // PR-review seed (D-06/D-07): prefill once per agent session after connect.
    // Reuses the exact Insert-description mechanism (pasteApiRef → term.paste,
    // wrapped by xterm in bracketed-paste \x1b[200~..\x1b[201~ so claude
    // receives it as a single un-submitted prompt). NEVER auto-sent — the user
    // edits and presses Enter. Guarded by the SESSION id at module scope
    // (12-07 fix #4): a freshly started session seeds once on first connect;
    // reopening the review re-mounts onto the same session id → already in the
    // set → never re-pastes; reconnects (same id) also never re-paste.
    if (seed && agentSession && !seededSessionIds.has(agentSession.id)) {
      seededSessionIds.add(agentSession.id);
      pasteApiRef.current?.paste(seed);
    }
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
      headerActions={insertAction}
    />
  );
}

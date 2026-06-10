import { useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Task } from "@/api/types";
import type { AgentStatusEntry } from "@/api/agents";
import { useSpawnAgent, type TermSession } from "@/api/sessions";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { TerminalPane } from "@/components/terminal/TerminalPane";

/**
 * The permanent Agent tab's content (D-38..D-41): pre-start empty state,
 * running TerminalPane with the Insert-description header action, and the
 * exited banner whose single "Start again" action spawns a FRESH claude
 * session (Phase 5 owns recovery). The pane NEVER spawns — all spawning
 * happens here from click handlers (StrictMode double-spawn guard).
 */
export function AgentTab({
  task,
  agentSession,
}: {
  task: Task;
  agentSession: TermSession | undefined;
  projectId: number;
}) {
  const spawn = useSpawnAgent(task.id);
  const queryClient = useQueryClient();
  const pasteApiRef = useRef<{ paste: (t: string) => void } | null>(null);

  // State A — pre-start (D-40): no agent session has ever run.
  if (!agentSession) {
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
  };

  // State B — session exists (running or exited). Exited output stays
  // frozen, readable, copyable under the banner (D-15).
  return (
    <TerminalPane
      key={agentSession.id} // fresh session ("Start again") remounts the pane
      sessionId={agentSession.id}
      label={`Agent`}
      status={agentSession.status}
      exitCode={agentSession.exitCode}
      exitedPrimaryLabel={`Start again`} // D-41: always a fresh claude this phase
      showExitedClose={false} // D-38: permanent tab, no Close
      onNewTerminal={() => spawn.mutate()} // "Start again" — FRESH spawn
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

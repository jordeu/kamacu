import { GitBranch } from "lucide-react";
import type { Task } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

/**
 * Worktree presence meta line (D-25/D-26) — single dense row between the
 * header and the tab strip. Three states derived from task fields:
 * worktree_path set -> active branch line; worktree_error set -> failed with
 * Retry; both null -> absent with Create worktree. Copy is the verbatim
 * UI-SPEC contract.
 */
export function WorktreeMetaLine({
  task,
  onCreate,
  creating,
}: {
  task: Task;
  onCreate: () => void;
  creating: boolean;
}) {
  // Active: branch in mono muted, git-branch icon, tooltip = absolute path.
  if (task.worktree_path) {
    return (
      <div className="flex min-w-0 items-center gap-1">
        <GitBranch className="size-3 shrink-0 text-zinc-400" />
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="truncate text-xs font-mono text-muted-foreground">
              {task.branch}
            </span>
          </TooltipTrigger>
          <TooltipContent className="font-mono">
            {task.worktree_path}
          </TooltipContent>
        </Tooltip>
      </div>
    );
  }

  // Failed: destructive message (the message carries the color — the button
  // is standard outline, never red).
  if (task.worktree_error) {
    return (
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate text-xs text-red-500">
          Couldn't create a worktree: {task.worktree_error}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={creating}
          onClick={onCreate}
        >
          {creating ? "Creating…" : "Retry creation"}
        </Button>
      </div>
    );
  }

  // Absent (D-26): neutral — absence is not an error.
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span className="text-xs text-muted-foreground">No worktree yet.</span>
      <Button
        variant="outline"
        size="sm"
        disabled={creating}
        onClick={onCreate}
      >
        {creating ? "Creating…" : "Create worktree"}
      </Button>
    </div>
  );
}

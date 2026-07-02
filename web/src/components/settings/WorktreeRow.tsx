import { useState } from "react";
import { Check, Copy, TriangleAlert } from "lucide-react";
import {
  useClearPointer,
  type WorktreeRow as WorktreeRowData,
} from "@/api/worktreeCleanup";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ForceRemoveDialog } from "@/components/settings/ForceRemoveDialog";

interface WorktreeRowProps {
  row: WorktreeRowData;
}

/** Copy of the PR-state suffix — a quiet ` · merged`/` · closed`/` · open`
 *  appended after `PR #<n>`, and only when pr_state is known (never a broken
 *  chip when gh is off — Research Open Q1). */
function prSuffix(state: WorktreeRowData["pr_state"]): string {
  return state ? ` · ${state}` : "";
}

/**
 * One worktree row in the cleanup panel (WTREE-01). Layout:
 * `[classification] [path (flex-1, truncate)] [flag chips] [association] [action]`.
 *
 * Classification badge is muted for Referenced/Orphan/Stale and red only for
 * Blocked (UI-SPEC § Color). Flag chips (Dirty/Unpushed/Stash) render only when
 * their count > 0 — a pristine row is quiet. The Remove action opens the
 * force-remove confirm; a `blocked` remove degrades to the inline D-01 banner
 * with a COPYABLE `sudo rm -rf <path>` hint — the app never runs the command
 * (D-01, Security V5). A stale-pointer row offers Clear pointer instead.
 */
export function WorktreeRow({ row }: WorktreeRowProps) {
  const clearPointer = useClearPointer();

  // Blocked is seeded from the server row and can also become true after a
  // remove attempt returns {outcome:"blocked"} (tracked here so the banner
  // shows without a full refetch round-trip).
  const [blocked, setBlocked] = useState(row.blocked);
  const [blockedPath, setBlockedPath] = useState<string | null>(
    row.blocked_path,
  );
  const [removeOpen, setRemoveOpen] = useState(false);
  const [clearOpen, setClearOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const isStale = row.classification === "stale";

  const classificationBadge = blocked ? (
    <Badge variant="destructive">{`Blocked`}</Badge>
  ) : row.classification === "referenced" ? (
    <Badge variant="secondary">{`Referenced`}</Badge>
  ) : row.classification === "orphan" ? (
    <Badge variant="outline" className="text-muted-foreground">
      {`Orphan`}
    </Badge>
  ) : (
    <Badge variant="outline">{`Stale pointer`}</Badge>
  );

  // The copyable sudo hint's path: the offending dir if the server named one,
  // else the worktree path itself.
  const hintPath = blockedPath ?? row.path;

  async function copyHint() {
    // D-01 / T-23-12: pure text to the clipboard — the app NEVER spawns, evals,
    // or shells the command.
    await navigator.clipboard.writeText(`sudo rm -rf ${hintPath}`);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div
      className={
        blocked
          ? "border-l-2 border-destructive/40 pl-2"
          : undefined
      }
    >
      <div className="flex min-h-[40px] items-center gap-2 px-1 py-2 hover:bg-muted/50">
        {classificationBadge}

        <span
          className="flex-1 truncate font-mono text-sm"
          title={row.path}
        >
          {row.path}
        </span>

        {/* Flag chips — only when the count is present and > 0 (quiet by default). */}
        {row.dirty > 0 && (
          <Badge variant="outline">
            <TriangleAlert className="text-amber-400" />
            {`Dirty`}
          </Badge>
        )}
        {row.unpushed === null ? (
          <Badge variant="outline">{`Unpushed?`}</Badge>
        ) : (
          row.unpushed > 0 && <Badge variant="outline">{`Unpushed`}</Badge>
        )}
        {row.stash > 0 && <Badge variant="outline">{`Stash`}</Badge>}

        {/* Association: Task/PR text for referenced rows; nothing for orphans. */}
        {row.association && (
          <span className="text-sm text-muted-foreground whitespace-nowrap">
            {row.association}
            {row.pr_state !== null && prSuffix(row.pr_state)}
          </span>
        )}

        {/* Row action: Remove (referenced/orphan) / Clear pointer (stale). A
            blocked row has no active Remove — the banner below is the action. */}
        {!blocked &&
          (isStale ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setClearOpen(true)}
            >
              {`Clear pointer`}
            </Button>
          ) : (
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setRemoveOpen(true)}
            >
              {`Remove`}
            </Button>
          ))}
      </div>

      {/* D-01 blocked-row banner: informational, never a red error. */}
      {blocked && (
        <div className="mb-1 flex items-start gap-2 rounded-md border-l-2 border-destructive/40 bg-destructive/5 p-3">
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" />
          <div className="min-w-0 space-y-2">
            <p className="text-sm">
              {`Blocked — needs manual removal. A directory inside this worktree is owned by another user and can't be removed by Kamacu.`}
            </p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate font-mono text-xs">
                {`sudo rm -rf ${hintPath}`}
              </code>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Copy command`}
                onClick={() => void copyHint()}
              >
                {copied ? (
                  <Check className="size-4" />
                ) : (
                  <Copy className="size-4" />
                )}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Force-remove confirm (referenced/orphan). */}
      {!isStale && (
        <ForceRemoveDialog
          open={removeOpen}
          onOpenChange={setRemoveOpen}
          row={row}
          onBlocked={(path) => {
            setBlocked(true);
            setBlockedPath(path);
          }}
        />
      )}

      {/* Clear-stale-pointer confirm (D-07) — non-destructive, deletes no files. */}
      {isStale && (
        <AlertDialog open={clearOpen} onOpenChange={setClearOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{`Clear stale pointer?`}</AlertDialogTitle>
              <AlertDialogDescription>
                {`The task's worktree link points to a directory that no longer exists. This clears the link and prunes the stale git registration. The task and its branch are kept.`}
              </AlertDialogDescription>
            </AlertDialogHeader>
            {clearPointer.isError && (
              <p className="text-xs text-destructive">
                {`Couldn't clear the pointer: ${clearPointer.error.message}`}
              </p>
            )}
            <AlertDialogFooter>
              <AlertDialogCancel>{`Cancel`}</AlertDialogCancel>
              <AlertDialogAction
                disabled={clearPointer.isPending}
                onClick={(e) => {
                  e.preventDefault();
                  if (clearPointer.isPending) return;
                  clearPointer.mutate(
                    { task_id: row.task_id },
                    { onSuccess: () => setClearOpen(false) },
                  );
                }}
              >
                {clearPointer.isPending ? `Clearing…` : `Clear pointer`}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </div>
  );
}

import { useEffect, useState } from "react";
import { TriangleAlert } from "lucide-react";
import { useRemoveWorktree, type WorktreeRow } from "@/api/worktreeCleanup";
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
import { Input } from "@/components/ui/input";

interface ForceRemoveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The row to force-remove; carries dirty/sessions/branch/unpushed/stash/path. */
  row: WorktreeRow;
  /** Called with the offending path when the remove returns {outcome:"blocked"} —
   *  the row then renders the D-01 banner instead of a red error. */
  onBlocked: (path: string) => void;
}

/**
 * The WTREE-02 / D-03 force-remove confirm. It mirrors CleanupWorktreeDialog:
 * it names EXACTLY what will be destroyed (sessions / dirty / unpushed / stash)
 * before committing, always keeps the branch (last line of every variant, D-02),
 * and type-gates when the worktree is dirty (uncommitted work at risk, D-33).
 *
 * The row already carries a point-in-time snapshot; per Pitfall 8 the SERVER
 * re-checks every gate at remove time regardless of this snapshot. A `blocked`
 * outcome (D-01) is NOT an error — it closes the dialog and surfaces the row's
 * blocked banner; a real ApiError renders inline (mirrors CleanupWorktreeDialog).
 */
export function ForceRemoveDialog({
  open,
  onOpenChange,
  row,
  onBlocked,
}: ForceRemoveDialogProps) {
  const remove = useRemoveWorktree();
  const [confirmText, setConfirmText] = useState("");

  // Fresh dialog on every open/close flip: clear the typed confirmation and
  // any prior mutation error.
  const resetRemove = remove.reset;
  useEffect(() => {
    setConfirmText("");
    resetRemove();
  }, [open, resetRemove]);

  const { sessions, dirty, unpushed, stash, branch } = row;
  // Type-to-confirm target = the worktree DIRECTORY NAME (basename of path).
  const target = row.path.split("/").pop() ?? "";

  // The gate (D-33): exact, case-sensitive match required while dirty.
  const gateOpen = dirty === 0 || confirmText === target;
  const confirmDisabled = remove.isPending || !gateOpen;

  function handleConfirm() {
    if (remove.isPending) return;
    // D-03: force + stop_sessions are both true for the single per-item override.
    remove.mutate(
      {
        repo: row.repo,
        path: row.path,
        task_id: row.task_id,
        force: true,
        stop_sessions: true,
      },
      {
        onSuccess: (result) => {
          if (result.outcome === "blocked") {
            // D-01: not a red error — close and let the row show the banner.
            onOpenChange(false);
            onBlocked(result.path ?? row.path);
            return;
          }
          onOpenChange(false);
        },
        // A real error renders inline (remove.isError below) — dialog stays open.
      },
    );
  }

  const ctaLabel = remove.isPending
    ? "Removing…"
    : sessions > 0
      ? "Stop sessions and force-remove"
      : "Force-remove";

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{`Force-remove this worktree?`}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2 text-left">
              <p className="font-mono text-xs break-all">{row.path}</p>
              {sessions > 0 && (
                <p>
                  {sessions === 1
                    ? `1 session running in this worktree will be stopped.`
                    : `${sessions} sessions running in this worktree will be stopped.`}
                </p>
              )}
              {dirty > 0 && (
                <div className="flex items-start gap-1 px-3">
                  <TriangleAlert className="mt-0.5 size-4 shrink-0 text-amber-400" />
                  <span className="text-zinc-50">
                    {`This worktree has ${dirty === 1 ? "1 changed file" : `${dirty} changed files`} with uncommitted work. Removing it permanently deletes those changes.`}
                  </span>
                </div>
              )}
              {unpushed !== null && unpushed > 0 && (
                <p>
                  {unpushed === 1
                    ? `1 unpushed commit will be lost.`
                    : `${unpushed} unpushed commits will be lost.`}
                </p>
              )}
              {stash > 0 && (
                <p>
                  {stash === 1
                    ? `1 stash entry will be lost.`
                    : `${stash} stash entries will be lost.`}
                </p>
              )}
              {/* Branch-kept reassurance — last line of EVERY variant (D-02/D-34). */}
              <p>
                {`The branch `}
                <span className="font-mono">{branch}</span>
                {` is kept.`}
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>

        {dirty > 0 && (
          <div className="space-y-2">
            <label
              htmlFor="force-remove-confirm-input"
              className="block text-xs font-medium text-muted-foreground"
            >
              {`Type `}
              <span className="font-mono">{target}</span>
              {` to confirm`}
            </label>
            <Input
              id="force-remove-confirm-input"
              autoFocus
              value={confirmText}
              className="font-mono"
              onChange={(e) => setConfirmText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && gateOpen && !remove.isPending) {
                  e.preventDefault();
                  handleConfirm();
                }
              }}
            />
          </div>
        )}

        {remove.isError && (
          <p className="text-xs text-destructive">
            {`Couldn't remove the worktree: ${remove.error.message}`}
          </p>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel>{`Cancel`}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={confirmDisabled}
            onClick={(e) => {
              // Keep the dialog open until the mutation lands (pending state,
              // inline failure) — mirror CleanupWorktreeDialog.
              e.preventDefault();
              handleConfirm();
            }}
          >
            {ctaLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

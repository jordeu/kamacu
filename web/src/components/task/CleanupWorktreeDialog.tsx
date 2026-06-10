import { useEffect, useState } from "react";
import { TriangleAlert } from "lucide-react";
import { useCleanupWorktree, useWorktreeState } from "@/api/worktrees";
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
import { Skeleton } from "@/components/ui/skeleton";

interface CleanupWorktreeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: number;
  taskTitle: string;
  projectId: number;
  /** "done" = board move-to-Done trigger (cancel reads `Keep worktree`); "menu" = ellipsis item (cancel reads `Cancel`). */
  trigger: "done" | "menu";
}

/**
 * The GIT-02/GIT-03 cleanup dialog (D-31..D-34): one AlertDialog whose body
 * composes from FRESH server state into the four UI-SPEC variants —
 * (a) clean+idle, (b) sessions running, (c) uncommitted changes,
 * (d) dirty AND sessions. State is fetched when the dialog opens (Pitfall 8:
 * never from board cache), so the variant shown reflects reality at confirm
 * time. Declining does nothing — there is no silent code path that stops
 * sessions or removes a worktree without the explicit confirm (D-32).
 */
export function CleanupWorktreeDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
  projectId,
  trigger,
}: CleanupWorktreeDialogProps) {
  // Variant derives from the RESPONSE, never from cached task/board data.
  const state = useWorktreeState(taskId, open);
  const cleanup = useCleanupWorktree(taskId, projectId);

  const [confirmText, setConfirmText] = useState("");

  // Fresh dialog on every open/close flip: clear the typed confirmation and
  // any prior mutation error.
  const resetCleanup = cleanup.reset;
  useEffect(() => {
    setConfirmText("");
    resetCleanup();
  }, [open, resetCleanup]);

  // Worktree vanished between trigger and open — nothing to clean up, close
  // silently.
  useEffect(() => {
    if (open && state.error && state.error.status === 404) {
      onOpenChange(false);
    }
  }, [open, state.error, onOpenChange]);

  const loading = !state.data;
  const sessions = state.data?.running_sessions ?? 0;
  const dirty = state.data?.dirty_files ?? 0;
  const branch = state.data?.branch ?? "";
  // Type-to-confirm target = the worktree DIRECTORY NAME (basename of path).
  const target = state.data?.path.split("/").pop() ?? "";

  // The gate (D-33): exact, case-sensitive match required while dirty.
  const gateOpen = dirty === 0 || confirmText === target;
  const confirmDisabled = loading || cleanup.isPending || !gateOpen;

  function handleConfirm() {
    if (!state.data || cleanup.isPending) return;
    cleanup.mutate(
      { stop_sessions: sessions > 0, force: dirty > 0 },
      {
        onSuccess: () => {
          // Hook invalidations land the task in the absent state; bash tabs
          // vanish because their sessions were stopped.
          onOpenChange(false);
        },
        onError: () => {
          // A 409 raced-state error means reality moved (Pitfall 8 server
          // re-check showing through) — refetch so the variant re-derives.
          void state.refetch();
        },
      },
    );
  }

  const ctaLabel = cleanup.isPending
    ? "Cleaning up…"
    : sessions > 0
      ? "Stop sessions and clean up"
      : "Remove worktree";

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Clean up worktree?</AlertDialogTitle>
          {loading ? (
            <Skeleton className="h-4 w-full" />
          ) : (
            <AlertDialogDescription asChild>
              <div className="space-y-2 text-left">
                {sessions === 0 && dirty === 0 && (
                  <p>{`The worktree for "${taskTitle}" will be removed.`}</p>
                )}
                {sessions > 0 && (
                  <p>
                    {sessions === 1
                      ? "1 session running in this worktree will be stopped."
                      : `${sessions} sessions running in this worktree will be stopped.`}
                  </p>
                )}
                {sessions > 0 && dirty === 0 && (
                  <p>The worktree will be removed.</p>
                )}
                {dirty > 0 && (
                  <div className="flex items-start gap-1 px-3">
                    <TriangleAlert className="mt-0.5 size-4 shrink-0 text-zinc-400" />
                    <span className="text-zinc-50">
                      {`This worktree has ${dirty === 1 ? "1 changed file" : `${dirty} changed files`} with uncommitted work. Removing it permanently deletes those changes.`}
                    </span>
                  </div>
                )}
                {/* Branch-kept reassurance — last line of EVERY variant (D-34). */}
                <p>
                  The branch <span className="font-mono">{branch}</span>
                  {" is kept."}
                </p>
              </div>
            </AlertDialogDescription>
          )}
        </AlertDialogHeader>

        {!loading && dirty > 0 && (
          <div className="space-y-2">
            <label
              htmlFor="cleanup-confirm-input"
              className="block text-xs font-medium text-muted-foreground"
            >
              Type <span className="font-mono">{target}</span> to confirm
            </label>
            <Input
              id="cleanup-confirm-input"
              autoFocus
              value={confirmText}
              className="font-mono"
              onChange={(e) => setConfirmText(e.target.value)}
              onKeyDown={(e) => {
                // Enter activates the confirm only when the gate is open.
                if (e.key === "Enter" && gateOpen && !cleanup.isPending) {
                  e.preventDefault();
                  handleConfirm();
                }
              }}
            />
          </div>
        )}

        {cleanup.isError && (
          <p className="text-xs text-red-500">
            {`Couldn't remove the worktree: ${cleanup.error.message}`}
          </p>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>
            {trigger === "done" ? "Keep worktree" : "Cancel"}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={confirmDisabled}
            onClick={(e) => {
              // Keep the dialog open until the cleanup lands (pending state,
              // inline failure) — mirror DeleteTaskDialog.
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

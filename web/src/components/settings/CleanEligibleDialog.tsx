import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  cleanEligible,
  useCleanEligiblePreview,
  type CleanupProgress,
  type EligibleItem,
} from "@/api/worktreeCleanup";
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
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

const REASON_LABEL: Record<EligibleItem["reason"], string> = {
  orphaned: "Orphaned",
  done: "Done",
  pr_merged: "PR merged",
  pr_closed: "PR closed",
};

export function CleanEligibleDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [cleaning, setCleaning] = useState(false);
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!cleaning) onOpenChange(next);
      }}
    >
      <AlertDialogContent
        className="max-h-[90dvh] overflow-y-auto data-[size=default]:max-w-[calc(100%-2rem)] data-[size=default]:sm:max-w-lg"
        onEscapeKeyDown={(event) => {
          if (cleaning) event.preventDefault();
        }}
      >
        {open && <CleanEligibleBody onCleaningChange={setCleaning} />}
      </AlertDialogContent>
    </AlertDialog>
  );
}

function CleanEligibleBody({
  onCleaningChange,
}: {
  onCleaningChange: (value: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [includeUnpushed, setIncludeUnpushed] = useState(false);
  const [includeDirty, setIncludeDirty] = useState(false);
  const [cleaning, setCleaning] = useState(false);
  const [progress, setProgress] = useState<CleanupProgress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const options = {
    include_unpushed: includeUnpushed,
    include_dirty: includeDirty,
  };
  const preview = useCleanEligiblePreview(options, !cleaning && !progress);
  const items = preview.data?.items ?? [];
  const finished = progress?.done === true;
  const ready =
    !preview.isFetching &&
    !preview.isError &&
    items.length > 0 &&
    !cleaning &&
    !progress;

  async function handleConfirm() {
    if (!ready) return;
    setError(null);
    setCleaning(true);
    onCleaningChange(true);
    setProgress({
      total: items.length,
      completed: 0,
      removed: 0,
      skipped: 0,
      done: false,
    });
    try {
      await cleanEligible(options, items, setProgress);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Cleanup was interrupted. Refresh the preview to check what remains.",
      );
    } finally {
      setCleaning(false);
      onCleaningChange(false);
      void queryClient.invalidateQueries({ queryKey: ["worktrees"] });
    }
  }

  function refreshPreview() {
    setError(null);
    setProgress(null);
    void preview.refetch();
  }

  return (
    <>
      <AlertDialogHeader className="place-items-start text-left">
        <AlertDialogTitle>
          {finished
            ? "Cleanup complete"
            : cleaning
              ? "Cleaning worktrees…"
              : "Clean eligible worktrees"}
        </AlertDialogTitle>
        <AlertDialogDescription>
          Remove worktrees for completed tasks, merged or closed PRs, and
          worktrees without a task. Running sessions are always skipped. Tasks
          and Git branches are kept.
        </AlertDialogDescription>
      </AlertDialogHeader>

      {!progress && (
        <>
          <fieldset disabled={cleaning} className="space-y-3">
            <legend className="mb-3 text-sm font-medium">Also include</legend>
            <label className="flex cursor-pointer items-start gap-3 text-sm">
              <Checkbox
                className="mt-0.5"
                checked={includeUnpushed}
                onCheckedChange={(value) => setIncludeUnpushed(value === true)}
                aria-describedby="cleanup-unpushed-hint"
              />
              <span>
                <span className="font-medium">
                  Worktrees with unpushed commits or stashes
                </span>
                <span
                  id="cleanup-unpushed-hint"
                  className="mt-1 block text-muted-foreground"
                >
                  Local commits stay on their branches. Repository stashes are
                  kept.
                </span>
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-3 text-sm">
              <Checkbox
                className="mt-0.5"
                checked={includeDirty}
                onCheckedChange={(value) => setIncludeDirty(value === true)}
                aria-describedby="cleanup-dirty-hint"
              />
              <span>
                <span className="font-medium">
                  Worktrees with uncommitted changes
                </span>
                <span
                  id="cleanup-dirty-hint"
                  className={`mt-1 block ${includeDirty ? "text-destructive" : "text-muted-foreground"}`}
                >
                  Uncommitted changes and untracked files in these worktrees
                  will be permanently deleted.
                </span>
              </span>
            </label>
          </fieldset>
          <section
            aria-label="Worktrees to remove"
            aria-busy={preview.isFetching}
            className="border-t pt-4"
          >
            {preview.isFetching ? (
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">
                  Checking eligible worktrees…
                </p>
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-3/4" />
              </div>
            ) : preview.isError ? (
              <div role="alert" className="space-y-2">
                <p className="text-sm text-destructive">
                  Could not load the preview. {preview.error.message}
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void preview.refetch()}
                >
                  Try again
                </Button>
              </div>
            ) : items.length === 0 ? (
              <p role="status" className="text-sm text-muted-foreground">
                No worktrees match these options. Worktrees with running
                sessions or active tasks remain protected.
              </p>
            ) : (
              <>
                <p role="status" className="mb-2 text-sm font-medium">
                  {items.length} {items.length === 1 ? "worktree" : "worktrees"}{" "}
                  to remove
                </p>
                <ul className="max-h-48 divide-y overflow-y-auto">
                  {items.map((item) => (
                    <li
                      key={`${item.repo}:${item.path}`}
                      className="py-2 text-sm"
                    >
                      <div className="flex flex-wrap justify-between gap-x-3 gap-y-1">
                        <span className="font-medium">{item.project_name}</span>
                        <span className="text-muted-foreground">
                          {REASON_LABEL[item.reason]}
                        </span>
                      </div>
                      <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                        {item.path}
                      </p>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </section>
        </>
      )}

      {progress && (
        <section className="space-y-3" aria-label="Cleanup progress">
          <div
            className="flex justify-between gap-3 text-sm"
            role="status"
            aria-live="polite"
          >
            <span>
              {finished
                ? "All worktrees checked"
                : progress.total === 0
                  ? "Checking worktrees…"
                  : `${progress.completed} of ${progress.total} checked`}
            </span>
            <span className="tabular-nums">
              {progress.total > 0
                ? `${Math.round((progress.completed / progress.total) * 100)}%`
                : finished
                  ? "100%"
                  : ""}
            </span>
          </div>
          <progress
            aria-label="Worktree cleanup"
            className="block h-2 w-full appearance-none overflow-hidden rounded-full bg-muted [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-primary [&::-moz-progress-bar]:bg-primary"
            max={progress.total || 1}
            value={finished && progress.total === 0 ? 1 : progress.completed}
          />
          <p className="text-sm">
            {progress.removed} removed · {progress.skipped} skipped
          </p>
          {progress.skipped > 0 && (
            <p className="text-sm text-muted-foreground">
              Skipped worktrees were no longer eligible or could not be removed.
              Refresh the worktree list to review them.
            </p>
          )}
          {cleaning && (
            <p className="text-sm text-muted-foreground">
              Keep this dialog open until cleanup finishes.
            </p>
          )}
        </section>
      )}
      {error && (
        <div role="alert" className="space-y-2">
          <p className="text-sm text-destructive">{error}</p>
          <Button variant="outline" size="sm" onClick={refreshPreview}>
            Refresh preview
          </Button>
        </div>
      )}

      <AlertDialogFooter>
        <AlertDialogCancel disabled={cleaning}>
          {finished ? "Done" : progress ? "Close" : "Cancel"}
        </AlertDialogCancel>
        {!progress && (
          <AlertDialogAction
            variant={includeDirty ? "destructive" : "default"}
            disabled={!ready}
            onClick={(event) => {
              event.preventDefault();
              void handleConfirm();
            }}
          >
            {`Remove ${items.length} ${items.length === 1 ? "worktree" : "worktrees"}`}
          </AlertDialogAction>
        )}
      </AlertDialogFooter>
    </>
  );
}

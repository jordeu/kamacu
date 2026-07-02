import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { useTaskDiff } from "@/api/diffs";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { DiffFileSection } from "@/components/task/DiffFileSection";

/**
 * The read-only Diff tab (REVW-01): worktree changes vs the base branch as a
 * totals bar + collapsible per-file unified diffs (05-UI-SPEC Layout & Color).
 *
 * Single vertical scroll container at full main-area width (code needs width —
 * NOT the 860px Description island). The colored ± lines are the loudest thing
 * on the page by design; all diff chrome stays neutral.
 *
 * Fetch policy (D-61): the query fetches on mount (this component mounts only
 * while the tab is active) and on the manual refresh button — never polls.
 * While refetching with data present, current content stays rendered and only
 * the refresh button spins.
 */
export function DiffTab({ taskId }: { taskId: number }) {
  const { data, isPending, isError, error, isFetching, refetch } =
    useTaskDiff(taskId);

  // First-load "Loading diff…" only renders after a 150ms delay (no flash —
  // mirrors the TerminalPane showConnecting pattern).
  const [showLoading, setShowLoading] = useState(false);
  useEffect(() => {
    if (!(isPending && !data)) {
      setShowLoading(false);
      return;
    }
    const timer = window.setTimeout(() => setShowLoading(true), 150);
    return () => window.clearTimeout(timer);
  }, [isPending, data]);

  if (isError && !data) {
    return (
      <div className="flex h-full min-h-0 flex-col items-center justify-center gap-2 px-4 py-8 text-center">
        <p className="text-sm">{`Couldn't load the diff.`}</p>
        <p className="text-xs text-muted-foreground">{error.message}</p>
        <Button variant="outline" size="sm" onClick={() => refetch()}>
          Reload diff
        </Button>
      </div>
    );
  }

  if (isPending && !data) {
    return (
      <div className="flex h-full min-h-0 items-center justify-center px-4 py-8 text-center">
        {showLoading && (
          <p className="text-sm text-muted-foreground">{`Loading diff…`}</p>
        )}
      </div>
    );
  }

  if (!data) return null;

  const { base, totals, files } = data;

  if (files.length === 0) {
    return (
      <div className="flex h-full min-h-0 items-center justify-center px-4 py-8 text-center">
        <p className="text-sm text-muted-foreground">
          {`No changes yet. The worktree matches `}
          <span className="font-mono">{base}</span>
          {`.`}
        </p>
      </div>
    );
  }

  const fileCount = `${totals.files} ${
    totals.files === 1 ? "file changed" : "files changed"
  }`;

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
      <div className="sticky top-0 z-10 flex items-center border-b border-border bg-background px-3 py-2">
        <p className="flex-1 text-sm text-zinc-50">
          {fileCount}
          {totals.additions > 0 && (
            <>
              {", "}
              <span className="text-green-400 tabular-nums">{`+${totals.additions}`}</span>
            </>
          )}
          {totals.deletions > 0 && (
            <>
              {" "}
              <span className="text-red-400 tabular-nums">{`−${totals.deletions}`}</span>
            </>
          )}
          {" vs "}
          <span className="font-mono">{base}</span>
        </p>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Refresh diff"
              disabled={isFetching}
              onClick={() => refetch()}
            >
              <RefreshCw
                className={
                  isFetching
                    ? "size-4 animate-spin motion-reduce:animate-none"
                    : "size-4"
                }
              />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refresh diff</TooltipContent>
        </Tooltip>
      </div>

      <div className="flex flex-col gap-2 pt-2">
        {files.map((file) => (
          // key=path:hash so a changed file (new hash) remounts un-viewed +
          // expanded (DIFF-04 auto-reset).
          <DiffFileSection
            key={`${file.path}:${file.hash}`}
            file={file}
            taskId={taskId}
          />
        ))}
      </div>
    </div>
  );
}

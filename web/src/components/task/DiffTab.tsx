import { useCallback, useEffect, useRef, useState } from "react";
import { PanelLeft, RefreshCw } from "lucide-react";
import { useTaskDiff } from "@/api/diffs";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { DiffFileSection } from "@/components/task/DiffFileSection";
import { FileTree } from "@/components/task/FileTree";
import { useScrollSpy } from "@/hooks/use-scroll-spy";

/**
 * The read-only Diff tab (REVW-01 / DIFF-01): worktree changes vs the base
 * branch as a GitHub "Files changed" two-pane layout — a fixed 288px left file
 * tree (nested, path-compressed, folders-default-expanded) beside the existing
 * single-scroll right pane (sticky totals bar + collapsible per-file unified
 * diffs). The colored ± lines stay the loudest thing on the page; all chrome
 * stays neutral (05/22-UI-SPEC Layout & Color).
 *
 * Tree ↔ diff wiring (deterministic, UI-SPEC Decision 1):
 * - Clicking a tree row is SCROLL-ONLY (`scrollToPath`) — it never mutates the
 *   file's collapse or Viewed state.
 * - A scroll-spy (one IntersectionObserver rooted on the right pane) highlights
 *   whichever file is at the top of the scrolled viewport.
 * - A `PanelLeft` toggle in the totals bar hides/shows the tree pane.
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
  // Tree pane visibility, toggled by the PanelLeft button.
  const [treeVisible, setTreeVisible] = useState(true);
  // The right-pane scroll container: IntersectionObserver root + scroll target.
  const scrollRef = useRef<HTMLDivElement>(null);

  // Safe file list for the hooks below (data is undefined during first load).
  const files = data?.files ?? [];
  // Joined path key re-observes the scroll-spy when the file list changes.
  const activePath = useScrollSpy(scrollRef, files.map((f) => f.path).join("\n"));

  useEffect(() => {
    if (!(isPending && !data)) {
      setShowLoading(false);
      return;
    }
    const timer = window.setTimeout(() => setShowLoading(true), 150);
    return () => window.clearTimeout(timer);
  }, [isPending, data]);

  // Scroll-only jump to a file's diff (UI-SPEC Decision 1). The scroll-mt-[41px]
  // on each section wrapper (Plan 03) lands the header under the sticky totals
  // bar; CSS.escape guards paths with special characters (T-22-08). Never
  // mutates collapse or Viewed.
  const scrollToPath = useCallback((path: string) => {
    const container = scrollRef.current;
    if (!container) return;
    const el = container.querySelector(
      `[data-diff-path="${CSS.escape(path)}"]`,
    );
    const reduce = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    ).matches;
    el?.scrollIntoView({ block: "start", behavior: reduce ? "auto" : "smooth" });
  }, []);

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

  const { base, totals } = data;

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
    <div className="flex h-full min-h-0">
      {treeVisible && (
        <div className="w-72 shrink-0 overflow-y-auto border-r border-border">
          <FileTree
            files={files}
            activePath={activePath}
            onSelect={scrollToPath}
          />
        </div>
      )}

      <div className="flex min-h-0 flex-1 flex-col">
        {/* Totals bar is a FIXED header OUTSIDE the scroll pane below — nothing
            can scroll above it. Per-file headers pin to the scroll pane's top
            (DiffFileSection sticky top-0), tucked right under this bar. */}
        <div className="flex shrink-0 items-center gap-1 border-b border-border bg-background px-3 py-2">
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
                aria-label={treeVisible ? "Hide file tree" : "Show file tree"}
                onClick={() => setTreeVisible((v) => !v)}
              >
                <PanelLeft className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {treeVisible ? "Hide file tree" : "Show file tree"}
            </TooltipContent>
          </Tooltip>
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

        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto">
          <div className="flex flex-col">
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
      </div>
    </div>
  );
}

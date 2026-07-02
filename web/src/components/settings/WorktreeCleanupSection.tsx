import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { useWorktreeList } from "@/api/worktreeCleanup";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProjectAvatar } from "@/components/ui/ProjectAvatar";
import { WorktreeRow } from "@/components/settings/WorktreeRow";
import { CleanEligibleDialog } from "@/components/settings/CleanEligibleDialog";

const SECTION_HELP = `Every git worktree across all projects. Removing a worktree never deletes its branch.`;
const EMPTY = `No worktrees to clean up. Every task and PR worktree is accounted for.`;

/**
 * The WTREE-01..04 cleanup panel — one more Settings <section> in the 640px
 * column (D-09). It owns the single ["worktrees"] query (fetch-on-mount, no
 * poll — D-10): the Refresh button spins on refetch, and every mutation in the
 * child dialogs invalidates the key so the list re-reads reality after any
 * action. It groups worktrees per project (server-provided grouping; the main
 * worktree is excluded server-side) and renders a WorktreeRow per entry.
 *
 * States are scoped to the section (never blank the whole Settings page): a
 * loading skeleton on first load, a spin-only Refresh on a refetch-with-data,
 * an empty line when nothing is listed, and an inline Retry on a load error.
 */
export function WorktreeCleanupSection() {
  const { data, isLoading, isError, isFetching, refetch } = useWorktreeList();
  const [cleanOpen, setCleanOpen] = useState(false);

  const total = data?.counts.total ?? 0;
  const orphaned = data?.counts.orphaned ?? 0;

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Worktree cleanup`}</h2>
      <p className="text-xs text-muted-foreground">{SECTION_HELP}</p>

      {/* Header controls: [count (flex-1)] [Refresh] [Clean eligible]. */}
      <div className="flex items-center gap-2">
        <span className="flex-1 text-sm text-muted-foreground">
          {`${total} worktrees`}
          {orphaned > 0 && ` · ${orphaned} orphaned`}
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Refresh worktrees`}
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
          <TooltipContent>{`Refresh worktrees`}</TooltipContent>
        </Tooltip>
        <Button
          variant="outline"
          size="sm"
          disabled={isLoading || total === 0}
          onClick={() => setCleanOpen(true)}
        >
          {`Clean eligible`}
        </Button>
      </div>

      {isError && !data ? (
        <div className="flex items-center gap-3">
          <p className="text-xs text-destructive">{`Couldn't load worktrees.`}</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {`Retry`}
          </Button>
        </div>
      ) : isLoading || !data ? (
        <div className="flex flex-col gap-2">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      ) : total === 0 ? (
        <p className="text-sm text-muted-foreground">{EMPTY}</p>
      ) : (
        <div className="flex flex-col gap-6">
          {data.projects.map((group) => (
            <div key={group.project_id} className="flex flex-col gap-1">
              <div className="flex items-center gap-2">
                <ProjectAvatar
                  size="inline"
                  letters={group.icon_letters}
                  color={group.icon_color}
                />
                <span className="text-sm">{group.project_name}</span>
              </div>
              {group.worktrees.map((row) => (
                <WorktreeRow key={`${row.repo}:${row.path}`} row={row} />
              ))}
            </div>
          ))}
        </div>
      )}

      <CleanEligibleDialog open={cleanOpen} onOpenChange={setCleanOpen} />
    </section>
  );
}

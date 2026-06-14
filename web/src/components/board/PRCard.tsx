import { ExternalLink } from "lucide-react";
import { useNavigate } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAgo } from "@/lib/time";
import { useOpenReview, type PRSummary } from "@/api/pullRequests";

/** Checks-dot visual language echoed from StatusDot's dotMeta (D-03). A small
 *  local 3-way switch keeps this independent of the agent-status-shaped
 *  StatusDot. "none" never reaches here — the dot is rendered conditionally. */
function checksDot(
  checks: Exclude<PRSummary["checks"], "none">,
): { className: string; tooltip: string } {
  switch (checks) {
    case "pass":
      return { className: "bg-green-500", tooltip: "Checks passing" };
    case "fail":
      return { className: "bg-red-500", tooltip: "Checks failing" };
    case "pending":
      return { className: "bg-amber-400", tooltip: "Checks running" };
  }
}

/** Presentational variant of TaskCard's CardRow/CardShell (D-01). The body is
 *  now the OPEN trigger (Phase 12 D-02): clicking/Enter/Space opens or reattaches
 *  the PR review and routes to its task-like view (UI-SPEC §A). The ↗ external
 *  link stops propagation so it opens GitHub only; the checks dot stays
 *  non-interactive (a click on it falls through to open). It is NOT a dnd item
 *  (cursor-pointer, never cursor-grab — D-13). */
export function PRCard({
  pr,
  projectId,
}: {
  pr: PRSummary;
  projectId: number;
}) {
  const navigate = useNavigate();
  const openReview = useOpenReview(projectId);

  const open = () => {
    // In-flight guard (RESEARCH Pitfall 5): a double-click must not fire two
    // open mutations / two provisions.
    if (openReview.isPending) return;
    openReview.mutate(pr.number, {
      onSuccess: (res) =>
        navigate(`/projects/${projectId}/tasks/${res.task.id}`),
    });
  };

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={`Open review for PR #${pr.number}`}
      aria-busy={openReview.isPending}
      onClick={open}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          open();
        }
      }}
      className={`rounded-md border border-border bg-card px-3 py-2 cursor-pointer hover:bg-[#27272a] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500${
        openReview.isPending ? " opacity-60" : ""
      }`}
    >
      {/* Row 1: title (flex-1 owns extra width) + right-aligned dot/↗ cluster. */}
      <div className="flex gap-2">
        <span className="line-clamp-2 flex-1 text-sm font-medium">
          {pr.title}
        </span>
        {pr.checks !== "none" && (() => {
          // none → render nothing: no dot, no reserved gutter, no layout shift
          // (D-04, the dotless-card rule echoed from TaskCard).
          const { className, tooltip } = checksDot(pr.checks);
          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex shrink-0">
                  <span
                    role="img"
                    aria-label={tooltip}
                    className={`size-2 rounded-full shrink-0 mt-[6px] ${className}`}
                  />
                </span>
              </TooltipTrigger>
              <TooltipContent>{tooltip}</TooltipContent>
            </Tooltip>
          );
        })()}
        <a
          href={pr.url}
          target="_blank"
          rel="noreferrer"
          aria-label={`Open PR #${pr.number} on GitHub`}
          // ↗ opens GitHub only — stop the body's open-review handler (D-02).
          onClick={(e) => e.stopPropagation()}
          className="mt-[2px] shrink-0 text-muted-foreground hover:text-foreground"
        >
          <ExternalLink className="size-3.5" />
        </a>
      </div>
      {/* Row 2: meta line — #number · @author · updated Xh ago (D-02 minimal). */}
      <div className="truncate text-xs text-muted-foreground">
        #{pr.number} · @{pr.author} · updated {formatAgo(pr.updatedAt, Date.now())}{" "}
        ago
      </div>
      {/* In-flight + error feedback (UI-SPEC §F). The endpoint is synchronous;
          this primarily covers the open mutation while it provisions. */}
      {openReview.isPending && (
        <div className="mt-1 text-xs text-muted-foreground">
          Setting up the review worktree…
        </div>
      )}
      {openReview.isError && !openReview.isPending && (
        <div className="mt-1 text-xs text-red-500">
          Couldn't open this review.
        </div>
      )}
    </div>
  );
}

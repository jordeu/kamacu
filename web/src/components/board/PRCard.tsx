import { Check, X, Circle, ExternalLink } from "lucide-react";
import { useNavigate } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatAgo } from "@/lib/time";
import { useAgentStatuses } from "@/api/agents";
import { StatusDot } from "@/components/StatusDot";
import { useOpenReview, type PRSummary } from "@/api/pullRequests";

/** CI status → bare lucide glyph (CHECK-01/02, D-01/D-02/D-03). Replaces the
 *  old colored dot so the colored-dot vocabulary is freed for agent state.
 *  pass → green Check, fail → red X, running → STATIC amber Circle (no spinner).
 *  "none" never reaches here — the icon is rendered conditionally on
 *  `pr.checks !== "none"`, which preserves the no-icon/no-gutter behavior. Note
 *  the CI amber is `text-amber-500` (a half-step deeper than the dot/border
 *  `amber-400`) so the two ambers are not pixel-identical (UI-SPEC §Color). */
function checksIcon(
  checks: Exclude<PRSummary["checks"], "none">,
): { Icon: typeof Check; className: string; tooltip: string } {
  switch (checks) {
    case "pass":
      return { Icon: Check, className: "text-green-500", tooltip: "Checks passing" };
    case "fail":
      return { Icon: X, className: "text-red-500", tooltip: "Checks failing" };
    case "pending":
      return { Icon: Circle, className: "text-amber-500", tooltip: "Checks running" };
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

  // Shared 5s agent-status poll (the same query task cards use). The matched
  // entry is the linked review session (source='github_pr'); a single lookup
  // drives BOTH the StatusDot and the left-border class (D-08/D-15).
  const { data: agents } = useAgentStatuses();
  const entry = agents?.find(
    (e) => e.projectId === projectId && e.prNumber === pr.number,
  );

  // Hoisted out of the meta-line render to clear the carried Date.now()-in-render
  // advisory (D-50, non-blocking lint cleanup).
  const now = Date.now();

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
      className={cn(
        "rounded-md border border-border bg-card px-3 py-2 cursor-pointer hover:bg-[#27272a] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500",
        // Session-border state matrix (D-06/D-07): keyed ONLY on `entry`. This
        // intentionally diverges from TaskCard's waiting-only D-44 rule — any
        // open session gets an always-on blue left edge; waiting shifts it amber.
        // The border itself is STATIC (the pulse lives on the StatusDot).
        entry && entry.status === "waiting" && "border-l-2 border-l-amber-400",
        entry && entry.status !== "waiting" && "border-l-2 border-l-blue-500/60",
        openReview.isPending && "opacity-60",
      )}
    >
      {/* Row 1: title (flex-1 owns extra width) + right-aligned cluster. Locked
          order: [title]·[StatusDot if session]·[CI icon if checks≠none]·[↗]. */}
      <div className="flex gap-2">
        <span className="line-clamp-2 flex-1 text-sm font-medium">
          {pr.title}
        </span>
        {/* Agent dot — the "mine" signal, read first (SIGNL-01). No entry → no
            dot, no gutter (the dotless-card rule, matches TaskCard). */}
        {entry && <StatusDot entry={entry} className="mt-[6px]" />}
        {pr.checks !== "none" && (() => {
          // none → render nothing: no icon, no reserved gutter, no layout shift
          // (CHECK-02, the dotless-card rule echoed from TaskCard).
          const { Icon, className, tooltip } = checksIcon(pr.checks);
          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex shrink-0">
                  <Icon
                    role="img"
                    aria-label={tooltip}
                    className={`size-3.5 shrink-0 mt-[6px] ${className}`}
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
        #{pr.number} · @{pr.author} · updated {formatAgo(pr.updatedAt, now)}{" "}
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

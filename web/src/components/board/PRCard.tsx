import { Check, X, Circle, ExternalLink } from "lucide-react";
import { useNavigate } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatAgo } from "@/lib/time";
import { useAgentStatuses, type AgentStatusEntry } from "@/api/agents";
import { useOpenReview, type PRSummary } from "@/api/pullRequests";

/** CI status → bare lucide glyph (CHECK-01/02, D-01/D-02/D-03). The card's ONLY
 *  mark: pass → green Check, fail → red X, running → STATIC amber Circle (no
 *  spinner). "none" never reaches here — the icon is rendered conditionally on
 *  `pr.checks !== "none"`, preserving the no-icon/no-gutter behavior. The CI
 *  amber is `text-amber-500` (a half-step deeper than the rail's `amber-400`)
 *  so the two ambers are never pixel-identical (UI-SPEC §Color). */
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

/** Agent session state → left-rail color + label (SIGNL-01/02, D-04/D-06/D-07).
 *  Checkpoint redesign (2026-06-17): agent state is shown ONLY by a colored left
 *  rail — the old `StatusDot` was removed because a filled dot sitting beside the
 *  line-art CI glyph read as two clashing marks. Any rail = an open review
 *  session; the rail COLOR is the agent state, mirroring StatusDot's `dotMeta`
 *  palette (working green, waiting amber + pulse, idle blue, exited gray). The
 *  two signals now live on fully separate channels: left edge = your agent,
 *  right glyph = the PR's CI — nothing sits side-by-side, nothing misaligns. */
function agentRail(
  status: AgentStatusEntry["status"],
): { barClass: string; pulse: boolean; label: string } {
  switch (status) {
    case "working":
      return { barClass: "bg-green-500", pulse: false, label: "working" };
    case "waiting":
      return { barClass: "bg-amber-400", pulse: true, label: "waiting for input" };
    case "idle":
      return { barClass: "bg-blue-500/60", pulse: false, label: "idle" };
    case "exited":
      return { barClass: "bg-zinc-600", pulse: false, label: "exited" };
  }
}

/** Presentational variant of TaskCard (D-01/D-13). The body is the OPEN trigger
 *  (Phase 12 D-02): clicking/Enter/Space opens or reattaches the PR review and
 *  routes to its task-like view. The ↗ external link stops propagation so it
 *  opens GitHub only. It is NOT a dnd item (cursor-pointer, never cursor-grab —
 *  D-13). Agent state shows as a left rail; CI as a single right glyph. */
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
  // entry is the linked review session (source='github_pr'); it drives the left
  // rail — the card's sole agent-state signal (D-08/D-15).
  const { data: agents } = useAgentStatuses();
  const entry = agents?.find(
    (e) => e.projectId === projectId && e.prNumber === pr.number,
  );
  const rail = entry ? agentRail(entry.status) : null;

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
      aria-label={
        rail
          ? `Open review for PR #${pr.number} — agent ${rail.label}`
          : `Open review for PR #${pr.number}`
      }
      title={rail ? `Agent ${rail.label}` : undefined}
      aria-busy={openReview.isPending}
      onClick={open}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          open();
        }
      }}
      className={cn(
        "relative overflow-hidden rounded-md border border-border bg-card px-3 py-2 cursor-pointer hover:bg-[#27272a] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500",
        openReview.isPending && "opacity-60",
      )}
    >
      {/* Agent-state left rail (SIGNL-01/02, D-04/D-06/D-07) — the card's ONLY
          agent signal (the dot was removed in the checkpoint redesign). Any rail
          = an open session; its color is the agent state; waiting pulses (the
          attention cue, the old dot's `animate-pulse` moved here). No session →
          no rail, no gutter, identical to a plain card. aria-hidden: state is
          exposed via the card's aria-label/title. */}
      {rail && (
        <span
          aria-hidden
          className={cn(
            "pointer-events-none absolute inset-y-0 left-0 w-[3px]",
            rail.barClass,
            rail.pulse && "animate-pulse motion-reduce:animate-none",
          )}
        />
      )}
      {/* Row 1: title (flex-1 owns extra width) + right-aligned cluster. Locked
          order (redesigned): [title]·[CI icon if checks≠none]·[↗]. */}
      <div className="flex gap-2">
        <span className="line-clamp-2 flex-1 text-sm font-medium">
          {pr.title}
        </span>
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
      {/* In-flight + error feedback (UI-SPEC §F). */}
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

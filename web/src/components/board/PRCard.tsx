import { ExternalLink } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAgo } from "@/lib/time";
import type { PRSummary } from "@/api/pullRequests";

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
 *  INERT (D-08): it carries no hover state, no grab cursor, no body onClick,
 *  and no drag wiring — the only interactive element is the ↗ external-link to
 *  github.com (D-09). Whole-card click is reserved for Phase 12. */
export function PRCard({ pr }: { pr: PRSummary }) {
  return (
    <div className="rounded-md border border-border bg-card px-3 py-2">
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
    </div>
  );
}

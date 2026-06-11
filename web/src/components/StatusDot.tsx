import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { AgentStatusEntry } from "@/api/agents";

/** Pure status → presentation mapping (D-43). Stop-initiated exits (e.g.
 *  code 143) render GRAY, not red — only the color softens; the tooltip
 *  always shows the real exit code (research OQ4). */
export function dotMeta(
  entry: Pick<AgentStatusEntry, "status" | "exitCode" | "stopRequested">,
): { className: string; tooltip: string } {
  switch (entry.status) {
    case "working":
      return { className: "bg-green-500", tooltip: "Working" };
    case "waiting":
      return {
        className: "bg-amber-400 animate-pulse motion-reduce:animate-none",
        tooltip: "Waiting for input",
      };
    case "idle":
      return { className: "bg-zinc-400", tooltip: "Idle" };
    case "exited":
      if (entry.exitCode === null && !entry.stopRequested) {
        // Post-restart DB-derived entry (D-57 / Pitfall 3): a resumable past
        // session the manager no longer knows about. Muted exited gray with a
        // code-free tooltip — never red, never "code null". No new dot states,
        // no pulse, no badge.
        return { className: "bg-zinc-600", tooltip: `Exited` };
      }
      return {
        className:
          entry.exitCode === 0 || entry.stopRequested
            ? "bg-zinc-600"
            : "bg-red-500",
        tooltip: `Exited (code ${entry.exitCode})`,
      };
  }
}

/** 8px status dot wrapped in a tooltip trigger span. The wrapper takes
 *  hover; the dot itself is non-interactive and never intercepts card
 *  click/drag (UI-SPEC) — no stopPropagation anywhere. */
export function StatusDot({
  entry,
  className,
}: {
  entry: Pick<AgentStatusEntry, "status" | "exitCode" | "stopRequested">;
  className?: string;
}) {
  const { className: dotClassName, tooltip } = dotMeta(entry);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("inline-flex shrink-0", className)}>
          <span
            role="img"
            aria-label={`Agent status: ${tooltip.toLowerCase()}`}
            className={cn("size-2 rounded-full shrink-0", dotClassName)}
          />
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

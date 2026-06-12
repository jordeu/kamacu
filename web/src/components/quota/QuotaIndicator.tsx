import { RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from "@/components/ui/hover-card";
import { useQuota, useRefreshQuota, type UsageWindow } from "@/api/usage";

/** Threshold colors (D-69): muted zinc below 60, amber 60–84, red ≥85.
 *  The traffic-light low band never appears — status dots own that color;
 *  quota bars stay neutral until it matters. */
function barColor(pct: number): string {
  if (pct >= 85) return "bg-red-500";
  if (pct >= 60) return "bg-amber-400";
  return "bg-zinc-600";
}

/** Whole minutes since `iso` — "0m", "7m". Computed at render; the 60s
 *  query tick re-renders the component, so no extra timer is needed
 *  (minute granularity makes the poll cadence sufficient). */
function formatAgo(iso: string | null): string {
  if (iso === null) return "0m";
  const mins = Math.max(0, Math.floor((Date.now() - Date.parse(iso)) / 60_000));
  return `${mins}m`;
}

/** Delta until `iso`: <60m → "37m"; <24h → "4h 12m"; else "2d 5h". */
function formatReset(iso: string): string {
  const mins = Math.max(0, Math.floor((Date.parse(iso) - Date.now()) / 60_000));
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ${mins % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

/** Reset column copy: null → em dash (no prefix); past → "Resets now". */
function resetCopy(resetsAt: string | null): string {
  if (resetsAt === null) return "—";
  if (Date.parse(resetsAt) <= Date.now()) return "Resets now";
  return `Resets in ${formatReset(resetsAt)}`;
}

function QuotaRow({ w }: { w: UsageWindow }) {
  const pct = Math.min(100, Math.max(0, w.utilization));
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-12 shrink-0 truncate text-muted-foreground">
        {w.label}
      </span>
      {/* Row bar colored by this window's OWN utilization (D-69 scheme). */}
      <div className="h-1.5 w-24 shrink-0 overflow-hidden rounded-full bg-zinc-800">
        <div
          className={`h-full rounded-full ${barColor(w.utilization)}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="w-9 shrink-0 text-right tabular-nums">
        {Math.round(w.utilization)}%
      </span>
      <span className="min-w-0 flex-1 truncate text-right text-muted-foreground">
        {resetCopy(w.resetsAt)}
      </span>
    </div>
  );
}

/** Compact "Claude 5h" quota trigger with an all-windows hover popup.
 *  Five-state render mapping (D-71/D-72/D-73): loading or no_credentials →
 *  nothing; windows present → bar + popup (stale footer variant); empty
 *  auth_expired/error → motion-free warning chip. */
export function QuotaIndicator() {
  const { data } = useQuota();
  const refresh = useRefreshQuota();

  // Loading (first fetch unresolved) and credential-less users render
  // nothing in the header (D-71) — never a fabricated 0% bar.
  if (data === undefined || data.state === "no_credentials") return null;

  const windows = data.windows ?? [];

  if (windows.length === 0) {
    // Warning-chip states (D-72): credentials exist but no data to show.
    const message =
      data.state === "auth_expired"
        ? "Token expired — re-authenticate with `claude`"
        : "Quota unavailable";
    return (
      <HoverCard openDelay={150}>
        <HoverCardTrigger asChild>
          <div className="flex shrink-0 flex-col items-center gap-1">
            <span className="text-xs leading-none text-muted-foreground">
              Claude 5h
            </span>
            <TriangleAlert
              aria-label="Quota warning"
              className="size-3.5 text-amber-400"
            />
          </div>
        </HoverCardTrigger>
        <HoverCardContent align="end" className="w-72">
          <p className="text-xs text-muted-foreground">{message}</p>
        </HoverCardContent>
      </HoverCard>
    );
  }

  // Bar WIDTH = the 5h window's utilization (D-68); bar COLOR = threshold
  // of the MAX utilization across ALL windows (QUOTA-07/D-70) — a 20%-wide
  // bar must turn red when the 7d window hits 85.
  const fiveHour = windows.find((w) => w.key === "five_hour") ?? windows[0];
  const fiveHourUtil = Math.min(100, Math.max(0, fiveHour.utilization));
  const maxUtil = Math.max(...windows.map((w) => w.utilization));

  return (
    <HoverCard openDelay={150}>
      <HoverCardTrigger asChild>
        <div className="flex shrink-0 flex-col items-center gap-1">
          <span className="text-xs leading-none text-muted-foreground">
            Claude 5h
          </span>
          <div className="h-1.5 w-12 overflow-hidden rounded-full bg-zinc-800">
            <div
              className={`h-full rounded-full ${barColor(maxUtil)}`}
              style={{ width: `${fiveHourUtil}%` }}
            />
          </div>
        </div>
      </HoverCardTrigger>
      <HoverCardContent align="end" className="w-72">
        {/* One row per server-returned window, in server order — the set,
            labels, and order are never hardcoded here (QUOTA-02/D-74/D-75). */}
        <div className="flex flex-col gap-2">
          {windows.map((w) => (
            <QuotaRow key={w.key} w={w} />
          ))}
        </div>
        <div className="mt-2.5 flex items-center justify-between border-t border-foreground/10 pt-1.5">
          {data.stale ? (
            // Stale variant (D-73): amber footer text, trigger untouched.
            <span className="text-xs text-amber-400">
              error · {formatAgo(data.fetchedAt)} old
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">
              Updated {formatAgo(data.fetchedAt)} ago
            </span>
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Refresh quota"
            onClick={() => refresh.mutate()}
            disabled={refresh.isPending}
          >
            <RefreshCw className="size-3.5" />
          </Button>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}

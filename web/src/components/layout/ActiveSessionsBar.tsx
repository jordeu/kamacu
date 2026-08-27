import { useEffect, useRef, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import { ChevronUp, ChevronDown } from "lucide-react";
import { useAgentStatuses, type AgentStatusEntry } from "@/api/agents";
import { dotMeta } from "@/components/StatusDot";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

/**
 * The Global Active Sessions Bar (SBAR-01..09) — a persistent, collapsible bar
 * pinned to the bottom of the app shell (mounted once in AppLayout, outside the
 * <Outlet/>), present on every route. It is the 4th consumer of the existing
 * useAgentStatuses() 5s poll (D-12) — no new polling hook.
 *
 * Collapsed (default, SBAR-02/03): per-state colored counts (working / waiting /
 * idle) + a total, with the waiting count amber-emphasized and pulsing ONLY when
 * > 0. Expanded (SBAR-04/05): a flat list across all projects, attention-first
 * (waiting -> working -> idle), each row a clickable [state dot · project · task/
 * PR title] navigating cross-project to that task's agent view (SBAR-06).
 *
 * The expanded panel floats UP over content as an overlay (the whole component is
 * `fixed inset-x-0 bottom-0`), so main content and the height-sensitive xterm
 * terminals NEVER reflow or resize (D-03). Collapse state persists in
 * localStorage under a single GLOBAL key, default collapsed (SBAR-07 / D-04).
 * Only LIVE sessions appear; `exited` (incl. DB-derived post-restart) entries are
 * filtered out client-side (D-09).
 */
export function ActiveSessionsBar() {
  // --- Collapse persistence — mirror ReviewColumn exactly, GLOBAL key
  // (SBAR-07 / D-04). Absent key => collapsed (the `!== "0"` idiom); explicit
  // expand persists as "0". ---
  const storageKey = "kamacu:sessions-bar-collapsed";
  const [collapsed, setCollapsed] = useState<boolean>(
    () => localStorage.getItem(storageKey) !== "0",
  );
  const toggle = () =>
    setCollapsed((c) => {
      const next = !c;
      localStorage.setItem(storageKey, next ? "1" : "0");
      return next;
    });
  // --- Unconditional collapse used when a row is opened (SBAR-06): the expanded
  // list has served its purpose, so dismiss it and persist collapsed ("1") so the
  // state survives a reload. Distinct from `toggle`, which flips relative state. ---
  const collapse = () => {
    setCollapsed(true);
    localStorage.setItem(storageKey, "1");
  };

  // --- Outside-click auto-collapse (D-11): when expanded, a pointer press
  // anywhere outside the bar dismisses it via collapse() (persists "1"). Mirrors
  // the TaskPage document-listener idiom (add-on-mount / remove-on-cleanup). Uses
  // the `mousedown` bubble phase so nested surfaces inside the bar count as
  // "inside" via the ref-contains check — the collapsed bar's own onClick={toggle}
  // therefore keeps toggling without this listener double-firing a re-expand. The
  // effect is a no-op while already collapsed (guarded on `collapsed`), and it
  // deliberately does NOT collapse on Escape — the gesture is click-only this pass. ---
  const barRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (collapsed) return;
    function onMouseDown(e: MouseEvent) {
      if (barRef.current && !barRef.current.contains(e.target as Node)) {
        collapse();
      }
    }
    document.addEventListener("mousedown", onMouseDown);
    return () => document.removeEventListener("mousedown", onMouseDown);
  }, [collapsed]);

  // --- Data — the single existing 5s poll (D-12); no new hook. ---
  const { data } = useAgentStatuses();
  const navigate = useNavigate();
  const { taskId: openTaskId } = useParams(); // current-task highlight (D-07)
  // Global-route highlight (Pitfall 6): no taskId param exists on /global, so
  // a global row's currency is derived from the pathname instead.
  const location = useLocation();

  // --- LIVE filter (D-09): working / waiting / idle (claude) AND running
  // (custom-engine, M001). `exited` (incl. the DB-derived/post-restart
  // resumable rows) never appears. ---
  const live = (data ?? []).filter(
    (e) =>
      e.status === "working" ||
      e.status === "waiting" ||
      e.status === "idle" ||
      e.status === "running",
  );

  const working = live.filter((e) => e.status === "working").length;
  const waiting = live.filter((e) => e.status === "waiting").length;
  const idle = live.filter((e) => e.status === "idle").length;
  const running = live.filter((e) => e.status === "running").length;
  const total = live.length;

  // --- Sorted list (SBAR-05 / D-05), attention-first, STABLE within a state
  // (Array.prototype.sort is stable in modern engines, so equal-rank rows keep
  // feed order — avoids 5s jitter). ---
  const rank = { waiting: 0, working: 1, running: 2, idle: 3 } as const;
  const sorted = [...live].sort(
    (a, b) =>
      rank[a.status as keyof typeof rank] - rank[b.status as keyof typeof rank],
  );

  const loading = data === undefined;

  // --- Collapsed-bar aria-label describing current counts (mirror ReviewColumn /
  // ProjectSidebar pluralized count aria). ---
  const barAriaLabel = `Active agent sessions: ${working} working, ${waiting} waiting, ${idle} idle, ${running} running — ${
    collapsed ? "expand" : "collapse"
  }`;

  return (
    <div ref={barRef} className="fixed inset-x-0 bottom-0 z-30 flex flex-col">
      {/* Expanded panel — rendered ABOVE the collapsed bar so it floats UP over
          content (overlay; never reflows the xterm terminals, D-03). */}
      {!collapsed && (
        <div className="max-h-80 overflow-y-auto border-t border-border bg-[#101013] p-3 transition-[max-height,opacity] duration-150 ease-out motion-reduce:transition-none">
          {total === 0 ? (
            <div className="flex flex-col gap-1 py-6 text-center">
              <div className="text-sm text-muted-foreground">
                No active sessions
              </div>
              <div className="text-xs text-muted-foreground">
                Agent sessions appear here while they're running.
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-0.5">
              {sorted.map((entry) => (
                <SessionRow
                  // Keyed by sessionId (P6): a global entry's taskId is 0, so
                  // taskId keying would collide across rows. Empty sessionIds
                  // occur only on DB-derived exited rows, which the LIVE
                  // filter above removes before render — live rows always
                  // carry unique sessionIds (16-RESEARCH Pitfall 5).
                  key={entry.sessionId}
                  entry={entry}
                  isCurrent={
                    entry.source === "global"
                      ? location.pathname === "/global"
                      : String(entry.taskId) === openTaskId
                  }
                  onOpen={() => {
                    if (entry.source === "global") {
                      // GINT-01: the global row targets the Scratchpad view
                      // (route lands in 16-02) — replaces the Phase-15
                      // interim dead-route artifact.
                      navigate("/global");
                    } else {
                      navigate(
                        `/projects/${entry.projectId}/tasks/${entry.taskId}`,
                      );
                    }
                    collapse();
                  }}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Collapsed bar — always rendered; the WHOLE bar is the toggle (D-04). */}
      <div
        role="button"
        aria-expanded={!collapsed}
        aria-label={barAriaLabel}
        onClick={toggle}
        className="flex h-9 cursor-pointer items-center gap-2 border-t border-border bg-[#101013] px-3"
      >
        {loading ? (
          <Skeleton className="h-3 w-24" />
        ) : total === 0 ? (
          <span className="text-xs text-muted-foreground">
            No active sessions
          </span>
        ) : (
          <>
            {/* Working count */}
            <CountGroup status="working" count={working} />
            {/* Waiting count — amber-emphasized + pulsing ONLY when > 0 */}
            <CountGroup status="waiting" count={waiting} />
            {/* Idle count */}
            <CountGroup status="idle" count={idle} />
            {/* Running count (M001 custom-engine agents; no finer state) */}
            <CountGroup status="running" count={running} />
          </>
        )}

        {/* Chevron — up = will expand (collapsed), down = will collapse. */}
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={collapsed ? "Expand active sessions" : "Collapse active sessions"}
          onClick={(e) => {
            e.stopPropagation();
            toggle();
          }}
          className="ml-auto text-muted-foreground"
        >
          {collapsed ? (
            <ChevronUp className="size-3.5 text-muted-foreground" />
          ) : (
            <ChevronDown className="size-3.5 text-muted-foreground" />
          )}
        </Button>
      </div>
    </div>
  );
}

/**
 * A single collapsed-bar count group: a state dot + the numeric count. The
 * waiting group gets the amber chip treatment (mirroring ProjectSidebar) ONLY
 * when count > 0; otherwise it renders with the same muted treatment as the
 * working/idle groups. All dot colors come from dotMeta() — never hardcoded.
 */
function CountGroup({
  status,
  count,
}: {
  status: "working" | "waiting" | "idle" | "running";
  count: number;
}) {
  const { className: dotClassName, tooltip } = dotMeta({
    status,
    exitCode: null,
    stopRequested: false,
  });
  const waitingEmphasis = status === "waiting" && count > 0;
  return (
    <span
      aria-label={`${count} ${tooltip.toLowerCase()}`}
      className={cn(
        "inline-flex items-center gap-1 text-xs font-medium tabular-nums",
        waitingEmphasis ? "text-amber-400" : "text-muted-foreground",
      )}
    >
      <span className={cn("size-2 rounded-full", dotClassName)} />
      {count}
    </span>
  );
}

/**
 * A single expanded-list row: [state dot · project name · task/PR title]. The
 * whole row is the click target (SBAR-06). PR-review rows (source==='github_pr')
 * append a muted #<n> identifier badge (D-06). The current open task's row is
 * subtly highlighted, non-chromatic (D-07).
 */
function SessionRow({
  entry,
  isCurrent,
  onOpen,
}: {
  entry: AgentStatusEntry;
  isCurrent: boolean;
  onOpen: () => void;
}) {
  const { className: dotClassName } = dotMeta(entry);
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={`Open ${entry.taskTitle} in ${entry.projectName}`}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        "flex min-h-9 items-center gap-2 rounded-md px-2 py-1.5 text-sm cursor-pointer hover:bg-muted/50",
        isCurrent && "bg-sidebar-accent text-sidebar-accent-foreground",
      )}
    >
      <span className={cn("size-2 rounded-full shrink-0", dotClassName)} />
      <span className="shrink-0 text-muted-foreground">
        {entry.projectName}
      </span>
      <span className="text-muted-foreground">·</span>
      <span className="min-w-0 flex-1 truncate" title={entry.taskTitle}>
        {entry.taskTitle}
      </span>
      {entry.source === "github_pr" && entry.prNumber != null && (
        <span className="shrink-0 rounded-full bg-card px-1.5 text-xs font-medium text-muted-foreground tabular-nums">
          #{entry.prNumber}
        </span>
      )}
    </div>
  );
}

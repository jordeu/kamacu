import type { StatsBlock, TimeStat } from "@/api/types";
import { formatDuration } from "@/lib/time";

/**
 * Phase 11 — the statistics strip (CONTEXT D-08/D-09/D-11). A compact strip
 * across the top of the page: counts as lead numbers, then three time-stat
 * rows (cycle / dwellInProgress / dwellInReview) rendered as
 * `min · median · max` with an "over N tasks" caption.
 *
 * D-11 em-dash guard: when `stat.n === 0` the row shows `—` (in muted text),
 * NOT `0m`/`0s` — never imply a measured zero. The guard runs BEFORE any
 * formatDuration call (Phase 11 RESEARCH Pitfall 4). Counts still render
 * literal `0`.
 *
 * No cards grid, no charts (D-09 / Out of Scope ACTFUT-02). Reviews
 * contribute `reviewCount` ONLY (STATS-04 — never a time-stat row).
 */
function TimeStatRow({ label, stat }: { label: string; stat: TimeStat }) {
  return (
    <div className="flex items-baseline gap-2 text-sm">
      <span className="min-w-[5.5rem] text-muted-foreground">{label}</span>
      {stat.n === 0 ? (
        <span className="font-mono text-muted-foreground">—</span>
      ) : (
        <span className="font-mono">
          {formatDuration(stat.min)} · {formatDuration(stat.median)} ·{" "}
          {formatDuration(stat.max)}
        </span>
      )}
      <span className="text-xs text-muted-foreground">
        {`over ${stat.n} tasks`}
      </span>
    </div>
  );
}

export function StatsStrip({ stats }: { stats: StatsBlock }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Statistics`}</h2>
      <div className="flex items-baseline gap-6">
        <div className="flex items-baseline gap-2">
          <span className="text-2xl font-semibold tabular-nums">
            {stats.taskCount}
          </span>
          <span className="text-xs text-muted-foreground">{`tasks`}</span>
        </div>
        <div className="flex items-baseline gap-2">
          <span className="text-2xl font-semibold tabular-nums">
            {stats.reviewCount}
          </span>
          <span className="text-xs text-muted-foreground">{`reviews`}</span>
        </div>
      </div>
      <div className="flex flex-col gap-1">
        <TimeStatRow label={`Cycle`} stat={stats.cycle} />
        <TimeStatRow label={`In progress`} stat={stats.dwellInProgress} />
        <TimeStatRow label={`In review`} stat={stats.dwellInReview} />
      </div>
    </section>
  );
}

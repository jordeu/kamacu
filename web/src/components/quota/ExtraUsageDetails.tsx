import { ChevronRight } from "lucide-react";
import type { ExtraUsage, UsageWindow } from "@/api/usage";
import { formatCreditAmount } from "@/lib/creditUsage";

export function ExtraUsageDetails({ extra, windows, stale, now }: {
  extra: ExtraUsage | null;
  windows: UsageWindow[];
  stale: boolean;
  now: number;
}) {
  if (!extra) return <p className="mt-2 text-xs text-muted-foreground">Extra-credit usage unavailable</p>;
  const exhausted = windows.filter((w) => w.utilization >= 100);
  const today = extra.daily?.find((day) => day.date === new Date(now).toISOString().slice(0, 10));
  return (
    <div className="mt-2.5 border-t border-foreground/10 pt-2 text-xs">
      {exhausted.length > 0 && (
        <div className="mb-3 rounded-md border border-amber-400/20 bg-amber-400/5 px-3 py-3">
          <div className="flex items-center gap-1.5 text-[10px] font-medium text-amber-400">
            <span className={`size-1.5 rounded-full ${stale ? "border border-amber-400" : "bg-amber-400"}`} />
            {stale ? "Last known extra usage" : "Quota exhausted"}
          </div>
          <div className="mt-2 text-3xl font-medium tracking-tight tabular-nums">
            {today ? formatCreditAmount(today.spent, extra.currency) : "—"}
          </div>
          <p className="mt-0.5">Extra spending today</p>
          <p className="mt-1 text-[10px] text-muted-foreground">
            {exhausted.map((w) => w.label).join(" + ")} limit · today (UTC)
          </p>
          {today?.partial && (
            <p className="mt-2 text-[10px] leading-relaxed text-muted-foreground">Partial day: only observed spending is included.</p>
          )}
          {!today && (
            <p className="mt-2 text-[10px] leading-relaxed text-muted-foreground">Today's extra spending is unavailable.</p>
          )}
        </div>
      )}
      <div className="flex justify-between gap-2">
        <span className="text-muted-foreground">Extra credits · this month</span>
        <span className="tabular-nums">{extra.usedCredits === null ? "Unavailable" : formatCreditAmount(extra.usedCredits, extra.currency)}</span>
      </div>
      <p className="mt-1 text-muted-foreground">
        {extra.enabled ? "Enabled" : "Disabled"}
        {extra.monthlyLimit !== null && ` · ${formatCreditAmount(extra.monthlyLimit, extra.currency)} monthly limit`}
      </p>
      {extra.historyAvailable && extra.daily?.length ? (
        <details className="group mt-3 border-t border-foreground/10 pt-2">
          <summary className="flex cursor-pointer list-none items-center justify-between rounded-sm py-1 text-[11px] outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
            <span>Daily extra spending</span>
            <ChevronRight className="size-3 text-muted-foreground transition-transform group-open:rotate-90" aria-hidden="true" />
          </summary>
          <div className="mt-2 max-h-48 overflow-y-auto">
            <table className="w-full text-left tabular-nums">
              <thead className="text-muted-foreground"><tr><th className="font-normal">Date (UTC)</th><th className="text-right font-normal">Observed</th></tr></thead>
              <tbody>{extra.daily.map((day) => (
                <tr key={day.date}>
                  <td className="py-1">{day.date}{day.partial && <span title="Partial day or gap in tracking"> *</span>}</td>
                  <td className="py-1 text-right">{formatCreditAmount(day.spent, extra.currency)}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
          <p className="mt-1 text-muted-foreground">Changes between samples, assigned to the day observed. * Partial day or tracking gap. Updates while Kamacu is running.</p>
        </details>
      ) : <p className="mt-2 text-muted-foreground">Daily tracking unavailable{extra.usedCredits === null ? " until Claude reports spending." : "."}</p>}
    </div>
  );
}

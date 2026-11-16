import type { ExtraUsage, UsageWindow } from "@/api/usage";

export function formatCreditAmount(cents: number, currency: string): string {
  return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(cents / 100);
}

export function activeCreditPeriod(extra: ExtraUsage | null, windows: UsageWindow[]) {
  return extra?.enabled && windows.some((w) => w.utilization >= 100)
    ? extra.creditPeriod
    : null;
}

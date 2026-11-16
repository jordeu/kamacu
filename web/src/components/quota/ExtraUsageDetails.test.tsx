import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { ExtraUsage, UsageWindow } from "@/api/usage";
import { ExtraUsageDetails } from "./ExtraUsageDetails";

const windows: UsageWindow[] = [{ key: "five_hour", label: "5h", utilization: 100, resetsAt: null }];
const extra: ExtraUsage = {
  enabled: true, usedCredits: 1847, monthlyLimit: 5000, utilization: 36.94,
  currency: "USD", historyAvailable: true,
  creditPeriod: { spentCredits: 425, startedAt: "2026-11-16T10:42:00Z", baselineKnown: true },
  daily: [{ date: "2026-11-16", spent: 425, partial: true }],
};

function render(value = extra, quota = windows, stale = false) {
  return renderToStaticMarkup(<ExtraUsageDetails extra={value} windows={quota} stale={stale} />);
}

describe("extra-credit details", () => {
  it("shows period spending separately from the monthly total and starts history collapsed", () => {
    const html = render();
    expect(html).toContain("$4.25");
    expect(html).toContain("$18.47");
    expect(html).toContain("$50.00 monthly limit");
    expect(html).toContain("Since quota reached");
    expect(html).toMatch(/<summary[^>]*>.*Daily extra spending/);
    expect(html).not.toMatch(/<details[^>]*\bopen\b/);
  });
  it("identifies partial tracking and stale values", () => {
    const html = render({ ...extra, creditPeriod: { ...extra.creditPeriod!, baselineKnown: false } }, windows, true);
    expect(html).toContain("Since tracking began");
    expect(html).toContain("Earlier spending or gaps in tracking are unknown");
    expect(html).toContain("Last known extra usage");
  });
  it("hides the period when quota replenishes or credits are disabled", () => {
    expect(render(extra, [{ ...windows[0], utilization: 30 }])).not.toContain("Since quota reached");
    expect(render({ ...extra, enabled: false })).not.toContain("Using extra credits");
  });
  it("keeps missing period spending unknown", () => {
    const html = render({ ...extra, creditPeriod: null });
    expect(html).toContain("Period spending unavailable");
    expect(html).not.toContain("$0.00");
  });
});

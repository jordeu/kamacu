import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { ExtraUsage, UsageWindow } from "@/api/usage";
import { ExtraUsageDetails } from "./ExtraUsageDetails";

const now = Date.parse("2026-11-18T08:28:39Z");
const windows: UsageWindow[] = [{ key: "five_hour", label: "5h", utilization: 100, resetsAt: null }];
const extra: ExtraUsage = {
  enabled: true, usedCredits: 65836, monthlyLimit: 100000, utilization: 65.836,
  currency: "EUR", historyAvailable: true,
  daily: [{ date: "2026-11-18", spent: 453, partial: false }],
};

function render(value = extra, quota = windows, stale = false, time = now) {
  return renderToStaticMarkup(<ExtraUsageDetails extra={value} windows={quota} stale={stale} now={time} />);
}

describe("daily extra-credit details", () => {
  it("highlights today's €4.53 when quota is exhausted and keeps history collapsed", () => {
    const html = render();
    expect(html).toContain("€4.53");
    expect(html).toContain("Extra spending today");
    expect(html).toContain("Quota exhausted");
    expect(html).toContain("€658.36");
    expect(html).toContain("monthly limit");
    expect(html).toMatch(/<summary[^>]*>.*Daily extra spending/);
    expect(html).not.toMatch(/<details[^>]*\bopen\b/);
  });
  it("hides the daily highlight when quota is available", () => {
    const html = render(extra, [{ ...windows[0], utilization: 99 }]);
    expect(html).not.toContain("Extra spending today");
    expect(html).toContain("Extra credits · this month");
  });
  it("highlights daily spending when a weekly limit is exhausted", () => {
    const html = render(extra, [
      { ...windows[0], utilization: 30 },
      { key: "seven_day", label: "7d", utilization: 100, resetsAt: null },
    ]);
    expect(html).toContain("Extra spending today");
    expect(html).toContain("7d limit · today (UTC)");
  });
  it("shows recorded spending even if extra credits have since been disabled", () => {
    const html = render({ ...extra, enabled: false });
    expect(html).toContain("Extra spending today");
    expect(html).toContain("Disabled");
    expect(html).not.toContain("Using extra credits");
  });
  it("identifies partial daily totals and stale samples", () => {
    const html = render({ ...extra, daily: [{ ...extra.daily![0], partial: true }] }, windows, true);
    expect(html).toContain("€4.53");
    expect(html).toContain("Partial day");
    expect(html).toContain("Last known extra usage");
  });
  it("shows a recorded zero as zero", () => {
    const html = render({ ...extra, daily: [{ ...extra.daily![0], spent: 0 }] });
    expect(html).toContain("€0.00");
    expect(html).not.toContain("extra spending is unavailable");
  });
  it("keeps missing daily spending unknown", () => {
    const html = render({ ...extra, daily: [] });
    expect(html).toContain(">—</div>");
    expect(html).toContain("extra spending is unavailable");
    expect(html).not.toContain("€0.00");
  });
  it("does not reuse yesterday's spending after UTC midnight", () => {
    const html = render(extra, windows, false, Date.parse("2026-11-19T00:00:00Z"));
    expect(html).toContain(">—</div>");
    expect(html).toContain("extra spending is unavailable");
    expect(html).not.toContain("€0.00");
  });
});

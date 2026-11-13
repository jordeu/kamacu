import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { PRStatusDetails, PRStatusOverview } from "./PRStatusOverview";
import type { PRStatusSnapshot } from "@/api/prStatus";

function data(bot = "comments"): PRStatusSnapshot {
  return { repository: "acme/app", number: 12, lifecycle: "open", head: "a84c190",
    humans: { state: "approved", reviewers: [{ login: "maria", state: "approved" }] },
    bots: { state: bot, reviewers: [{ login: "reviewbot", state: bot, detail: "Current head" }] },
    ci: { state: "pass", jobs: [{ name: "Unit tests", state: "pass" }] },
    fetched_at: "2026-11-13T10:00:00Z", stale: false };
}
it("announces the green primary result and secondary bot feedback", () => {
  const html = renderToStaticMarkup(<PRStatusOverview repository="acme/app" number={12} snapshot={data()} />);
  expect(html).toContain("Human approved · CI passing. Bots: Review comments.");
  expect(html).toContain('aria-haspopup="dialog"');
  expect(html).toContain('aria-expanded="false"');
  expect(html).toContain("lucide-message-square");
});
it("removes the badge for completed no-findings outcomes", () => {
  const html = renderToStaticMarkup(<PRStatusOverview repository="acme/app" number={12} snapshot={data("clean")} />);
  expect(html).toContain("Bots: Completed · no new findings");
  expect(html).not.toContain("lucide-message-square");
});
it("keeps CI, humans and comment-only bot outcomes separate in the overview", () => {
  const html = renderToStaticMarkup(<PRStatusDetails repository="acme/app" number={12} snapshot={data("clean")} />);
  expect(html).toContain('aria-label="CI checks"');
  expect(html).toContain('aria-label="Human reviews"');
  expect(html).toContain('aria-label="Bot feedback"');
  expect(html).toContain("@maria");
  expect(html).toContain("@reviewbot");
  expect(html).toContain("Completed · no new findings");
  expect(html).toContain("not a human approval");
});

it("shows an empty-body completed review without calling it an older commit or no-new-findings report", () => {
  const html = renderToStaticMarkup(<PRStatusDetails repository="acme/app" number={12} snapshot={data("complete")} />);
  expect(html).toContain("Completed");
  expect(html).not.toContain("Older commit");
  expect(html).not.toContain("Completed · no new findings");
});

it.each([
  ["pass", "All checks passing", "text-green-400"],
  ["fail", "Checks failing", "text-red-400"],
  ["pending", "Checks pending", "text-amber-400"],
  ["none", "No checks", "text-zinc-400"],
  ["unknown", "CI unavailable", "text-zinc-400"],
])("summarizes %s CI in one line without listing jobs", (state, label, color) => {
  const snapshot = data();
  snapshot.ci = { state, jobs: Array.from({ length: 100 }, (_, index) => ({ name: `Job ${index}`, state })) };
  const html = renderToStaticMarkup(<PRStatusDetails repository="acme/app" number={12} snapshot={snapshot} />);
  const ci = html.match(/<section aria-label="CI checks"[^>]*>(.*?)<\/section>/)?.[1];
  expect(ci).toContain(label);
  expect(ci).toContain(color);
  expect(ci).not.toContain("Job ");
  expect(ci).not.toContain("<ul");
});

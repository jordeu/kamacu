import type {
  ActivityResponse,
  ActivityReviewState,
  ReviewDoneSummary,
} from "@/api/types";
import { formatAgo } from "@/lib/time";
import { groupByProject } from "./ActivityList";

/**
 * Phase 11 — the reviews-done list (CONTEXT D-12, D-13, D-14; ACT-04;
 * REVIEWS-02/03).
 *
 * D-12 / ACT-04 "quietly empty": when `reviews.state` is in the HARD_DEGRADE
 * set AND `prs` is empty, the ENTIRE section is suppressed (no heading, no
 * hint, no body) — return `null`. This is the literal reading of ACT-04's
 * "quietly degrades to empty." Contrast ReviewColumn.tsx which renders an
 * amber hint in the same states — Activity does NOT.
 *
 * `auth_required` is in the degrade set even though CONTEXT D-12 omits it —
 * the backend emits it (rollupReviewState) and the page must treat it as a
 * hard degrade (Phase 11 RESEARCH Pitfall 2).
 *
 * For ok / partial / error-with-cached-prs: the prs are sorted by
 * `completedAt DESC` client-side BEFORE grouping (Pitfall 5 — the server
 * concatenates per-repo lists unsorted; tasks by contrast arrive pre-sorted).
 * `completedAt` is uniform second-precision ISO, so a lexical descending sort
 * is chronological.
 *
 * D-14 (deliberate simplification, flag at UAT): clicking an entry opens the
 * GitHub PR URL universally via a plain `<a target="_blank" rel="noreferrer">`
 * — the `else the PR` fallback, NOT an in-app review-workspace lookup. NEVER
 * `window.open` (popup-blocked — Pitfall 7). NEVER a router `<Link>` (external).
 */
const HARD_DEGRADE: ReadonlySet<ActivityReviewState> = new Set([
  "disabled",
  "no_gh",
  "auth_required",
  "error",
]);

/**
 * Sort comparator: completedAt DESC. `completedAt` is uniform second-precision
 * ISO (gh closedAt) so lexical descending sort is chronological (Pitfall 5).
 */
function byCompletedDesc(a: ReviewDoneSummary, b: ReviewDoneSummary): number {
  return b.completedAt.localeCompare(a.completedAt);
}

export function ReviewsList({ reviews }: { reviews: ActivityResponse["reviews"] }) {
  // Null-safe: a buggy/older server can ship `prs: null` (Go nil slice -> JSON
  // null). Normalize once so the degrade check and the spread never throw.
  const prs = reviews.prs ?? [];

  // D-12: quietly empty on hard-degrade with no cached PRs — return null (NO
  // amber hint, NO heading, NO body). Never toast (Phase 26 convention).
  if (HARD_DEGRADE.has(reviews.state) && prs.length === 0) {
    return null;
  }

  // ok / partial / error-with-cached-prs: sort the cross-repo merge by
  // completedAt DESC before grouping (Pitfall 5). Never whitelist {ok} alone —
  // partial carries real PRs and must render.
  const sorted = [...prs].sort(byCompletedDesc);
  const groups = groupByProject(sorted);

  // Quiet empty even on ok+empty (Phase 11 RESEARCH Open Q1 — suppress the
  // whole section rather than render a heading with no entries).
  if (groups.length === 0) {
    return null;
  }

  const now = Date.now();

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Reviews completed`}</h2>
      <div className="flex flex-col gap-3">
        {groups.map((group) => (
          <div key={group.projectId} className="flex flex-col gap-1">
            <div className="text-xs font-medium text-muted-foreground">
              {group.projectName}
            </div>
            {group.entries.map((review) => (
              <a
                key={review.number}
                href={review.url}
                target="_blank"
                rel="noreferrer"
                aria-label={`Open PR #${review.number} on GitHub`}
                className="flex items-baseline gap-2 text-sm hover:text-foreground"
              >
                <span className="flex-1 truncate text-muted-foreground hover:text-foreground">
                  <span className="font-mono">#{review.number}</span>{" "}
                  {review.title}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {formatAgo(review.completedAt, now)}
                </span>
              </a>
            ))}
          </div>
        ))}
      </div>
    </section>
  );
}

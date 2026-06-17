import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { formatAgo } from "@/lib/time";
import { useProjects } from "@/api/queries";
import { useSettings } from "@/api/settings";
import {
  usePullRequests,
  useRefreshPullRequests,
} from "@/api/pullRequests";
import { PRCard } from "./PRCard";

/**
 * The Review column — a self-gating, collapsible sibling of the status columns,
 * rendered to the RIGHT of Done in Board.tsx's flex row. It is NEVER a dnd
 * drop target and carries no dnd-kit wiring whatsoever — PR cards never enter
 * the kanban (D-13, GHREV-04 forward-looking).
 *
 * The component SELF-GATES (RESEARCH Pattern 4, recommendation b): it renders
 * null unless GitHub integration is on (github_integration === 'on') AND the
 * project is linked (github_repo set). When gated off the board flex row is
 * byte-for-byte pre-v1.3 (GHSET-02), so Board.tsx passes only projectId and its
 * props stay untouched. Because the hooks are called AFTER the gate, a gated-off
 * project never polls gh — only the visible/linked project mounts this (D-11).
 */
export function ReviewColumn({ projectId }: { projectId: number }) {
  // --- Render gate (GHCOL-01 + GHSET-02 OFF cascade). MUST run before the
  // data hooks so a gated-off project does not poll. The `disabled` server
  // state therefore never surfaces as copy (UI-SPEC: it is the gate result). ---
  const { data: settings } = useSettings();
  const { data: projects } = useProjects();
  const project = projects?.find((p) => p.id === projectId);
  const enabled = settings?.github_integration?.value === "on";
  const linked = project?.github_repo != null && project.github_repo !== "";
  if (!enabled || !linked) return null;

  return <ReviewColumnInner projectId={projectId} />;
}

/**
 * The gated-in body. Split out so the data hooks (which violate the
 * rules-of-hooks if placed after an early `return null`) live in a component
 * that is only mounted once the gate is open.
 */
function ReviewColumnInner({ projectId }: { projectId: number }) {
  // --- Collapse state — localStorage, per-project, default collapsed
  // (supersedes D-05 per user request 2026-06-14; D-06 / GHCOL-06). The column
  // is collapsed UNLESS the user has explicitly expanded it (stored "0"); an
  // absent key => collapsed. ---
  const storageKey = `kangent:review-collapsed:${projectId}`;
  const [collapsed, setCollapsed] = useState<boolean>(
    () => localStorage.getItem(storageKey) !== "0",
  );
  const toggle = () =>
    setCollapsed((c) => {
      const next = !c;
      localStorage.setItem(storageKey, next ? "1" : "0");
      return next;
    });

  // --- Data — poll (60s, visibility-paused) + manual refresh (GHCOL-04). ---
  const { data } = usePullRequests(projectId);
  const refresh = useRefreshPullRequests(projectId);
  const prs = data?.prs ?? [];
  // Recently-reviewed list (REVWD-01) — already deduped against `prs` and sorted
  // most-recently-updated-first server-side (16-01); rendered verbatim.
  const reviewed = data?.reviewed ?? [];
  // count only when the server reports a clean read; null while loading/degraded
  // so the header/badge never shows a fabricated 0 (D-07). The header count stays
  // the AWAITING count — reviewed never rolls into it (UI-SPEC §Recently Reviewed).
  const count = data?.state === "ok" ? prs.length : null;

  // --- Collapsed rail (GHCOL-06) — a slim w-10 strip, clickable to expand. ---
  if (collapsed) {
    return (
      <div
        role="button"
        aria-label={`Expand review column (${count ?? 0} PRs)`}
        onClick={toggle}
        className="flex w-10 shrink-0 flex-col items-center gap-2 rounded-md bg-[#101013] p-2"
      >
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Expand review column"
          onClick={(e) => {
            e.stopPropagation();
            toggle();
          }}
          className="text-muted-foreground"
        >
          <ChevronRight className="size-3.5" />
        </Button>
        <span className="text-xs uppercase tracking-wide text-muted-foreground [writing-mode:vertical-rl] rotate-180">
          Review
        </span>
        {count !== null && (
          <span className="rounded-full bg-card px-1.5 text-xs font-medium text-muted-foreground">
            {count}
          </span>
        )}
      </div>
    );
  }

  // --- Expanded column (GHCOL-01) — echoes Column.tsx exactly, minus the
  // droppable ring (this is NOT a droppable). ---
  return (
    <div className="flex min-h-0 min-w-[260px] flex-1 flex-col">
      <div className="flex items-baseline gap-2 px-3 pb-2">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-expanded={!collapsed}
          aria-label="Collapse review column"
          onClick={toggle}
          className="text-muted-foreground"
        >
          <ChevronDown className="size-3.5" />
        </Button>
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Review
        </span>
        <span className="text-xs font-medium text-muted-foreground">
          {count ?? ""}
        </span>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Refresh pull requests"
          onClick={() => refresh.mutate()}
          disabled={refresh.isPending}
          className="ml-auto text-muted-foreground"
        >
          <RefreshCw className="size-3.5" />
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto rounded-md bg-[#101013] p-3">
        <ReviewStates
          data={data}
          prs={prs}
          reviewed={reviewed}
          projectId={projectId}
        />
      </div>
    </div>
  );
}

/**
 * Inline state branches (GHCOL-05) — all rendered INSIDE the card-list
 * container, never a modal, never blocking the board. Branches on data/state/
 * stale per UI-SPEC §States and §Copywriting Contract.
 */
function ReviewStates({
  data,
  prs,
  reviewed,
  projectId,
}: {
  data: ReturnType<typeof usePullRequests>["data"];
  prs: import("@/api/pullRequests").PRSummary[];
  reviewed: import("@/api/pullRequests").PRSummary[];
  projectId: number;
}) {
  // Loading — first fetch unresolved. Quiet skeletons, no spinner, no overlay.
  // (reviewed is [] here, so reviewedSection below yields null — no shimmer.)
  if (data === undefined) {
    return (
      <>
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-14 w-full" />
      </>
    );
  }

  // Hoisted to clear the carried Date.now()-in-render advisory (D-50, non-blocking).
  const now = Date.now();

  const stale = data.stale === true;
  const staleFooter = stale ? (
    <div className="text-xs text-amber-400">
      error · {formatAgo(data.fetchedAt, now)} old
    </div>
  ) : null;

  // "Recently reviewed" subsection (REVWD-01/03/04) — a hairline-divided label
  // row + the reviewed PRCards, rendered below the awaiting list inside the same
  // scroll area. Quietly omitted when empty (D-11): no divider, no label, no
  // "(0)". Reviewed cards reuse PRCard verbatim, so they get the dot/border/CI
  // icon for free (SIGNL-03/D-05). Server order is preserved — do NOT re-sort.
  const reviewedSection =
    reviewed.length > 0 ? (
      <>
        <div className="mt-2 flex items-baseline gap-2 border-t border-border pt-2">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Recently reviewed
          </span>
          <span className="text-xs font-medium text-muted-foreground">
            {reviewed.length}
          </span>
        </div>
        {reviewed.map((pr) => (
          <PRCard key={`reviewed-${pr.number}`} pr={pr} projectId={projectId} />
        ))}
      </>
    ) : null;

  // Degraded — gh missing / unauthenticated / error, AND no cached prs to show.
  // (`disabled` never reaches here — the render gate returns null first.) A
  // quiet inline amber note; the refresh button stays active.
  const degraded =
    data.state === "no_gh" ||
    data.state === "auth_required" ||
    data.state === "error";
  if (degraded && prs.length === 0) {
    let heading: string;
    let body: React.ReactNode;
    switch (data.state) {
      case "no_gh":
        heading = "GitHub CLI not found.";
        body = "Install gh to see review requests.";
        break;
      case "auth_required":
        heading = "GitHub not connected.";
        body = <>Run `gh auth login` to see review requests.</>;
        break;
      default:
        heading = "Couldn't reach GitHub.";
        body = "Try refresh.";
    }
    return (
      <>
        <div className="flex items-start gap-2 text-xs text-muted-foreground">
          <TriangleAlert className="size-3.5 shrink-0 text-amber-400" />
          <div>
            <div>{heading}</div>
            <div>{body}</div>
          </div>
        </div>
        {staleFooter}
      </>
    );
  }

  // Empty — clean read, zero AWAITING PRs. The "caught up" copy STAYS even when
  // reviewed has items (UI-SPEC §State Matrix: do NOT suppress the empty block);
  // the reviewedSection renders below it (quiet-omitted if reviewed is empty too).
  if (data.state === "ok" && prs.length === 0) {
    return (
      <>
        <div className="flex flex-col gap-1 py-6 text-center">
          <div className="text-sm text-muted-foreground">
            You're all caught up
          </div>
          <div className="text-xs text-muted-foreground">
            No PRs are waiting for your review.
          </div>
        </div>
        {reviewedSection}
      </>
    );
  }

  // List — server order is already most-recently-updated first (D-14); do NOT
  // re-sort, and do NOT locally retain reviewed PRs (self-emptying, PITFALL 2).
  // When stale-with-cache, still render the prs above the amber stale footer —
  // the column never goes blank just because a refresh failed.
  return (
    <>
      {prs.map((pr) => (
        <PRCard key={pr.number} pr={pr} projectId={projectId} />
      ))}
      {staleFooter}
      {reviewedSection}
    </>
  );
}

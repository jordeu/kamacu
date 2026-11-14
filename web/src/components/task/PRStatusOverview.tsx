import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Popover } from "radix-ui";
import {
  Bot, Check, CircleCheck, CircleHelp, CircleMinus, CircleX, Clock,
  GitMerge, GitPullRequest, GitPullRequestClosed, GitPullRequestDraft,
  History, MessageSquare, TriangleAlert, UserRound,
} from "lucide-react";
import type { PRStatusGroup, PRStatusSnapshot } from "@/api/prStatus";
import { primaryPRStatus, prStatusLabels, statusTone, type StatusTone } from "@/lib/prStatus";
import { cn } from "@/lib/utils";

const colors: Record<StatusTone, string> = {
  green: "text-green-400", red: "text-red-400", amber: "text-amber-400", neutral: "text-zinc-400",
};
const icons = { pass: CircleCheck, fail: CircleX, waiting: Clock, unknown: CircleHelp, none: CircleMinus };

export function PRLifecycleIcon({ state }: { state?: string }) {
  const Icon = state === "merged" ? GitMerge : state === "closed" ? GitPullRequestClosed
    : state === "draft" ? GitPullRequestDraft : GitPullRequest;
  return <Icon aria-hidden="true" className={cn("size-3.5 shrink-0", state === "merged" ? "text-purple-400"
    : state === "closed" ? "text-red-400" : state === "open" ? "text-green-400" : "text-zinc-400")} />;
}

function ReviewGroup({ group, bots = false }: { group?: PRStatusGroup; bots?: boolean }) {
  const Icon = bots ? Bot : UserRound;
  const label = bots ? "Bot feedback" : "Human reviews";
  return <section aria-label={label} className="border-t border-zinc-700/80 pt-3">
    <div className="mb-2 flex items-center justify-between gap-2 text-[10px] uppercase tracking-wide">
      <h4 className="flex items-center gap-1.5 text-zinc-400"><Icon aria-hidden="true" className="size-3.5" />{label}</h4>
      <span className={colors[statusTone(group?.state ?? "unknown")]}>{prStatusLabels[group?.state ?? "unknown"]}</span>
    </div>
    {!group?.reviewers.length ? <p className="text-xs text-zinc-400">{group?.state === "none"
      ? (bots ? "No bot review reported." : "No human review decision or request.") : "Review information unavailable."}</p>
      : <ul className="space-y-2">{group.reviewers.map((reviewer) => {
        const Mark = reviewer.state === "approved" || reviewer.state === "clean" || reviewer.state === "complete" ? Check
          : reviewer.state === "comments" || reviewer.state === "commented" ? MessageSquare
            : reviewer.state === "changes" ? TriangleAlert : reviewer.state === "stale" ? History
              : reviewer.state === "waiting" || reviewer.state === "running" ? Clock : CircleHelp;
        return <li key={reviewer.login} className="text-xs">
          <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
            <span className="min-w-0 break-all text-zinc-200">{reviewer.login.startsWith("team:") ? `Team: ${reviewer.login.slice(5)}` : reviewer.login ? `@${reviewer.login}` : "Unknown reviewer"}</span>
            <span className={cn("inline-flex items-center gap-1 text-[11px]", colors[statusTone(reviewer.state)])}>
              <Mark aria-hidden="true" className="size-3" />
              {reviewer.findings ? `${reviewer.findings} open ${reviewer.findings === 1 ? "finding" : "findings"}` : prStatusLabels[reviewer.state] ?? "Unknown"}
            </span>
          </div>
          {reviewer.detail && <p className="mt-0.5 text-[10px] text-zinc-400">{reviewer.detail}</p>}
        </li>;
      })}</ul>}
    {bots && <p className="mt-2 text-[10px] text-zinc-400">“No new findings” is a bot outcome, not a human approval.</p>}
  </section>;
}

export function PRStatusDetails({ snapshot, repository, number, showJobs = false }: {
  snapshot?: PRStatusSnapshot; repository: string; number: number; showJobs?: boolean;
}) {
  const primary = primaryPRStatus(snapshot);
  const Mark = icons[primary.icon];
  const ciState = snapshot?.ci.state ?? "unknown";
  const CIMark = icons[ciState === "pass" || ciState === "fail" || ciState === "none" ? ciState : ciState === "pending" ? "waiting" : "unknown"];
  const ciLabel = ciState === "pass" ? "All checks passing" : ciState === "fail" ? "Checks failing"
    : ciState === "pending" ? "Checks pending" : ciState === "none" ? "No checks" : "CI unavailable";
  return <div className="space-y-3">
    <header className="space-y-1.5">
      <h3 className={cn("flex items-center gap-2 text-xs font-semibold", colors[primary.tone])}>
        <Mark aria-hidden="true" className="size-4 shrink-0" />{primary.label}
      </h3>
      <p className="break-words text-[11px] text-zinc-400">{repository} · PR #{number}
        {snapshot && <> · {snapshot.lifecycle}{snapshot.head && <> · <span className="font-mono">{snapshot.head.slice(0, 7)}</span></>}</>}
      </p>
    </header>
    <section aria-label="CI checks" className="flex items-center justify-between gap-2 border-t border-zinc-700/80 pt-3">
      <h4 className="text-[10px] uppercase tracking-wide text-zinc-400">CI checks</h4>
      <span className={cn("inline-flex items-center gap-1.5 text-xs", colors[statusTone(ciState)])}>
        <CIMark aria-hidden="true" className="size-3.5 shrink-0" />{ciLabel}
      </span>
    </section>
    {showJobs && !!snapshot?.ci.jobs.length && <details className="border-t border-zinc-700/80 pt-3">
      <summary className="cursor-pointer text-xs">All {snapshot.ci.jobs.length} checks</summary>
      <ul className="mt-3 space-y-2">{snapshot.ci.jobs.map((job, index) => <li key={`${job.name}:${index}`} className="text-xs">
        <div className="flex justify-between gap-3"><span className="min-w-0 break-words">{job.name}</span><span className={colors[statusTone(job.state)]}>{prStatusLabels[job.state] ?? job.state}</span></div>
        {job.detail && <p className="text-[10px] text-zinc-400">{job.detail}</p>}
      </li>)}</ul>
    </details>}
    <ReviewGroup group={snapshot?.humans} />
    <ReviewGroup group={snapshot?.bots} bots />
    <footer className="space-y-1 border-t border-zinc-700/80 pt-2 text-[10px] text-zinc-400">
      {snapshot?.fetched_at && <p>{snapshot.stale ? "Last known snapshot" : "Updated"}: <time dateTime={snapshot.fetched_at}>{new Date(snapshot.fetched_at).toLocaleString()}</time></p>}
      {snapshot?.error && <p role="status">{snapshot.error}</p>}
      <p>Green describes human approval and CI, not merge eligibility.</p>
    </footer>
  </div>;
}

/** Hover/focus previews the overview; clicking pins it for touch and keyboard.
 * The anchor and portal stop all three board drag sensors independently. */
export function PRStatusOverview({ snapshot, repository, number, reference, title, previewContent, onPreviewChange }: {
  snapshot?: PRStatusSnapshot; repository: string; number: number;
  reference?: { url: string; label: string };
  title?: string;
  previewContent?: ReactNode;
  onPreviewChange?: (open: boolean) => void;
}) {
  const [open, setOpenState] = useState(false);
  const setOpen = useCallback((value: boolean) => {
    setOpenState(value);
    onPreviewChange?.(value);
  }, [onPreviewChange]);
  const pinned = useRef(false);
  const restoreFocus = useRef(false);
  const skipFocusPreview = useRef(false);
  const suppressed = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const trigger = useRef<HTMLButtonElement>(null);
  const referenceTrigger = useRef<HTMLAnchorElement>(null);
  const content = useRef<HTMLDivElement>(null);
  // Drag isolation stops bubbling pointer events. Use capture for outside
  // dismissal so an earlier pointer event inside the portal cannot swallow it.
  useEffect(() => {
    if (!open || reference) return;
    const dismiss = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || trigger.current?.contains(event.target) || content.current?.contains(event.target)) return;
      clearTimeout(timer.current);
      pinned.current = false;
      suppressed.current = true;
      setOpen(false);
    };
    document.addEventListener("pointerdown", dismiss, true);
    return () => document.removeEventListener("pointerdown", dismiss, true);
  }, [open, reference, setOpen]);
  const id = useId();
  const primary = primaryPRStatus(snapshot);
  const Mark = icons[primary.icon];
  const bots = snapshot?.bots.state;
  const BotMark = bots === "comments" ? MessageSquare : bots === "changes" ? TriangleAlert
    : bots === "running" || bots === "waiting" ? Clock : bots === "stale" ? History : CircleHelp;
  const showBot = !!snapshot?.fetched_at && bots !== "clean" && bots !== "complete" && bots !== "none";
  const clear = () => clearTimeout(timer.current);
  const show = () => { clear(); if (!suppressed.current) setOpen(true); };
  const showFromPointer = (event: { pointerType: string }) => {
    if (event.pointerType === "touch") return;
    // A real re-entry is a new preview request, even after Escape dismissed it.
    suppressed.current = false;
    show();
  };
  const showReference = (event: { pointerType: string }) => {
    if (event.pointerType === "touch") return;
    clear();
    suppressed.current = false;
    // Ignore incidental pointer crossings before loading the preview.
    timer.current = setTimeout(() => setOpen(true), 120);
  };
  const hideLater = () => {
    clear();
    timer.current = setTimeout(() => {
      if (!pinned.current && document.activeElement !== trigger.current && document.activeElement !== referenceTrigger.current && !content.current?.contains(document.activeElement)) setOpen(false);
    }, 180);
  };
  const controls = <>
      {reference && <a ref={referenceTrigger} href={reference.url} target="_blank" rel="noopener noreferrer"
          className="inline-flex min-w-0 items-center gap-1 rounded-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onFocus={(event) => { if (event.currentTarget.matches(":focus-visible")) { suppressed.current = false; show(); } }} onBlur={hideLater}
          onClick={clear}>
          <GitPullRequest aria-hidden="true" className="size-3 shrink-0" />
          <span className="break-all">{reference.label}</span>
        </a>}
      <Popover.Trigger asChild>
        <button ref={trigger} type="button" aria-haspopup="dialog" aria-expanded={open} aria-controls={id}
          aria-label={`${reference ? `${repository} PR ${number}. ` : ""}${primary.label}. Bots: ${prStatusLabels[bots ?? "unknown"]}. Show PR status overview.`}
          className="inline-flex size-7 cursor-pointer items-center justify-center rounded-sm hover:bg-zinc-700/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onPointerEnter={showFromPointer} onPointerLeave={() => { suppressed.current = false; hideLater(); }}
          onFocus={() => { if (skipFocusPreview.current) { skipFocusPreview.current = false; return; } suppressed.current = false; show(); }} onBlur={hideLater}
          onKeyDown={(event) => {
            if (event.key === "Tab" && !event.shiftKey && open) {
              event.preventDefault(); pinned.current = true; restoreFocus.current = true; content.current?.focus();
            }
          }}
          onClick={(event) => {
            event.preventDefault(); clear(); const next = !(pinned.current && open);
            pinned.current = next; suppressed.current = !next; setOpen(next);
            if (next && event.detail === 0) {
              restoreFocus.current = true;
              requestAnimationFrame(() => content.current?.focus());
            }
          }}>
          <span className="relative inline-flex size-5 items-center justify-center">
            <Mark aria-hidden="true" className={cn("size-[18px]", colors[primary.tone])} />
            {showBot && <span className={cn("absolute -right-1 -bottom-0.5 flex size-3.5 items-center justify-center rounded-sm bg-card", colors[statusTone(bots ?? "unknown")])}>
              <BotMark aria-hidden="true" className="size-3" />
            </span>}
          </span>
        </button>
      </Popover.Trigger>
  </>;
  return <span
    className={reference ? "inline-flex min-w-0 items-center gap-1 rounded-md border border-orange-300/20 bg-orange-300/5 px-2 py-1 text-xs text-orange-200" : "inline-flex shrink-0"}
    onPointerEnter={reference ? showReference : undefined}
    onPointerLeave={reference ? () => { suppressed.current = false; hideLater(); } : undefined}
    onClick={(event) => event.stopPropagation()}
    onPointerDown={(event) => event.stopPropagation()}
    onMouseDown={(event) => event.stopPropagation()}
    onTouchStart={(event) => event.stopPropagation()}
    onKeyDown={(event) => event.stopPropagation()}
  >
    <Popover.Root open={open} onOpenChange={(value) => { clear(); pinned.current = false; suppressed.current = !value; setOpen(value); }}>
      {/* Anchor both controls so positioning stays attached to a mounted node
          when the trigger removes its initial fallback anchor. */}
      {reference ? <Popover.Anchor asChild><span className="inline-flex min-w-0 items-center gap-1">{controls}</span></Popover.Anchor> : controls}
      <Popover.Portal>
        <Popover.Content ref={content} id={id} tabIndex={0} aria-label={`${repository} PR ${number} status overview`}
          sideOffset={6} collisionPadding={12} align="start"
          onPointerEnter={clear} onPointerLeave={hideLater}
          onFocus={() => { pinned.current = true; restoreFocus.current = true; }}
          onInteractOutside={(event) => {
            // The reference belongs to the preview's controls; preserve its
            // native link action when it receives focus or pointer input.
            if (event.target instanceof Node && referenceTrigger.current?.contains(event.target)) event.preventDefault();
          }}
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (restoreFocus.current && (document.activeElement === document.body || content.current?.contains(document.activeElement))) {
              skipFocusPreview.current = true;
              trigger.current?.focus();
            }
            restoreFocus.current = false;
          }}
          className="z-50 max-h-[min(600px,var(--radix-popover-content-available-height))] w-[330px] max-w-[calc(100vw-24px)] overflow-y-auto rounded-lg border border-zinc-600 bg-zinc-800 p-4 text-zinc-100 shadow-xl outline-none">
          {title && <h3 className="mb-3 break-words text-sm font-semibold">{title}</h3>}
          {previewContent}
          <PRStatusDetails snapshot={snapshot} repository={repository} number={number} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  </span>;
}

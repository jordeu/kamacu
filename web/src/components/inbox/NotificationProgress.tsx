import { useEffect, useState } from "react";
import { LoaderCircle } from "lucide-react";
import type { NotificationProcessing } from "@/api/notifications";

const stages: Record<string, { label: string; step: number }> = {
  preparing: { label: "Preparing notifications", step: 0 },
  fetching: { label: "Fetching activity from GitHub", step: 0 },
  gathering_context: { label: "Gathering related tasks and inbox history", step: 0 },
  building_context: { label: "Preparing activity for analysis", step: 0 },
  analyzing: { label: "Analyzing notifications", step: 1 },
  reading_context: { label: "Looking up additional context", step: 1 },
  checking_updates: { label: "Checking discussions for new activity", step: 2 },
  revising: { label: "Updating decisions with the latest activity", step: 2 },
  saving: { label: "Saving processing results", step: 3 },
  publishing: { label: "Creating inbox messages", step: 3 },
};

function elapsed(milliseconds: number) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

export function NotificationProgress({ run, expanded }: { run: NotificationProcessing; expanded: boolean }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const progress = run.progress;
  const stage = run.status === "queued"
    ? { label: "Queued · waiting to start", step: 0 }
    : stages[progress?.stage ?? ""] ?? { label: "Preparing notifications", step: 0 };
  const counted = progress && progress.total > 0;
  return <div className={expanded ? "space-y-3 rounded-lg border border-border bg-background/60 p-4" : "space-y-2 text-xs"}>
    <div role="status" aria-live="polite" className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <span className="inline-flex items-center gap-2 font-medium"><LoaderCircle aria-hidden="true" className="size-4 animate-spin text-primary" />{stage.label}</span>
      {counted && <span className="text-sm tabular-nums text-muted-foreground">{progress.completed} of {progress.total} discussions</span>}
    </div>
    {counted && <div role="progressbar" aria-label={stage.label} aria-valuemin={0} aria-valuemax={progress.total} aria-valuenow={progress.completed} className="h-1.5 overflow-hidden rounded-full bg-muted">
      <div className="h-full rounded-full bg-primary transition-[width]" style={{ width: `${Math.min(100, Math.max(0, progress.completed / progress.total * 100))}%` }} />
    </div>}
    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums text-muted-foreground">
      <span>Elapsed {elapsed(now - run.created_at)}</span>
      {progress && <>
        <span>Current stage {elapsed(now - progress.stage_started_at)}</span>
        {progress.github_requests > 0 && <span>{progress.github_requests} GitHub requests</span>}
        {progress.model_passes > 0 && <span>Analysis pass {progress.model_passes}</span>}
        {progress.lookups > 0 && <span>{progress.lookups} context {progress.lookups === 1 ? "lookup" : "lookups"}</span>}
      </>}
    </div>
    {expanded && <ol aria-label="Processing stages" className="grid grid-cols-2 gap-2 border-t border-border pt-3 text-xs sm:grid-cols-4">
      {["Collect activity", "Analyze", "Check updates", "Create messages"].map((label, index) => <li key={label} aria-current={index === stage.step ? "step" : undefined} className={`flex items-center gap-2 ${index === stage.step ? "font-medium text-foreground" : "text-muted-foreground"}`}>
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${index <= stage.step ? "bg-primary" : "bg-muted-foreground/30"}`} />{label}
      </li>)}
    </ol>}
  </div>;
}

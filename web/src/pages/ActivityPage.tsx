import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ActivityList } from "@/components/activity/ActivityList";
import { ReviewsList } from "@/components/activity/ReviewsList";
import { ScopeSelector } from "@/components/activity/ScopeSelector";
import { StatsStrip } from "@/components/activity/StatsStrip";
import { WindowToggle } from "@/components/activity/WindowToggle";
import { useActivity } from "@/api/queries";
import { useActivityView } from "@/lib/useActivityView";

/**
 * Phase 11 — the top-level Activity page (CONTEXT discretion; mirrors the
 * SettingsPage.tsx shell verbatim). Renders the GET /api/activity contract
 * (Phase 10 D-01: one endpoint → one query, one loading state) as a stacked
 * single column (D-08): control bar (scope dropdown + window toggle) → stats
 * strip → tasks-done section → reviews-done section.
 *
 * scope/window live in localStorage (kamacu.activity.*), NEVER the URL
 * (D-06) — so `/activity` is a bare route with no path params and is NOT
 * wrapped in BoardWorkspaceSync (which reconciles a URL :projectId).
 */
export default function ActivityPage() {
  const { scope, setScope, window, setWindow } = useActivityView();
  const { data, isLoading, isError, refetch } = useActivity(scope, window);

  if (isError) {
    // Mirrors SettingsPage.tsx:128-138 / BoardPage load-failure pattern.
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-muted-foreground">{`Couldn't load activity.`}</p>
        <Button variant="outline" onClick={() => refetch()}>
          {`Retry loading`}
        </Button>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="max-w-[640px]">
        <h1 className="text-base font-medium">{`Activity`}</h1>
        {isLoading || !data ? (
          <div className="mt-6 flex flex-col gap-6">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex flex-col gap-2">
                <Skeleton className="h-3 w-24" />
                <Skeleton className="h-8 w-full" />
              </div>
            ))}
          </div>
        ) : (
          <div className="mt-6 flex flex-col gap-6">
            <div className="flex items-center gap-2">
              <ScopeSelector scope={scope} onScopeChange={setScope} />
              <WindowToggle window={window} onWindowChange={setWindow} />
            </div>
            <StatsStrip stats={data.stats} />
            <ActivityList tasks={data.tasks ?? []} />
            <ReviewsList reviews={{ ...data.reviews, prs: data.reviews.prs ?? [] }} />
          </div>
        )}
      </div>
    </div>
  );
}

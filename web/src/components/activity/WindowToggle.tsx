import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { ActivityWindow } from "@/lib/useActivityView";

/**
 * Phase 11 — the Week/Month segmented control (CONTEXT D-07). Default-variant
 * `Tabs` IS visually a segmented control (tabs.tsx renders a `bg-muted` list
 * with a `data-active:bg-background` pill), so this is zero-new-code reuse of
 * an existing primitive.
 *
 * Toggling writes `kamacu.activity.window` (via the onWindowChange callback
 * wired to useActivityView) and updates the useActivity query key — one
 * refetch, one loading state (Phase 10 D-01).
 */
export function WindowToggle({
  window,
  onWindowChange,
}: {
  window: ActivityWindow;
  onWindowChange: (w: ActivityWindow) => void;
}) {
  return (
    <Tabs
      value={window}
      onValueChange={(v) => onWindowChange(v as ActivityWindow)}
    >
      <TabsList>
        <TabsTrigger value="week">{`Week`}</TabsTrigger>
        <TabsTrigger value="month">{`Month`}</TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

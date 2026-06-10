import type { ReactNode } from "react";
import { X } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

/**
 * The task view's tab strip is the architectural seam for the whole product
 * (D-06): Phase 1 ships only `{ id: "description", label: "Description" }`,
 * later phases append Agent, Bash, and Diff tabs to the same `TabDef[]` array.
 *
 * Phase 3: controlled (Pitfall 7 — the page owns `value` so closing the
 * active tab can reassign it in the same update), closable triggers (D-29),
 * a trailing slot for the `+` spawn button (D-27), and kept-mounted terminal
 * content so switching tabs never detaches a live WebSocket.
 */
export interface TabDef {
  id: string;
  label: string;
  content: ReactNode;
  /** Renders the × affordance; closing stops the session (D-29). */
  onClose?: () => void;
  /** Muted label — closing or exited sessions (liveness by muting). */
  muted?: boolean;
  /** Keep content mounted while inactive (terminal tabs: WS must survive). */
  keepMounted?: boolean;
  /** Rendered before the label (Agent tab status dot, D-50). */
  leading?: ReactNode;
}

export function TaskTabs({
  tabs,
  value,
  onValueChange,
  trailing,
}: {
  tabs: TabDef[];
  value: string;
  onValueChange: (v: string) => void;
  trailing?: ReactNode;
}) {
  if (tabs.length === 0) return null;

  return (
    <Tabs
      value={value}
      onValueChange={onValueChange}
      className="min-h-0 flex-1 gap-6"
    >
      {/* Visible strip even with a single tab — the seam must be visible.
          Never wraps: scrolls horizontally when bash tabs exceed the width. */}
      <TabsList
        variant="line"
        className="w-full shrink-0 justify-start overflow-x-auto border-b border-border"
      >
        {tabs.map((tab) => (
          <TabsTrigger
            key={tab.id}
            value={tab.id}
            className="flex-none px-2 data-active:after:bg-blue-500"
          >
            {/* dot + 4px gap + label (UI-SPEC) — trigger height unchanged */}
            <span className="flex items-center gap-1">
              {tab.leading}
              <span
                className={tab.muted ? "text-muted-foreground" : undefined}
              >
                {tab.label}
              </span>
            </span>
            {tab.onClose && (
              <Tooltip>
                <TooltipTrigger asChild>
                  {/* span[role=button], NOT <button> — button-in-button is
                      invalid HTML inside the TabsTrigger. Never red: closing
                      a bash tab is routine (D-29), not destructive. */}
                  <span
                    role="button"
                    tabIndex={0}
                    aria-label={`Stop ${tab.label} and close tab`}
                    className="ml-1 rounded-sm p-0.5 text-zinc-400 outline-none hover:bg-[#27272a] hover:text-zinc-50 focus-visible:ring-2 focus-visible:ring-blue-500"
                    onClick={(e) => {
                      e.stopPropagation();
                      tab.onClose?.();
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        e.stopPropagation();
                        tab.onClose?.();
                      }
                    }}
                  >
                    <X className="size-3" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>Stop and close</TooltipContent>
              </Tooltip>
            )}
          </TabsTrigger>
        ))}
        {trailing != null && (
          <div className="ml-2 flex items-center">{trailing}</div>
        )}
      </TabsList>
      {tabs.map((tab) =>
        tab.keepMounted ? (
          // forceMount + hidden-when-inactive: switching tabs never unmounts
          // the TerminalPane (its fit guard defers fitting while hidden).
          <TabsContent
            key={tab.id}
            value={tab.id}
            forceMount
            className="min-h-0 data-[state=inactive]:hidden"
          >
            {tab.content}
          </TabsContent>
        ) : (
          <TabsContent key={tab.id} value={tab.id} className="min-h-0">
            {tab.content}
          </TabsContent>
        ),
      )}
    </Tabs>
  );
}

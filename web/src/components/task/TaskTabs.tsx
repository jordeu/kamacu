import { useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";
import { Input } from "@/components/ui/input";
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
  /** Renders the trigger disabled (zinc-600 label) with an explanation tooltip. */
  disabled?: boolean;
  disabledTooltip?: string;
  /**
   * Enables double-click inline rename on this tab's label (TABS-01, D-02).
   * Set ONLY for bash/tmux tabs — Agent, Description, and Diff pass no
   * `onRename`, so they keep fixed labels. Called on commit with the trimmed
   * label; a trimmed-empty commit passes `""` as an explicit reset intent (the
   * server re-derives the `Bash N` default — D-05).
   */
  onRename?: (label: string) => void;
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
  // Inline tab-rename state (TABS-01, D-01) — mirrors TaskPage's title-edit
  // contract: a per-tab `renamingId`, the working `draft`, and a `cancelRef`
  // set on Escape and read-and-cleared in the commit path so the blur that
  // Escape triggers does NOT save.
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const cancelRenameRef = useRef(false);

  function commitRename(tab: TabDef, value: string) {
    const cancelled = cancelRenameRef.current;
    cancelRenameRef.current = false;
    setRenamingId(null);
    if (cancelled) return;
    const trimmed = value.trim();
    // D-05 divergence from the title editor: a trimmed-empty commit is an
    // explicit reset (server re-derives `Bash N`) — so DO NOT early-return on
    // empty. Send "" for a reset; otherwise send the trimmed value only when
    // it actually changed.
    if (trimmed === "") {
      tab.onRename?.("");
    } else if (trimmed !== tab.label) {
      tab.onRename?.(trimmed);
    }
  }

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
        {tabs.map((tab) =>
          tab.disabled ? (
            // Disabled tab (no worktree): the trigger swallows pointer events,
            // so the explanation tooltip fires on a wrapping span — mirror of
            // the bash `+` disabled pattern. Label goes zinc-600 (disabled).
            <Tooltip key={tab.id}>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <TabsTrigger
                    value={tab.id}
                    disabled
                    className="flex-none px-2 data-active:after:bg-blue-500"
                  >
                    <span className="flex items-center gap-1">
                      {tab.leading}
                      <span className="text-zinc-600">{tab.label}</span>
                    </span>
                  </TabsTrigger>
                </span>
              </TooltipTrigger>
              {tab.disabledTooltip && (
                <TooltipContent>{tab.disabledTooltip}</TooltipContent>
              )}
            </Tooltip>
          ) : (
            <TabsTrigger
              key={tab.id}
              value={tab.id}
              className="flex-none px-2 data-active:after:bg-blue-500"
            >
              {/* dot + 4px gap + label (UI-SPEC) — trigger height unchanged */}
              <span className="flex items-center gap-1">
                {tab.leading}
                {renamingId === tab.id ? (
                  // Inline rename editor (TABS-01, D-01) — copies the TaskPage
                  // title contract: Enter→blur (commit), Esc→cancelRef+blur
                  // (no save), blur→commit. Lives INSIDE TabsTrigger next to
                  // the × span, so — like the × (span[role=button], not a real
                  // <button>) — pointer/click events stopPropagation to keep
                  // the Radix trigger's select from firing while typing.
                  <Input
                    autoFocus
                    value={draft}
                    className="h-6 w-32 px-1 py-0 text-sm"
                    onChange={(e) => setDraft(e.target.value)}
                    onBlur={(e) => commitRename(tab, e.currentTarget.value)}
                    onClick={(e) => e.stopPropagation()}
                    onPointerDown={(e) => e.stopPropagation()}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.currentTarget.blur();
                      } else if (e.key === "Escape") {
                        cancelRenameRef.current = true;
                        e.currentTarget.blur();
                      }
                    }}
                  />
                ) : (
                  <span
                    className={tab.muted ? "text-muted-foreground" : undefined}
                    // Double-click enters rename mode (D-01) only when the tab
                    // offers `onRename` (bash/tmux — D-02). stopPropagation so
                    // the double-click does not also toggle selection; a
                    // single-click still selects via the Radix trigger.
                    onDoubleClick={
                      tab.onRename
                        ? (e) => {
                            e.stopPropagation();
                            setDraft(tab.label);
                            setRenamingId(tab.id);
                          }
                        : undefined
                    }
                  >
                    {tab.label}
                  </span>
                )}
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
          ),
        )}
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

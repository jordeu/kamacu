import { useState } from "react";
import { ChevronRight } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import { useToggleViewed, type DiffFile } from "@/api/diffs";

// GitHub-style STACKING headers: each file header is a `sticky top-0` sibling in
// the shared flat list. The Collapsible root is `display:contents` (no box), so
// it doesn't box-in the header — the header's containing block is the whole list,
// letting the current file's header stay pinned just under the fixed totals bar
// until the next file's header pushes it up. Result: always exactly one header
// pinned, and never raw content directly under the bar.
const STICKY_TOP = "top-0";

// blue-500 checked override (UI-SPEC Decision 2): the default checkbox fill is
// the near-white `primary` in dark; this surface's interactive accent is blue.
const CHECKBOX_BLUE =
  "data-[state=checked]:border-blue-500 data-[state=checked]:bg-blue-500 data-[state=checked]:text-white focus-visible:ring-blue-500";

/**
 * One file's unified diff (REVW-01, D-62). Loud colored change lines inside
 * quiet neutral chrome (05-UI-SPEC diff content palette — scoped to the Diff
 * tab only; never bleeds into app chrome). Read-only: selectable text, per-file
 * horizontal scroll, no per-line actions of any kind.
 *
 * The sticky file-header row carries a blue-500 "Viewed" checkbox (DIFF-03) as
 * a SIBLING of the collapse trigger (never a child — button-in-button is
 * invalid HTML; same rule as the TaskTabs × close). Marking Viewed collapses +
 * dims the file; unchecking re-expands + restores contrast. Collapse and Viewed
 * are independent: the Collapsible is controlled, so manual collapse never
 * touches viewed state. A changed file (new hash) remounts un-viewed + expanded
 * because DiffTab keys sections on `path:hash` (DIFF-04).
 *
 * Binary files render header-only ("Binary file changed"), not expandable; they
 * still get the Viewed checkbox (collapse-on-view is a harmless no-op — D-04).
 */
export function DiffFileSection({
  file,
  taskId,
}: {
  file: DiffFile;
  taskId: number;
}) {
  const changedLines = (file.additions ?? 0) + (file.deletions ?? 0);
  const toggle = useToggleViewed(taskId);
  const [open, setOpen] = useState(!file.viewed && changedLines <= 400);

  // Path slot: renamed files show "{oldPath} → {path}".
  const pathLabel =
    file.status === "renamed" && file.oldPath
      ? `${file.oldPath} → ${file.path}`
      : file.path;

  // Status suffix: only new/deleted get a muted label; modified gets none.
  const statusSuffix =
    file.status === "new" ? "new" : file.status === "deleted" ? "deleted" : null;

  // aria copy uses the path leaf (sentence case).
  const filename = file.path.split("/").pop() ?? file.path;
  const viewedLabel = (
    <label className="flex shrink-0 cursor-pointer items-center gap-1.5">
      <Checkbox
        checked={file.viewed}
        onClick={(e) => e.stopPropagation()}
        onCheckedChange={(checked) => {
          const viewed = checked === true;
          toggle.mutate({ path: file.path, hash: file.hash, viewed });
          // Collapse on view / re-expand on un-view. Harmless for the binary
          // branch which has no collapsible content.
          setOpen(!viewed);
        }}
        aria-label={
          file.viewed
            ? `Mark ${filename} as not viewed`
            : `Mark ${filename} as viewed`
        }
        className={CHECKBOX_BLUE}
      />
      <span className="text-xs text-muted-foreground">Viewed</span>
    </label>
  );

  // Non-sticky zero-height jump anchor at the file's flow top. Tree-click jumps
  // target THIS (via offsetTop), never the sticky header — a sticky element's
  // offsetTop/rect reflects its pinned position, which breaks upward jumps.
  const anchor = (
    <div data-diff-anchor={file.path} aria-hidden className="h-0" />
  );

  if (file.binary) {
    // Non-collapsible sticky header row — chevron slot empty, stats replaced by
    // the muted "Binary file changed" copy. Still carries the Viewed checkbox.
    return (
      <>
        {anchor}
        <div
          data-diff-path={file.path}
          className={cn(
            "sticky z-[5] flex items-center gap-2 border-b border-border bg-card px-3 py-2",
            STICKY_TOP,
          )}
        >
          <span className="inline-block size-4 shrink-0" />
          <span
            className={cn(
              "min-w-0 flex-1 truncate text-left font-mono text-sm",
              file.viewed && "text-muted-foreground",
            )}
          >
            {pathLabel}
          </span>
          {statusSuffix && (
            <span className="text-xs text-muted-foreground">
              {statusSuffix}
            </span>
          )}
          <span className="text-xs text-muted-foreground">
            Binary file changed
          </span>
          {viewedLabel}
        </div>
      </>
    );
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="group/diff-file contents"
    >
      {anchor}
      <div
        data-diff-path={file.path}
        className={cn(
          "sticky z-[5] flex items-center gap-2 border-b border-border bg-card px-3 py-2",
          STICKY_TOP,
        )}
      >
        <CollapsibleTrigger className="flex min-w-0 flex-1 items-center gap-1 rounded-sm text-left outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500">
          <ChevronRight className="size-4 shrink-0 transition-transform group-data-[state=open]/diff-file:rotate-90" />
          <span
            className={cn(
              "min-w-0 flex-1 truncate text-left font-mono text-sm",
              file.viewed && "text-muted-foreground",
            )}
          >
            {pathLabel}
          </span>
          {statusSuffix && (
            <span className="text-xs text-muted-foreground">{statusSuffix}</span>
          )}
          <span
            className={cn(
              "flex items-center gap-1 text-xs font-medium tabular-nums",
              file.viewed && "text-muted-foreground",
            )}
          >
            <span className={file.viewed ? undefined : "text-green-400"}>
              {`+${file.additions ?? 0}`}
            </span>
            <span className={file.viewed ? undefined : "text-red-400"}>
              {`−${file.deletions ?? 0}`}
            </span>
          </span>
        </CollapsibleTrigger>
        {viewedLabel}
      </div>
      <CollapsibleContent>
        <div className="overflow-x-auto border-b border-border bg-zinc-950">
          {file.hunks.map((hunk, hi) => (
            <div key={hi}>
              <div className="w-fit min-w-full bg-card px-3 py-2 font-mono text-xs whitespace-pre text-zinc-400">
                {hunk.header}
              </div>
              {hunk.lines.map((line, li) => (
                <div
                  key={li}
                  className={
                    line.kind === "add"
                      ? "w-fit min-w-full pl-3 font-mono text-xs leading-normal whitespace-pre bg-green-500/10"
                      : line.kind === "del"
                        ? "w-fit min-w-full pl-3 font-mono text-xs leading-normal whitespace-pre bg-red-500/10"
                        : "w-fit min-w-full pl-3 font-mono text-xs leading-normal whitespace-pre"
                  }
                >
                  <span
                    className={
                      line.kind === "add"
                        ? "inline-block w-4 shrink-0 text-green-400 select-none"
                        : line.kind === "del"
                          ? "inline-block w-4 shrink-0 text-red-400 select-none"
                          : "inline-block w-4 shrink-0 select-none"
                    }
                  >
                    {line.kind === "add" ? "+" : line.kind === "del" ? "−" : " "}
                  </span>
                  <span
                    className={
                      line.kind === "context" ? "text-zinc-400" : "text-zinc-50"
                    }
                  >
                    {line.text}
                  </span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

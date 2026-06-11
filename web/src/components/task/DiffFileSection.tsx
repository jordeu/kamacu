import { ChevronRight } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import type { DiffFile } from "@/api/diffs";

/**
 * One file's unified diff (REVW-01, D-62). Loud colored change lines inside
 * quiet neutral chrome (05-UI-SPEC diff content palette — scoped to the Diff
 * tab only; never bleeds into app chrome). Read-only: selectable text, per-file
 * horizontal scroll, no per-line actions of any kind.
 *
 * Binary files render header-only ("Binary file changed"), not expandable.
 * Otherwise the entire header row is the Radix Collapsible trigger; files with
 * more than 400 changed lines (additions + deletions) start collapsed — Radix
 * unmounts closed content, so a collapsed huge file costs zero DOM.
 */
export function DiffFileSection({ file }: { file: DiffFile }) {
  const changedLines = (file.additions ?? 0) + (file.deletions ?? 0);

  // Path slot: renamed files show "{oldPath} → {path}".
  const pathLabel =
    file.status === "renamed" && file.oldPath
      ? `${file.oldPath} → ${file.path}`
      : file.path;

  // Status suffix: only new/deleted get a muted label; modified gets none.
  const statusSuffix =
    file.status === "new" ? "new" : file.status === "deleted" ? "deleted" : null;

  if (file.binary) {
    // Non-collapsible header row — chevron slot empty, stats replaced by the
    // muted "Binary file changed" copy.
    return (
      <div className="overflow-hidden rounded-lg border border-border">
        <div className="flex items-center gap-1 bg-card px-3 py-2">
          <span className="inline-block size-4 shrink-0" />
          <span className="flex-1 truncate text-left font-mono text-sm">
            {pathLabel}
          </span>
          {statusSuffix && (
            <span className="text-xs text-muted-foreground">{statusSuffix}</span>
          )}
          <span className="text-xs text-muted-foreground">
            Binary file changed
          </span>
        </div>
      </div>
    );
  }

  return (
    <Collapsible
      defaultOpen={changedLines <= 400}
      className="group/diff-file overflow-hidden rounded-lg border border-border"
    >
      <CollapsibleTrigger className="flex w-full items-center gap-1 bg-card px-3 py-2 outline-none hover:bg-[#27272a] focus-visible:ring-2 focus-visible:ring-blue-500">
        <ChevronRight className="size-4 shrink-0 transition-transform group-data-[state=open]/diff-file:rotate-90" />
        <span className="flex-1 truncate text-left font-mono text-sm">
          {pathLabel}
        </span>
        {statusSuffix && (
          <span className="text-xs text-muted-foreground">{statusSuffix}</span>
        )}
        <span className="flex items-center gap-1 text-xs font-medium tabular-nums">
          <span className="text-green-400">{`+${file.additions ?? 0}`}</span>
          <span className="text-red-400">{`−${file.deletions ?? 0}`}</span>
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="overflow-x-auto bg-zinc-950">
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

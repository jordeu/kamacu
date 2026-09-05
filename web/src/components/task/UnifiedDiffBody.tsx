import type { DiffHunk } from "@/api/diffs";

/**
 * The unified-diff body (D-62): loud colored change lines inside quiet neutral
 * chrome (05-UI-SPEC diff content palette — scoped to diff content only, never
 * app chrome). Read-only: selectable text, per-file horizontal scroll, no
 * per-line actions of any kind. Extracted from DiffFileSection so the
 * Markdown viewer's "source" view renders byte-identical markup (MDV-01) —
 * callers own the scroll container + surface chrome.
 */
export function UnifiedDiffBody({ hunks }: { hunks: DiffHunk[] }) {
  return (
    <>
      {hunks.map((hunk, hi) => (
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
                {/* The parse contract (internal/diff/parse.go) keeps the
                    leading +/-/space byte in text — the gutter span above
                    already draws the marker, so strip the byte here or lines
                    render "++added" / "−-removed". */}
                {line.text.slice(1)}
              </span>
            </div>
          ))}
        </div>
      ))}
    </>
  );
}

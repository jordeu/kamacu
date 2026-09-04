import type { DiffHunk } from "@/api/diffs";

/**
 * Markdown-diff attribution (MDV-01): map the Diff tab's line-based hunks onto
 * a top-level Markdown BLOCK tree of the NEW text, so the Markdown viewer can
 * render prose as prose with the change marked in place.
 *
 * Two pure passes (no I/O, no React — deliberately testable):
 *
 *  1. mapHunks — walk each hunk with the counters from its `@@ -a,b +c,d @@`
 *     header and produce (a) the set of NEW-file line numbers that are `add`
 *     lines, and (b) the contiguous `del` runs, each ANCHORED at the NEW-file
 *     line number it sat before (an anchor may equal the document length — a
 *     removal at end of file; two hunks may flush runs at the same anchor,
 *     which merge). This pass is exact.
 *
 *  2. tokenizeBlocks + buildMarkdownDiff — a CommonMark-lite, GFM-aware block
 *     tokenizer assigns every line of the NEW text to exactly one top-level
 *     block (heading / fenced code / table / list / blockquote / thematic
 *     break / HTML / paragraph). A block is ADDED iff any of its lines is an
 *     add line; removed runs interleave as ghost blocks at their anchor, so a
 *     rewritten paragraph reads old-struck-red immediately followed by
 *     new-green. This pass approximates where CommonMark is deep (see each
 *     rule's comment); the blast radius of a mis-grouping is tint granularity
 *     and spacing — never content loss, because every line still renders.
 *
 * Reference-style link definitions are hoisted: they render to nothing
 * themselves, and their source is prepended to every block's render source so
 * per-block rendering never breaks `[text][ref]` links across block splits.
 */

/** Hunk-header `@@ -a[,b] +c[,d] @@` (a missing count means 1 — git rules). */
const RE_HUNK_HEADER = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/** Block-start detectors (0-3 leading spaces per CommonMark). */
const RE_FENCE = /^ {0,3}(`{3,}|~{3,})/;
const RE_HEADING = /^ {0,3}#{1,6}(?:\s|$)/;
const RE_THEMATIC_BREAK = /^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$/;
const RE_QUOTE = /^ {0,3}>/;
const RE_LIST_ITEM = /^ {0,3}([-*+]|\d{1,9}[.)])(?:\s|$)/;
const RE_SETEXT_UNDERLINE = /^ {0,3}(=+|-+)\s*$/;
// GFM delimiter row. Must ALSO contain a pipe — without that guard a bare
// "---" (thematic break / setext) would read as a one-column delimiter.
const RE_TABLE_DELIM =
  /^ {0,3}\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$/;
const RE_REF_DEF = /^ {0,3}\[[^\]]+\]:[ \t]*\S/;
const RE_HTML_BLOCK = /^ {0,3}<(?:[/!a-zA-Z])/;
const RE_INDENTED = /^(?: {4,}|\t)/;

/** A contiguous run of removed lines and where it sat in the NEW file. */
export interface RemovedRun {
  /** 0-based NEW-file line number this run renders BEFORE (may == doc length). */
  anchor: number;
  /** Old-side source lines, diff markers stripped. */
  lines: string[];
}

/** mapHunks' exact per-line attribution of the NEW text. */
export interface LineMap {
  addedLines: Set<number>;
  removedRuns: RemovedRun[];
}

/**
 * Pass 1 (exact): walk the hunks with both counters. Del lines buffer into a
 * pending run which flushes (anchored at the CURRENT new line) when an add or
 * context line follows — i.e. the removal renders immediately before whatever
 * now occupies its position — and at hunk end (anchor = one past the hunk's
 * last new line). Runs flushing at the same anchor (adjacent hunks) merge.
 */
export function mapHunks(hunks: DiffHunk[]): LineMap {
  const addedLines = new Set<number>();
  const removedRuns: RemovedRun[] = [];

  const flush = (anchor: number, pending: string[]) => {
    if (pending.length === 0) return;
    const last = removedRuns[removedRuns.length - 1];
    if (last && last.anchor === anchor) {
      last.lines.push(...pending);
    } else {
      removedRuns.push({ anchor, lines: pending });
    }
  };

  for (const hunk of hunks) {
    const m = RE_HUNK_HEADER.exec(hunk.header);
    if (!m) continue;
    // 1-based starts → 0-based; an omitted count means exactly one line.
    let newLn = Number(m[3]) - 1;
    let pending: string[] = [];

    for (const line of hunk.lines) {
      // Line.text keeps its unified-diff marker byte (parse.go contract).
      const body = line.text.slice(1);
      switch (line.kind) {
        case "context":
          flush(newLn, pending);
          pending = [];
          newLn++;
          break;
        case "add":
          flush(newLn, pending);
          pending = [];
          addedLines.add(newLn);
          newLn++;
          break;
        case "del":
          pending.push(body);
          break;
      }
    }
    flush(newLn, pending); // trailing removals: anchored one past the hunk
  }
  return { addedLines, removedRuns };
}

/** Top-level block types the tokenizer distinguishes. */
export type BlockType =
  | "heading"
  | "code"
  | "table"
  | "list"
  | "quote"
  | "thematic"
  | "html"
  | "refdef"
  | "paragraph";

/** One top-level block: the half-open-exclusive line span [start, end). */
export interface Block {
  type: BlockType;
  /** 0-based line of the block's first line. */
  start: number;
  /** 0-based line AFTER the block's last line (exclusive — matches length). */
  end: number;
  /** The block's source lines joined with "\n". */
  source: string;
}

function isBlank(line: string): boolean {
  return line.trim() === "";
}

/** A line that STARTS a new non-paragraph block (paragraph interrupters). */
function isBlockStart(line: string, next?: string): boolean {
  if (
    RE_FENCE.test(line) ||
    RE_HEADING.test(line) ||
    RE_QUOTE.test(line) ||
    RE_LIST_ITEM.test(line) ||
    RE_THEMATIC_BREAK.test(line) ||
    RE_HTML_BLOCK.test(line)
  ) {
    return true;
  }
  // GFM table trigger: this line has pipes and the NEXT line is a delimiter.
  return (
    next !== undefined &&
    line.includes("|") &&
    RE_TABLE_DELIM.test(next) &&
    next.includes("|")
  );
}

/**
 * Pass 2a: top-level block spans over the NEW text's lines. CommonMark-lite
 * by design — the deliberate approximations (each only affects grouping,
 * never content):
 *   - blockquotes stop at the first non-`>` line (no lazy continuation);
 *   - lists consume following non-blank lines (CommonMark lazy item
 *     continuation) and continue across blanks only when the next non-blank
 *     line is indented or an item — a nested fence stays inside the list;
 *   - indented (4-space) code is grouped, not fence-parsed;
 *   - reference definitions are their own `refdef` blocks (they render to
 *     nothing; the viewer hoists them) — one glued to a paragraph without a
 *     blank line between renders as paragraph text.
 */
export function tokenizeBlocks(lines: string[]): Block[] {
  const blocks: Block[] = [];
  let i = 0;

  const push = (type: BlockType, start: number, end: number) => {
    blocks.push({
      type,
      start,
      end,
      source: lines.slice(start, end).join("\n"),
    });
  };

  while (i < lines.length) {
    const line = lines[i];
    if (isBlank(line)) {
      i++;
      continue;
    }

    // Setext heading: a paragraph-looking line underlined by ===/---.
    const next = lines[i + 1];
    if (
      next !== undefined &&
      !isBlank(line) &&
      !isBlockStart(line, lines[i + 2]) &&
      RE_SETEXT_UNDERLINE.test(next)
    ) {
      push("heading", i, i + 2);
      i += 2;
      continue;
    }

    if (RE_FENCE.test(line)) {
      const marker = line.trimStart().startsWith("```") ? "`" : "~";
      const openLen = (line.trimStart().match(/^[`~]+/)?.[0] ?? "").length;
      // scan for the closing fence (same char, >= open length); an unclosed
      // fence runs to EOF (CommonMark)
      let k = i + 1;
      while (k < lines.length) {
        const t = lines[k].trimStart();
        const run = t.match(new RegExp(`^\\${marker}{3,}`))?.[0] ?? "";
        if (run.length >= openLen && t === run) {
          k++; // include the closing fence
          break;
        }
        k++;
      }
      push("code", i, k);
      i = k;
      continue;
    }

    if (RE_HEADING.test(line)) {
      push("heading", i, i + 1);
      i++;
      continue;
    }

    if (RE_THEMATIC_BREAK.test(line)) {
      push("thematic", i, i + 1);
      i++;
      continue;
    }

    if (RE_QUOTE.test(line)) {
      let j = i + 1;
      while (j < lines.length && RE_QUOTE.test(lines[j])) j++;
      push("quote", i, j);
      i = j;
      continue;
    }

    // GFM table: current line has pipes and the next line is a delimiter row.
    if (
      next !== undefined &&
      line.includes("|") &&
      next.includes("|") &&
      RE_TABLE_DELIM.test(next)
    ) {
      let j = i + 1; // rows: until blank or another block's start
      while (
        j < lines.length &&
        !isBlank(lines[j]) &&
        !isBlockStart(lines[j], lines[j + 1])
      ) {
        j++;
      }
      push("table", i, j);
      i = j;
      continue;
    }

    if (RE_LIST_ITEM.test(line)) {
      let j = i + 1;
      while (j < lines.length) {
        if (isBlank(lines[j])) {
          // Loose-list continuation: blank followed by an indented or item
          // line keeps the list open; otherwise the blank ends it.
          let k = j;
          while (k < lines.length && isBlank(lines[k])) k++;
          if (
            k < lines.length &&
            (RE_LIST_ITEM.test(lines[k]) || RE_INDENTED.test(lines[k]))
          ) {
            j = k;
          } else {
            break;
          }
        } else {
          // Item starts, indented continuations, and lazy (unindented) text
          // continuation lines all belong to the list.
          j++;
        }
      }
      push("list", i, j);
      i = j;
      continue;
    }

    if (RE_HTML_BLOCK.test(line)) {
      let j = i + 1;
      while (j < lines.length && !isBlank(lines[j])) j++;
      push("html", i, j);
      i = j;
      continue;
    }

    if (RE_INDENTED.test(line)) {
      let j = i + 1;
      while (
        j < lines.length &&
        (RE_INDENTED.test(lines[j]) ||
          (isBlank(lines[j]) &&
            lines.slice(j).findIndex((l) => !isBlank(l)) !== -1 &&
            RE_INDENTED.test(
              lines.slice(j).find((l) => !isBlank(l)) ?? "",
            )))
      ) {
        j++;
      }
      push("code", i, j);
      i = j;
      continue;
    }

    // Reference definition: its own invisible block (hoisted by the viewer).
    if (RE_REF_DEF.test(line)) {
      let j = i + 1;
      while (j < lines.length && RE_REF_DEF.test(lines[j])) j++;
      push("refdef", i, j);
      i = j;
      continue;
    }

    // Paragraph: until a blank line or another block's start. A following
    // setext underline is consumed as the paragraph's heading underline.
    let j = i + 1;
    while (
      j < lines.length &&
      !isBlank(lines[j]) &&
      !isBlockStart(lines[j], lines[j + 1])
    ) {
      j++;
      if (
        j < lines.length &&
        RE_SETEXT_UNDERLINE.test(lines[j]) &&
        lines[j].trimStart().startsWith("=")
      ) {
        j++; // setext H1 underline belongs to the paragraph-as-heading
        break;
      }
    }
    push("paragraph", i, j);
    i = j;
  }
  return blocks;
}

/** A rendered item: a NEW-text block, or a removed run ghost-in-place. */
export interface BlockRenderItem {
  kind: "block";
  block: Block;
  /** Any line of the block was an add line → green tint. */
  added: boolean;
  /**
   * Removed runs anchored INSIDE a table/list block — rendered as ghost
   * rows/items by the viewer's sub-block overrides (exact position).
   * Runs anchored inside other block types render as standalone ghost
   * blocks BEFORE the block (the stated approximation).
   */
  ghostRuns: RemovedRun[];
}

export interface GhostRenderItem {
  kind: "ghost";
  run: RemovedRun;
  /** The run's old lines tokenized into blocks (rendered struck-through). */
  blocks: Block[];
}

export type RenderItem = BlockRenderItem | GhostRenderItem;

/** buildMarkdownDiff's output: the renderable document + hoisted link defs. */
export interface MarkdownDiff {
  items: RenderItem[];
  /** Reference-definition source prepended to every block's render source. */
  refDefs: string;
  addedLines: Set<number>;
}

/**
 * Pass 2b: attribute + interleave. Runs are consumed in anchor order; a run
 * anchored at or before a block's first line (and any run anchored inside a
 * non-table/list block) renders as a standalone ghost block immediately
 * before that block; runs anchored inside a table/list block attach to it for
 * exact row/item ghosting. Trailing runs (anchor past the last block, e.g. a
 * deleted file's whole-content run anchored at 0 of an empty new text)
 * render at the end.
 */
export function buildMarkdownDiff(
  newText: string | null,
  hunks: DiffHunk[],
): MarkdownDiff {
  const { addedLines, removedRuns } = mapHunks(hunks);
  const lines = (newText ?? "").split("\n");
  const blocks = tokenizeBlocks(lines);

  const runs = [...removedRuns].sort((a, b) => a.anchor - b.anchor);
  const refDefLines: string[] = [];
  const items: RenderItem[] = [];
  let runIdx = 0;

  const ghostItem = (run: RemovedRun): RenderItem => ({
    kind: "ghost",
    run,
    blocks: tokenizeBlocks(run.lines),
  });

  for (const block of blocks) {
    if (block.type === "refdef") {
      refDefLines.push(block.source);
      continue;
    }
    while (runIdx < runs.length && runs[runIdx].anchor <= block.start) {
      items.push(ghostItem(runs[runIdx]));
      runIdx++;
    }
    const subRenderable = block.type === "table" || block.type === "list";
    const ghostRuns: RemovedRun[] = [];
    while (runIdx < runs.length && runs[runIdx].anchor <= block.end) {
      const run = runs[runIdx];
      if (subRenderable) {
        ghostRuns.push(run); // exact row/item placement by the overrides
      } else {
        items.push(ghostItem(run)); // approximation: before the block
      }
      runIdx++;
    }
    let added = false;
    for (let l = block.start; l < block.end && !added; l++) {
      added = addedLines.has(l);
    }
    items.push({ kind: "block", block, added, ghostRuns });
  }
  while (runIdx < runs.length) {
    items.push(ghostItem(runs[runIdx]));
    runIdx++;
  }

  return { items, refDefs: refDefLines.join("\n"), addedLines };
}

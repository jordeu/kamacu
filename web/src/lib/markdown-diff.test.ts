import { describe, expect, it } from "vitest";
import type { DiffHunk } from "@/api/diffs";
import {
  buildMarkdownDiff,
  mapHunks,
  tokenizeBlocks,
  type Block,
  type RenderItem,
} from "./markdown-diff";

/**
 * Unit tests for the Markdown viewer's two pure passes (MDV-01): the exact
 * hunk→line map, the CommonMark-lite block tokenizer, and their interleaving.
 * The cases marked APPROXIMATION below pin the documented approximations so
 * they are visible here rather than discovered in the UI — if one of these
 * assertions changes, the phase-1 report's exact/approximate table (records/
 * 2026-09-04/kamacu-399-md-viewer.md) must change with it.
 *
 * Line.text keeps its unified-diff marker byte (parse.go contract), so all
 * fixture lines carry their leading +/-/space.
 */

/** Build a hunk from spec strings: " old", "+new", "-gone" (marker first). */
function hunk(header: string, ...lines: string[]): DiffHunk {
  return {
    header,
    lines: lines.map((l) => ({
      kind: l.startsWith("+")
        ? "add"
        : l.startsWith("-")
          ? "del"
          : "context",
      text: l,
    })),
  };
}

/** hunk("@@ -1,3 +1,3 @@", ...) style helper: compute a real header. */
function hunkAt(oldStart: number, newStart: number, ...lines: string[]): DiffHunk {
  let oc = 0;
  let nc = 0;
  for (const l of lines) {
    if (l.startsWith("+")) nc++;
    else if (l.startsWith("-")) oc++;
    else {
      oc++;
      nc++;
    }
  }
  const header = `@@ -${oldStart},${oc} +${newStart},${nc} @@`;
  return hunk(header, ...lines);
}

describe("mapHunks (pass 1 — exact)", () => {
  it("maps added lines and anchors a del run before its replacement", () => {
    // New file lines (0-based): 0 ctx, 1 add, 2 ctx.
    const lm = mapHunks([
      hunkAt(1, 1, " ctx", "-gone", "+new", " tail"),
    ]);
    expect([...lm.addedLines]).toEqual([1]);
    expect(lm.removedRuns).toEqual([{ anchor: 1, lines: ["gone"] }]);
  });

  it("merges git-grouped del,del,add into one run", () => {
    const lm = mapHunks([hunkAt(1, 1, "-a", "-b", "+c")]);
    expect(lm.removedRuns).toEqual([{ anchor: 0, lines: ["a", "b"] }]);
    expect([...lm.addedLines]).toEqual([0]);
  });

  it("anchors trailing dels one past the hunk's last new line", () => {
    // ctx, add, del — the del has no following new line in this hunk.
    const lm = mapHunks([hunkAt(1, 1, " keep", "+born", "-dying")]);
    expect(lm.removedRuns).toEqual([{ anchor: 2, lines: ["dying"] }]);
  });

  it("merges runs from adjacent hunks flushing at the same anchor", () => {
    // Hunk 1 ends with a trailing del (flushed at hunk end, new line 1);
    // hunk 2 opens with a del whose following context is also new line 1 —
    // both removals sat before the same new line, so they merge.
    const lm = mapHunks([
      hunkAt(1, 1, " ctx", "-a"),
      hunkAt(3, 2, "-b", " ctx"),
    ]);
    expect(lm.removedRuns).toEqual([{ anchor: 1, lines: ["a", "b"] }]);
  });

  it("yields nothing for a no-textual-change (hunkless) file", () => {
    expect(mapHunks([])).toEqual({
      addedLines: new Set(),
      removedRuns: [],
    });
  });
});

describe("tokenizeBlocks (pass 2 — CommonMark-lite)", () => {
  const types = (blocks: Block[]) => blocks.map((b) => b.type);
  const spans = (blocks: Block[]) => blocks.map((b) => [b.start, b.end]);

  it("splits headings, paragraphs, fences, tables, lists, quotes", () => {
    const lines = [
      "# Title",
      "",
      "Para one.",
      "",
      "```",
      "code here",
      "```",
      "",
      "| a | b |",
      "| --- | --- |",
      "| 1 | 2 |",
      "",
      "> quoted",
      "",
      "---",
      "",
      "Final para.",
    ];
    expect(types(tokenizeBlocks(lines))).toEqual([
      "heading",
      "paragraph",
      "code",
      "table",
      "quote",
      "thematic",
      "paragraph",
    ]);
    expect(spans(tokenizeBlocks(lines))).toEqual([
      [0, 1],
      [2, 3],
      [4, 7],
      [8, 11],
      [12, 13],
      [14, 15],
      [16, 17],
    ]);
  });

  it("consumes an unclosed fence to EOF", () => {
    const blocks = tokenizeBlocks(["before", "", "```", "never closed"]);
    expect(blocks[1]).toMatchObject({ type: "code", start: 2, end: 4 });
  });

  it("treats a bare --- after a blank as a thematic break, not a table delimiter or setext", () => {
    const blocks = tokenizeBlocks(["text", "", "---", "", "more"]);
    expect(types(blocks)).toEqual(["paragraph", "thematic", "paragraph"]);
  });

  it("recognizes a setext heading (text + === underline)", () => {
    const blocks = tokenizeBlocks(["Title text", "===", "", "next"]);
    expect(blocks[0]).toMatchObject({ type: "heading", start: 0, end: 2 });
  });

  it("keeps list continuation and loose blank lines inside the list", () => {
    const blocks = tokenizeBlocks([
      "- item one",
      "  continued",
      "",
      "- item two",
      "",
      "outside",
    ]);
    expect(blocks[0]).toMatchObject({ type: "list", start: 0, end: 4 });
    expect(blocks[1]).toMatchObject({ type: "paragraph", start: 5, end: 6 });
  });

  it("collects reference definitions as invisible refdef blocks", () => {
    const blocks = tokenizeBlocks(["[ref]: https://x.example", "", "Uses [ref]."]);
    expect(types(blocks)).toEqual(["refdef", "paragraph"]);
  });

  it("groups indented (4-space) lines as a code block", () => {
    const blocks = tokenizeBlocks(["    indented code", "    line two"]);
    expect(types(blocks)).toEqual(["code"]);
  });
});

describe("buildMarkdownDiff (interleaving)", () => {
  const plain = (items: RenderItem[]) =>
    items
      .filter((i) => i.kind === "block")
      .map((i) => (i as { block: Block }).block);

  it("APPROXIMATION: a mid-paragraph deletion with no add renders the ghost BEFORE the block", () => {
    // New text: one 3-line paragraph; the del removed a line that (in the
    // old file) sat inside it, with no replacement add line.
    const newText = "alpha\nbeta\ngamma\n";
    const hunks = [hunkAt(1, 1, " alpha", "-beta", " gamma")];
    const { items } = buildMarkdownDiff(newText, hunks);
    // The ghost cannot render inside the paragraph: it comes first.
    expect(items[0]?.kind).toBe("ghost");
    expect(items[1]).toMatchObject({ kind: "block", added: false });
    expect(plain(items)[0]?.source).toBe("alpha\nbeta\ngamma");
  });

  it("renders a rewritten paragraph as ghost-old then added-new", () => {
    const newText = "intro kept\nrewritten\n";
    const hunks = [hunkAt(1, 1, " intro kept", "-old words", "+rewritten")];
    const { items } = buildMarkdownDiff(newText, hunks);
    expect(items[0]?.kind).toBe("ghost");
    expect(items[1]).toMatchObject({ kind: "block", added: true });
  });

  it("APPROXIMATION: a changed fence tints the whole code block and ghosts the old fence before it", () => {
    const newText = "```\nnew code\n```\n";
    const hunks = [hunkAt(1, 1, "-```", "-old code", "-```", "+```", "+new code", "+```")];
    const { items } = buildMarkdownDiff(newText, hunks);
    expect(items[0]?.kind).toBe("ghost");
    const ghost = items[0] as Extract<RenderItem, { kind: "ghost" }>;
    expect(ghost.blocks.map((b) => b.type)).toEqual(["code"]);
    expect(items[1]).toMatchObject({ kind: "block", added: true });
  });

  it("attaches mid-list removals to the list block for exact item ghosting", () => {
    const newText = "1. first\n2. second\n3. third\n";
    const hunks = [hunkAt(1, 1, " 1. first", "-2. old second", "+2. second", " 3. third")];
    const { items } = buildMarkdownDiff(newText, hunks);
    const list = items[0] as Extract<RenderItem, { kind: "block" }>;
    expect(list.block.type).toBe("list");
    expect(list.ghostRuns).toEqual([{ anchor: 1, lines: ["2. old second"] }]);
  });

  it("attaches mid-table removals to the table block for exact row ghosting", () => {
    const newText = "| a |\n| --- |\n| 1 |\n| 2 |\n";
    const hunks = [hunkAt(1, 1, " | a |", " | --- |", " | 1 |", "-| gone |", " | 2 |")];
    const { items } = buildMarkdownDiff(newText, hunks);
    const table = items[0] as Extract<RenderItem, { kind: "block" }>;
    expect(table.block.type).toBe("table");
    expect(table.ghostRuns).toEqual([{ anchor: 3, lines: ["| gone |"] }]);
  });

  it("added outright: every block is added, no ghosts", () => {
    const newText = "# New\n\nbody\n";
    const hunks = [hunkAt(0, 1, "+# New", "+", "+body")];
    const { items } = buildMarkdownDiff(newText, hunks);
    expect(items.every((i) => i.kind === "block" && i.added)).toBe(true);
    expect(items).toHaveLength(2);
  });

  it("deleted outright: only ghost items, anchored at 0 of the empty new text", () => {
    const hunks = [hunkAt(1, 0, "-# Gone", "-", "-body")];
    const { items } = buildMarkdownDiff(null, hunks);
    expect(items.every((i) => i.kind === "ghost")).toBe(true);
    const ghost = items[0] as Extract<RenderItem, { kind: "ghost" }>;
    expect(ghost.run.anchor).toBe(0);
    expect(ghost.blocks.map((b) => b.type)).toEqual(["heading", "paragraph"]);
  });

  it("no textual change: everything plain", () => {
    const newText = "# Same\n\nbody\n";
    const { items } = buildMarkdownDiff(newText, []);
    expect(items.map((i) => i.kind)).toEqual(["block", "block"]);
    expect(items.every((i) => i.kind === "block" && !i.added)).toBe(true);
  });

  it("hoists reference definitions out of the render items", () => {
    const newText = "[ref]: https://x.example\n\nUses [ref].\n";
    const hunks = [hunkAt(3, 3, "+new tail")];
    const { items, refDefs } = buildMarkdownDiff(newText, hunks);
    expect(refDefs).toBe("[ref]: https://x.example");
    // The refdef block itself renders nothing and is not among the items…
    expect(
      items.filter((i) => i.kind === "block" && (i as { block: Block }).block.type === "refdef"),
    ).toHaveLength(0);
    // …while the paragraph carries the added line.
    expect(items.some((i) => i.kind === "block" && (i as { added: boolean }).added)).toBe(true);
  });
});

import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { DiffHunk } from "@/api/diffs";
import { UnifiedDiffBody } from "./UnifiedDiffBody";

/**
 * Render test for the marker-byte contract: Line.text keeps its leading
 * +/-/space byte (internal/diff/parse.go — pinned by Go tests and folded into
 * the Viewed hash), while UnifiedDiffBody draws its own +/- gutter span. The
 * rendered line text must therefore strip the byte or every changed line
 * renders doubled markers ("++added", "−-removed"). Rendered with
 * react-dom/server so no DOM test environment is needed.
 */
const hunks: DiffHunk[] = [
  {
    header: "@@ -1,3 +1,3 @@",
    lines: [
      { kind: "context", text: " always been" },
      { kind: "del", text: "-old line" },
      { kind: "add", text: "+new line" },
    ],
  },
];

describe("UnifiedDiffBody", () => {
  it("renders the gutter marker and strips the marker byte from line text", () => {
    const html = renderToStaticMarkup(<UnifiedDiffBody hunks={hunks} />);
    expect(html).toContain("@@ -1,3 +1,3 @@");
    expect(html).toContain(">+</span>");
    expect(html).toContain(">−</span>");
    expect(html).toContain(">new line</span>");
    expect(html).toContain(">old line</span>");
    expect(html).toContain(">always been</span>");
    expect(html).not.toContain("++");
    expect(html).not.toContain("−-");
  });
});

import { describe, expect, it } from "vitest";
import favicon from "../../public/favicon.svg?raw";
import { inboxFavicon } from "./inboxFavicon";

describe("inbox favicon", () => {
  it("uses the transparent brand icon when empty", () => {
    expect(inboxFavicon(0)).toBe("/favicon.svg");
    expect(favicon).not.toContain("<rect");
  });

  it.each([[1, 24], [9, 24], [10, 22], [99, 22], [100, 17], [1000, 17]])(
    "keeps count %i readable in the tab",
    (count, fontSize) => {
      const svg = decodeURIComponent(inboxFavicon(count));
      const label = count > 99 ? "99+" : String(count);
      expect(svg).toContain(`>${label}</text>`);
      expect(svg).toContain(`font-size="${fontSize}"`);
      expect(svg).toContain('fill="#4ade80"');
      expect(svg).toContain('fill="#052e16"');
      expect(svg).toContain('height="22"');
      expect(svg).toContain('aria-label="Kamacu: ' + label + ' pending messages"');
    },
  );
});

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { CustomPromptsSection } from "./CustomPromptsSection";

vi.mock("@/api/settings", () => ({ useSaveSetting: () => ({ isPending: false }) }));

describe("custom prompt settings", () => {
  it("explains how to add and insert prompts when none are configured", () => {
    const html = renderToStaticMarkup(<CustomPromptsSection settings={{}} />);
    expect(html).toContain("No custom prompts yet.");
    expect(html).toContain("Add prompt");
    expect(html).toContain("Agent actions menu");
  });

  it("shows saved titles, text previews, and editing controls", () => {
    const html = renderToStaticMarkup(<CustomPromptsSection settings={{
      custom_prompts: { value: JSON.stringify([{ title: "Explain changes", text: "Explain the changes.\nInclude examples." }]), default: "[]" },
    }} />);
    expect(html).toContain("Explain changes");
    expect(html).toContain("Explain the changes.\nInclude examples.");
    expect(html).toContain('aria-label="Edit Explain changes"');
    expect(html).toContain('aria-label="Delete Explain changes"');
    expect(html).not.toContain("No custom prompts yet.");
  });
});

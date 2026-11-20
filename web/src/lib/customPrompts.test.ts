import { describe, expect, it } from "vitest";
import { applyCustomPromptDraft, parseCustomPrompts, type CustomPromptDraft } from "./customPrompts";

const original = [{ title: "First", text: "First prompt" }, { title: "Second", text: "Second prompt" }];
const source = JSON.stringify(original);
const draft: CustomPromptDraft = { index: 0, source, prompt: { title: " Updated ", text: "  Keep this formatting.\n" } };

describe("editing custom prompts", () => {
  it("updates only the selected prompt and preserves its literal text", () => {
    expect(applyCustomPromptDraft(source, draft)).toEqual([
      { title: "Updated", text: "  Keep this formatting.\n" }, original[1],
    ]);
  });

  it("refuses to overwrite another prompt after a refresh removes the edited row", () => {
    const refreshed = JSON.stringify([original[1]]);
    expect(() => applyCustomPromptDraft(refreshed, draft)).toThrow("Custom prompts changed");
    expect(parseCustomPrompts(refreshed)).toEqual([original[1]]);
    expect(draft.prompt.text).toBe("  Keep this formatting.\n");
  });

  it("refuses to append from a stale draft that would overwrite new prompts", () => {
    expect(() => applyCustomPromptDraft(source, { ...draft, index: -1, source: "[]" })).toThrow("Custom prompts changed");
  });

  it("adds the first prompt when the setting has not been returned yet", () => {
    expect(applyCustomPromptDraft(undefined, { ...draft, index: -1, source: "[]" })).toEqual([
      { title: "Updated", text: "  Keep this formatting.\n" },
    ]);
  });
});

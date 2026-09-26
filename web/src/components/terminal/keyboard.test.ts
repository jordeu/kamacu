import { describe, expect, it } from "vitest";
import { isBrowserSwitchShortcut } from "./keyboard";

describe("browser switching shortcuts", () => {
  it.each(["ctrlKey", "altKey", "metaKey"] as const)("keeps %s+Tab out of terminal stdin", (modifier) => {
    expect(isBrowserSwitchShortcut({ key: "Tab", ctrlKey: false, altKey: false, metaKey: false, [modifier]: true })).toBe(true);
  });

  it.each(["PageUp", "PageDown"])("keeps Ctrl+%s out of terminal stdin", (key) => {
    expect(isBrowserSwitchShortcut({ key, ctrlKey: true, altKey: false, metaKey: false })).toBe(true);
  });

  it.each(["Tab", "Enter", "Escape", "ArrowUp", "y"])("preserves ordinary %s input", (key) => {
    expect(isBrowserSwitchShortcut({ key, ctrlKey: false, altKey: false, metaKey: false })).toBe(false);
  });

  it("preserves terminal Ctrl+C", () => {
    expect(isBrowserSwitchShortcut({ key: "c", ctrlKey: true, altKey: false, metaKey: false })).toBe(false);
  });
});

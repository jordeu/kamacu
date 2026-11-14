import { describe, expect, it } from "vitest";
import { cursorKey, helperKeyRows } from "./helperKeys";

describe("terminal helper input", () => {
  it.each([
    ["A", "\x1b[A", "\x1bOA"],
    ["B", "\x1b[B", "\x1bOB"],
    ["C", "\x1b[C", "\x1bOC"],
    ["D", "\x1b[D", "\x1bOD"],
    ["H", "\x1b[H", "\x1bOH"],
    ["F", "\x1b[F", "\x1bOF"],
  ] as const)("encodes %s for shells and application cursor mode", (key, normal, application) => {
    expect(cursorKey(key, false)).toBe(normal);
    expect(cursorKey(key, true)).toBe(application);
  });

  it("sends control bytes for Ctrl shortcuts rather than printable labels", () => {
    for (const row of helperKeyRows) {
      for (const key of row) {
        if ("data" in key && key.label.startsWith("Ctrl+")) {
          expect(key.data).toBe(String.fromCharCode(key.label.charCodeAt(5) - 64));
        }
      }
    }
  });

  it("uses terminal escape sequences for keys absent from mobile keyboards", () => {
    const keys = new Map<string, string>(helperKeyRows.flatMap((row) => row.flatMap((key) => "data" in key ? [[key.name, key.data] as [string, string]] : [])));
    expect(keys.get("Escape")).toBe("\x1b");
    expect(keys.get("Tab")).toBe("\t");
    expect(keys.get("Shift+Tab")).toBe("\x1b[Z");
    expect(keys.get("Enter")).toBe("\r");
    expect(keys.get("Backspace")).toBe("\x7f");
    expect(keys.get("Delete")).toBe("\x1b[3~");
    expect(keys.get("Page up")).toBe("\x1b[5~");
    expect(keys.get("Page down")).toBe("\x1b[6~");
  });
});

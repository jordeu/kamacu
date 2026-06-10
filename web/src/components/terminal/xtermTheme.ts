import type { ITheme } from "@xterm/xterm";

// Exact values from 02-UI-SPEC.md "xterm Theme" + ANSI 16 tables.
// This module carries to Phases 3-5 verbatim — do not tweak.
export const zincTheme: ITheme = {
  background: "#09090b", // zinc-950
  foreground: "#fafafa", // zinc-50
  cursor: "#fafafa",
  cursorAccent: "#09090b",
  selectionBackground: "#3f3f46", // zinc-700
  // selectionForeground deliberately unset — preserve cell colors under selection
  black: "#18181b",
  brightBlack: "#52525b",
  red: "#f87171",
  brightRed: "#fca5a5",
  green: "#4ade80",
  brightGreen: "#86efac",
  yellow: "#facc15",
  brightYellow: "#fde047",
  blue: "#60a5fa",
  brightBlue: "#93c5fd",
  magenta: "#c084fc",
  brightMagenta: "#d8b4fe",
  cyan: "#22d3ee",
  brightCyan: "#67e8f9",
  white: "#e4e4e7",
  brightWhite: "#fafafa",
};

// Literal stack: a CSS var() does NOT resolve inside the xterm renderer (D-19)
export const terminalFontFamily =
  'ui-monospace, "SF Mono", "Cascadia Mono", Menlo, Consolas, monospace';
export const terminalFontSize = 13; // D-19 / UI-SPEC: 13 chosen for grid density

// Cursor keys must follow the mode selected by the foreground TUI (e.g. vim).
export function cursorKey(direction: "A" | "B" | "C" | "D" | "H" | "F", applicationMode: boolean): string {
  return `\x1b${applicationMode ? "O" : "["}${direction}`;
}

export const helperKeyRows = [
  [
    { label: "Esc", name: "Escape", data: "\x1b" },
    { label: "Tab", name: "Tab", data: "\t" },
    { label: "←", name: "Arrow left", cursor: "D" },
    { label: "↓", name: "Arrow down", cursor: "B" },
    { label: "↑", name: "Arrow up", cursor: "A" },
    { label: "→", name: "Arrow right", cursor: "C" },
    { label: "Enter", name: "Enter", data: "\r" },
    { label: "⇧ Tab", name: "Shift+Tab", data: "\x1b[Z" },
    { label: "⌫", name: "Backspace", data: "\x7f" },
  ],
  [
    { label: "Ctrl+C", name: "Ctrl+C: interrupt", data: "\x03" },
    { label: "Ctrl+D", name: "Ctrl+D: end of input", data: "\x04" },
    { label: "Ctrl+Z", name: "Ctrl+Z: suspend", data: "\x1a" },
    { label: "Ctrl+L", name: "Ctrl+L: clear screen", data: "\x0c" },
    { label: "Ctrl+R", name: "Ctrl+R: search history", data: "\x12" },
    { label: "Ctrl+B", name: "Ctrl+B: tmux prefix", data: "\x02" },
    { label: "Ctrl+A", name: "Ctrl+A: start of line", data: "\x01" },
    { label: "Ctrl+E", name: "Ctrl+E: end of line", data: "\x05" },
    { label: "Ctrl+U", name: "Ctrl+U: delete to start of line", data: "\x15" },
    { label: "Ctrl+W", name: "Ctrl+W: delete previous word", data: "\x17" },
    { label: "Home", name: "Home", cursor: "H" },
    { label: "End", name: "End", cursor: "F" },
    { label: "PgUp", name: "Page up", data: "\x1b[5~" },
    { label: "PgDn", name: "Page down", data: "\x1b[6~" },
    { label: "Del", name: "Delete", data: "\x1b[3~" },
  ],
] as const;

import type { Terminal } from "@xterm/xterm";

/** Match desktop wheel routing for tmux/TUIs, and drag local history 1:1. */
export function scrollTerminal(
  term: Terminal,
  lines: number,
  position: { clientX: number; clientY: number },
): void {
  if (term.modes.mouseTrackingMode === "none" && term.buffer.active.type === "normal") {
    term.scrollLines(lines);
    return;
  }

  // xterm encodes wheel input using the app's active mouse protocol (SGR,
  // legacy, etc.), or cursor keys in an alternate buffer without mouse mode.
  // One event per row: mouse protocols encode direction, not a row count.
  for (let row = 0; row < Math.abs(lines); row++) {
    term.element?.dispatchEvent(new WheelEvent("wheel", {
      bubbles: true,
      cancelable: true,
      deltaMode: WheelEvent.DOM_DELTA_LINE,
      deltaY: Math.sign(lines),
      clientX: position.clientX,
      clientY: position.clientY,
    }));
  }
}

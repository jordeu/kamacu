// Minimal module store for the terminal session the user currently has
// focused. The beacon (mounted once in AppLayout, outside the routed pages)
// reads it on every beat; TaskPage / GlobalTaskPage write it when their tab
// selection changes. Module-level on purpose: a store smaller than a
// context — one string, no subscribers, read at beat time only.

let activeSessionId: string | null = null;
let terminalObscured = false;

/** Report the focused terminal session (null on non-terminal tabs / unmount). */
export function setActiveSession(id: string | null): void {
  activeSessionId = id;
  if (typeof window !== "undefined")
    window.dispatchEvent(new Event("kamacu-terminal-view"));
}

/** The focused terminal session, if any. Read at beat time by the beacon. */
export function getActiveSession(): string | null {
  return terminalObscured ? null : activeSessionId;
}

/** Full-page overlays cover the terminal without changing its selected tab. */
export function setTerminalViewObscured(obscured: boolean): void {
  if (terminalObscured === obscured) return;
  terminalObscured = obscured;
  if (typeof window !== "undefined")
    window.dispatchEvent(new Event("kamacu-terminal-view"));
}

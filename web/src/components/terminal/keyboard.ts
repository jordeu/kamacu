// Returning false from xterm's custom handler leaves browser/OS shortcuts
// alone. In particular, xterm otherwise encodes modified Tab as plain Tab,
// which looks like an answer to a pending agent permission prompt.
export function isBrowserSwitchShortcut(event: Pick<KeyboardEvent, "key" | "ctrlKey" | "altKey" | "metaKey">): boolean {
  return (
    (event.key === "Tab" && (event.ctrlKey || event.altKey || event.metaKey)) ||
    (event.ctrlKey && (event.key === "PageUp" || event.key === "PageDown"))
  );
}

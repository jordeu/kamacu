/** Capture two-finger drags before xterm or the browser handles the gesture. */
export function attachTwoFingerScroll(
  element: HTMLElement,
  scrollLines: (lines: number) => void,
  getLineHeight: () => number,
): () => void {
  let active = false;
  let previous: { ids: number[]; y: number } | null = null;
  let remainder = 0;

  const handleTouch = (event: TouchEvent) => {
    // `touches` includes fingers anywhere on the page. `targetTouches` would
    // exclude fingers on different children of the same terminal.
    const touches = Array.from(event.touches).filter(
      (touch) => element.contains(touch.target as Node),
    );
    if (event.type === "touchstart" && touches.length === 2) active = true;
    if (!active) return;

    // Non-passive capture listeners keep the gesture out of page scrolling,
    // pinch zoom, and xterm's own touch/mouse handling.
    event.preventDefault();
    event.stopImmediatePropagation();

    if (touches.length === 0) {
      active = false;
      previous = null;
      remainder = 0;
      return;
    }
    if (event.type === "touchcancel" || touches.length !== 2) {
      previous = null;
      remainder = 0;
      return;
    }

    const [first, second] = touches;
    const ids = [first.identifier, second.identifier].sort((a, b) => a - b);
    const y = (first.clientY + second.clientY) / 2;
    if (event.type === "touchmove" && previous && ids[0] === previous.ids[0] && ids[1] === previous.ids[1]) {
      // Fingers move with the content. Retain partial rows for slow drags.
      remainder += (previous.y - y) / Math.max(1, getLineHeight());
      const lines = Math.trunc(remainder);
      if (lines !== 0) {
        scrollLines(lines);
        remainder -= lines;
      }
    } else {
      remainder = 0;
    }
    previous = { ids, y };
  };

  const events = ["touchstart", "touchmove", "touchend", "touchcancel"] as const;
  for (const type of events) {
    element.addEventListener(type, handleTouch, { capture: true, passive: false });
  }
  return () => {
    for (const type of events) element.removeEventListener(type, handleTouch, { capture: true });
  };
}

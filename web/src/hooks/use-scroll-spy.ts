import { useEffect, useState, type RefObject } from "react";

// The totals bar is a fixed header OUTSIDE this scroll container (DiffTab), so
// the active-detection band starts at the very top of the scroll pane — where
// per-file headers pin (sticky top-0). No totals-bar offset is needed here.
const BAND_TOP_PX = 0;

/**
 * Scroll-spy for the two-pane diff view (D-05): one IntersectionObserver rooted
 * on the right-pane scroll container reports which changed file is at the top of
 * the viewport as the user scrolls, so the file tree can highlight it.
 *
 * Verified config (RESEARCH §Area 3): `root` MUST be the scroll container
 * (omitting it observes the viewport and the spy never fires per-file); a thin
 * active band is defined by `rootMargin` (top offset 0 — the totals bar is a
 * fixed header outside this scroll pane; `-70%` bottom keeps it near the top);
 * `threshold: 0`.
 *
 * It observes the `[data-diff-path]` sticky file headers directly. With the band
 * starting at 0, a header pushed above the pin (top < 0) leaves the band, so the
 * smallest-top in-band header is always the one currently pinned at the top — the
 * active file. The observer disconnects on cleanup and re-observes when the file
 * list changes (keyed on `pathListKey`), which is StrictMode-safe (Pitfall 4).
 */
export function useScrollSpy<T extends HTMLElement>(
  scrollRef: RefObject<T | null>,
  pathListKey: string,
): string | null {
  const [activePath, setActivePath] = useState<string | null>(null);

  useEffect(() => {
    const scrollEl = scrollRef.current;
    if (!scrollEl) return;

    // path -> current boundingClientRect.top for every intersecting wrapper.
    const tops = new Map<string, number>();

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const path = (entry.target as HTMLElement).dataset.diffPath;
          if (!path) continue;
          if (entry.isIntersecting) {
            tops.set(path, entry.boundingClientRect.top);
          } else {
            tops.delete(path);
          }
        }
        // Active = the topmost (smallest top) wrapper currently in the band.
        let topPath: string | null = null;
        let topY = Infinity;
        for (const [path, y] of tops) {
          if (y < topY) {
            topY = y;
            topPath = path;
          }
        }
        if (topPath) setActivePath(topPath);
      },
      {
        root: scrollEl,
        rootMargin: `-${BAND_TOP_PX}px 0px -70% 0px`,
        threshold: 0,
      },
    );

    scrollEl.querySelectorAll("[data-diff-path]").forEach((el) => {
      observer.observe(el);
    });

    return () => observer.disconnect();
    // pathListKey (joined path string) rebuilds the observer when the file list
    // changes; scrollRef is stable. exhaustive-deps satisfied.
  }, [scrollRef, pathListKey]);

  return activePath;
}

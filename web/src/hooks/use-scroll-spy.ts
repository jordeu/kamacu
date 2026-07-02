import { useEffect, useState, type RefObject } from "react";

// Sticky totals-bar height (≈ py-2 + text-sm + border). Kept consistent with
// the TOTALS_BAR_PX value used by DiffFileSection (scroll-mt / sticky top) and
// DiffTab so the active-detection band sits just under the totals bar.
const TOTALS_BAR_PX = 41;

/**
 * Scroll-spy for the two-pane diff view (D-05): one IntersectionObserver rooted
 * on the right-pane scroll container reports which changed file is at the top of
 * the viewport as the user scrolls, so the file tree can highlight it.
 *
 * Verified config (RESEARCH §Area 3): `root` MUST be the scroll container
 * (omitting it observes the viewport and the spy never fires per-file); a thin
 * active band is defined by `rootMargin` (`-41px` top offset pushes it below the
 * sticky totals bar, `-70%` bottom keeps it near the top); `threshold: 0`.
 *
 * It observes the non-sticky `[data-diff-path]` wrappers — NOT the sticky file
 * headers, whose pinned rects would confuse the topmost calc (Pitfall 3). The
 * observer disconnects on cleanup and the effect re-observes when the file list
 * changes (keyed on `pathListKey`), which is StrictMode-safe (Pitfall 4).
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
        rootMargin: `-${TOTALS_BAR_PX}px 0px -70% 0px`,
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

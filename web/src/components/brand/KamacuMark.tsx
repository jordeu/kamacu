import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * KamacuMark — the Kamacu brand mark: an abstract warm-ember spark.
 *
 * ## The naming story — the "why" behind the shape (D-02)
 * "Kamacu" reads as the Quechua *kamaq / kamacu* — "the one who animates, the
 * giver of life-force." Kamacu the product animates your parallel Claude Code
 * agents; this mark is that animating force made visible — a single glowing
 * ember spark rising with motion. It is deliberately an ABSTRACT elemental
 * spark / wisp / flame, NOT a wordmark, NOT a 'K' monogram, and NOT a literal
 * animal or a face (there are no eyes and no character) — D-01.
 *
 * ## Multi-tone ember, NOT the amber "waiting" dot (D-03 / D-04)
 * The spark is a multi-tone warm glow: a soft radial ember halo behind an
 * orange→amber gradient flame body with rising ember particles. It reads as a
 * fuller, shaped, glowing form so it stays clearly DISTINCT from the flat,
 * single-hue `bg-amber-400` "agent waiting" status dot (see StatusDot.tsx:20
 * and the ProjectAvatar waiting overlay). A plain amber circle *is* that
 * waiting dot — this mark is intentionally a gradient, multi-tone silhouette
 * instead. Do a side-by-side mental check against the waiting dot: they must
 * never be confusable. The shipped asset is STATIC — animated logo variants are
 * deferred (BRAND-FUT-01), so there is no `animate-*` anywhere here.
 *
 * ## Sizes (D-05 / D-08)
 * Authored on a 24×24 grid and tuned to read cleanly small: it doubles as the
 * favicon basis (web/public/favicon.svg) and sits at the top of the ~3rem
 * collapsed sidebar rail. Default size is `size-5` (20px); callers resize by
 * passing `className` (e.g. `size-6`, `size-8`) — cn() lets the caller override.
 *
 * Contract consumed by the 20-03 sidebar lockup:
 *   interface KamacuMarkProps extends React.ComponentProps<"svg">
 *   function KamacuMark({ className, ...props }): JSX.Element
 */
interface KamacuMarkProps extends React.ComponentProps<"svg"> {
  className?: string;
}

function KamacuMark({ className, ...props }: KamacuMarkProps) {
  // Unique gradient ids per instance so multiple marks on one page never
  // collide when they share the DOM. Colons from useId() are stripped for
  // maximal SVG fragment-reference compatibility.
  const uid = React.useId().replace(/:/g, "");
  const glowId = `kamacu-glow-${uid}`;
  const bodyId = `kamacu-body-${uid}`;

  return (
    <svg
      viewBox="0 0 24 24"
      role="img"
      aria-label="Kamacu"
      className={cn("size-5", className)}
      {...props}
    >
      <defs>
        {/* Soft ember halo — the "glow" (D-03), fading fully transparent so it
            reads on any background and never becomes a hard-edged disc. */}
        <radialGradient id={glowId} cx="50%" cy="55%" r="55%">
          <stop offset="0%" stopColor="#fb923c" stopOpacity="0.55" />
          <stop offset="55%" stopColor="#f97316" stopOpacity="0.22" />
          <stop offset="100%" stopColor="#f97316" stopOpacity="0" />
        </radialGradient>
        {/* Ember body — deep orange core → bright amber tip (multi-tone, D-04),
            so it is unmistakably not a flat single-hue amber dot. */}
        <linearGradient
          id={bodyId}
          x1="12"
          y1="18"
          x2="12"
          y2="2"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0%" stopColor="#ea580c" />
          <stop offset="45%" stopColor="#f97316" />
          <stop offset="100%" stopColor="#fcd34d" />
        </linearGradient>
      </defs>

      {/* Ember halo behind the spark. */}
      <circle cx="12" cy="11" r="9.5" fill={`url(#${glowId})`} />

      {/* The spark: a rising flame / wisp with an inner lick of motion —
          abstract, no face. */}
      <path
        d="M12 2.5C14 6 16.5 9 16.5 13C16.5 15.5 14.5 17.5 12 17.5C9.5 17.5 7.5 15.5 7.5 13C7.5 10.5 8.8 8.5 10 6.8C10.1 8.6 10.8 9.7 12 10.2C13 9.3 13 6 12 2.5Z"
        fill={`url(#${bodyId})`}
      />

      {/* Rising ember particles — the "motion" of the animating spark. */}
      <circle cx="16.6" cy="6" r="0.95" fill="#fcd34d" />
      <circle cx="7.8" cy="4.4" r="0.7" fill="#fdba74" />
    </svg>
  );
}

export { KamacuMark };

import * as React from "react";

import { cn } from "@/lib/utils";

/** ProjectAvatar — the shared rounded-square monogram primitive for a project's
 *  identity (ICON-07/10). White `font-medium` letters centered on the project's
 *  `icon_color` (a PROJECT_PALETTE hue). This is the ONLY place a palette hex is
 *  applied as a background (D-04). One component, three call sites: the collapsed
 *  rail, the expanded-row inline avatar, and the settings preview.
 *
 *  - size "rail"   → 32px box (`size-8`, `text-sm`) — supports the rail-only
 *    `active` ring (D-06) and `waiting` amber dot (D-07).
 *  - size "inline" → 20px box (`size-5`, `text-[10px]`) — `active`/`waiting` are
 *    ignored (rail-only visuals).
 *
 *  The letter color is ALWAYS white — never derived from `color`; the 9 curated
 *  Tailwind-600 hues meet AA contrast with white at these sizes (Phase 18 D-11
 *  guarantees `color` is a palette member). The waiting dot is STATIC (no pulse
 *  animation), to match the existing expanded count chip (D-07). */
interface ProjectAvatarProps extends React.ComponentProps<"div"> {
  /** rail = 32px (size-8/text-sm), inline = 20px (size-5/text-[10px]) — D-05. */
  size: "rail" | "inline";
  /** project.icon_letters — server-guaranteed non-empty (Phase 18 D-10). */
  letters: string;
  /** project.icon_color — a PROJECT_PALETTE hex, applied as inline backgroundColor. */
  color: string;
  /** Rail-only active ring around the avatar (D-06). */
  active?: boolean;
  /** Rail-only static amber waiting dot overlay (D-07). */
  waiting?: boolean;
  /** aria-label when `waiting`, e.g. "2 agents waiting for input". */
  waitingLabel?: string;
}

function ProjectAvatar({
  size,
  letters,
  color,
  active = false,
  waiting = false,
  waitingLabel,
  className,
  ...props
}: ProjectAvatarProps) {
  const isRail = size === "rail";
  // active/waiting are rail-only visuals (inline ignores them per D-06/D-07).
  const showActive = isRail && active;
  const showWaiting = isRail && waiting;

  return (
    <div
      data-slot="project-avatar"
      aria-label={showWaiting ? waitingLabel : undefined}
      style={{ backgroundColor: color }}
      className={cn(
        "flex shrink-0 items-center justify-center rounded-md font-medium leading-none text-white",
        isRail ? "size-8 text-sm" : "size-5 text-[10px]",
        showActive &&
          "ring-2 ring-sidebar-ring ring-offset-2 ring-offset-sidebar",
        showWaiting && "relative",
        className,
      )}
      {...props}
    >
      {letters}
      {showWaiting ? (
        <span
          aria-hidden="true"
          className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-amber-400"
        />
      ) : null}
    </div>
  );
}

export { ProjectAvatar };

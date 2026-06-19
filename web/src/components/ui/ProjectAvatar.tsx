import * as React from "react";

import { cn } from "@/lib/utils";

/** ProjectAvatar — the shared circular monogram primitive for a project's
 *  identity (ICON-07/10). White `font-medium` letters centered on the project's
 *  `icon_color` (a PROJECT_PALETTE hue). This is the ONLY place a palette hex is
 *  applied as a background (D-04). One component, three call sites: the collapsed
 *  rail, the expanded-row inline avatar, and the settings preview.
 *
 *  - size "rail"   → 24px circle (`size-6`, `text-xs`) — sits with breathing room
 *    inside the 3rem icon rail; supports the rail-only `waiting` amber dot (D-07).
 *  - size "inline" → 20px circle (`size-5`, `text-[10px]`) — `waiting` is ignored
 *    (rail-only visual).
 *
 *  Shape is a full circle (`rounded-full`) per Phase 19 UAT (the original
 *  rounded-square read as too boxy). Active/selected state is NOT drawn on the
 *  avatar itself — the enclosing rail `SidebarMenuButton` carries the same
 *  `bg-sidebar-accent` highlight the expanded rows use (Phase 19 UAT: the earlier
 *  white avatar ring read as ugly and inconsistent with the expanded state). The
 *  letter color is ALWAYS white — never derived from `color`; the 9 curated muted
 *  hues meet AA contrast (>=4.5:1) with white at these sizes (Phase 18 D-11
 *  guarantees `color` is a palette member). The waiting dot is STATIC (no pulse
 *  animation), to match the existing expanded count chip (D-07). */
interface ProjectAvatarProps extends React.ComponentProps<"div"> {
  /** rail = 24px (size-6/text-xs), inline = 20px (size-5/text-[10px]) — D-05. */
  size: "rail" | "inline";
  /** project.icon_letters — server-guaranteed non-empty (Phase 18 D-10). */
  letters: string;
  /** project.icon_color — a PROJECT_PALETTE hex, applied as inline backgroundColor. */
  color: string;
  /** Rail-only static amber waiting dot overlay (D-07). */
  waiting?: boolean;
  /** aria-label when `waiting`, e.g. "2 agents waiting for input". */
  waitingLabel?: string;
}

function ProjectAvatar({
  size,
  letters,
  color,
  waiting = false,
  waitingLabel,
  className,
  ...props
}: ProjectAvatarProps) {
  const isRail = size === "rail";
  // The waiting dot is a rail-only visual (inline ignores it per D-07).
  const showWaiting = isRail && waiting;

  return (
    <div
      data-slot="project-avatar"
      aria-label={showWaiting ? waitingLabel : undefined}
      style={{ backgroundColor: color }}
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full font-medium leading-none text-white",
        isRail ? "size-6 text-xs" : "size-5 text-[10px]",
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

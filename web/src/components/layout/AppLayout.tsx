import { useState, type CSSProperties } from "react";
import { Outlet } from "react-router";
import {
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProjectSidebar } from "@/components/sidebar/ProjectSidebar";
import { ActiveSessionsBar } from "@/components/layout/ActiveSessionsBar";

const SIDEBAR_STORAGE_KEY = "kangent.sidebar";

/**
 * Reopen affordance shown only while the sidebar is collapsed — the offcanvas
 * sidebar hides its own header trigger when closed.
 */
function CollapsedSidebarTrigger() {
  const { open } = useSidebar();
  if (open) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <SidebarTrigger
          aria-label="Toggle sidebar"
          className="absolute top-2 left-2 z-20"
        />
      </TooltipTrigger>
      <TooltipContent side="right">Toggle sidebar (Ctrl+B)</TooltipContent>
    </Tooltip>
  );
}

export function AppLayout() {
  // D-04: collapsed state persists across reloads. The generated shadcn
  // SidebarProvider only writes a cookie (never reads it back in an SPA),
  // so the open state is controlled here and backed by localStorage.
  const [open, setOpen] = useState(
    () => localStorage.getItem(SIDEBAR_STORAGE_KEY) !== "false",
  );

  function handleOpenChange(next: boolean) {
    setOpen(next);
    localStorage.setItem(SIDEBAR_STORAGE_KEY, String(next));
  }

  return (
    <TooltipProvider delayDuration={0}>
      <SidebarProvider
        open={open}
        onOpenChange={handleOpenChange}
        style={{ "--sidebar-width": "15rem" } as CSSProperties}
      >
        <ProjectSidebar />
        <main
          className={
            open
              ? "relative flex-1 overflow-hidden"
              : "relative flex-1 overflow-hidden pl-9"
          }
        >
          <CollapsedSidebarTrigger />
          <Outlet />
        </main>
        {/* D-13: mounted once outside <main>/<Outlet/> so the bar is present on
            every route. It is `fixed inset-x-0 bottom-0` (overlay) — a single
            mount renders it over every route without changing <main>'s layout
            or height, so the xterm terminals never reflow (D-03). */}
        <ActiveSessionsBar />
      </SidebarProvider>
    </TooltipProvider>
  );
}

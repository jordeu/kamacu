import { useState, type CSSProperties } from "react";
import { Outlet } from "react-router";
import { SidebarProvider } from "@/components/ui/sidebar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ProjectSidebar } from "@/components/sidebar/ProjectSidebar";
import { ActiveSessionsBar } from "@/components/layout/ActiveSessionsBar";

const SIDEBAR_STORAGE_KEY = "kangent.sidebar";

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
        {/* The collapsed sidebar is now a 3rem icon rail (collapsible="icon")
            that occupies layout space, so <main> needs no collapsed padding and
            the floating re-expand trigger is retired — the rail's header
            SidebarTrigger is the re-expand entry point (D-02). */}
        <main className="relative flex-1 overflow-hidden">
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

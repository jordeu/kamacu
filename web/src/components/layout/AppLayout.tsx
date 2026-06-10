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
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProjectSidebar } from "@/components/sidebar/ProjectSidebar";

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
    <SidebarProvider
      open={open}
      onOpenChange={handleOpenChange}
      style={{ "--sidebar-width": "15rem" } as CSSProperties}
    >
      <ProjectSidebar />
      <main className="relative flex-1 overflow-hidden">
        <CollapsedSidebarTrigger />
        <Outlet />
      </main>
    </SidebarProvider>
  );
}

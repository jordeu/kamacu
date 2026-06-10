import { Outlet } from "react-router";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";

export function AppLayout() {
  return (
    <SidebarProvider>
      <Sidebar>
        <SidebarHeader>
          <span className="px-2 py-1 font-medium">kangent</span>
        </SidebarHeader>
        {/* Plan 01-04 replaces this content with the project list. */}
        <SidebarContent />
      </Sidebar>
      <main className="flex-1 overflow-hidden">
        <SidebarTrigger />
        <Outlet />
      </main>
    </SidebarProvider>
  );
}

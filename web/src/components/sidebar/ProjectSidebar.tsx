import { useState } from "react";
import { Link, useLocation, useParams } from "react-router";
import { Plus, Settings } from "lucide-react";
import { useProjects } from "@/api/queries";
import { useAgentStatuses } from "@/api/agents";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProjectAvatar } from "@/components/ui/ProjectAvatar";
import { KamacuMark } from "@/components/brand/KamacuMark";
import { AddProjectDialog } from "@/components/sidebar/AddProjectDialog";
import { ProjectMenu } from "@/components/sidebar/ProjectMenu";

export function ProjectSidebar() {
  const { data: projects } = useProjects();
  const { data: agentStatuses } = useAgentStatuses();
  const { projectId } = useParams();
  const location = useLocation();
  const [addOpen, setAddOpen] = useState(false);

  // D-49: count agents (not tasks-with-sessions) waiting for input, per project.
  const waitingByProject = new Map<number, number>();
  for (const entry of agentStatuses ?? []) {
    if (entry.status === "waiting") {
      waitingByProject.set(
        entry.projectId,
        (waitingByProject.get(entry.projectId) ?? 0) + 1,
      );
    }
  }

  return (
    <Sidebar collapsible="icon" className="pb-9">
      <SidebarHeader className="flex-row items-center justify-between group-data-[collapsible=icon]:justify-center">
        {/* Brand lockup (D-07/D-09; D-08 revised at 20-04 UAT): the KamacuMark
            ember spark + "Kamacu" wordmark are EXPANDED-ONLY. When the sidebar is
            collapsed to the icon rail the whole lockup is hidden so the mark can't
            misalign with the toggle button — the rail then shows only the toggle. */}
        <span className="flex items-center gap-2 px-1 group-data-[collapsible=icon]:hidden">
          <KamacuMark className="size-5 shrink-0" />
          <span className="text-sm font-medium group-data-[collapsible=icon]:hidden">
            Kamacu
          </span>
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <SidebarTrigger aria-label="Toggle sidebar" />
          </TooltipTrigger>
          <TooltipContent side="right">Toggle sidebar (Ctrl+B)</TooltipContent>
        </Tooltip>
      </SidebarHeader>

      <SidebarContent>
        <SidebarMenu className="gap-1 px-2 group-data-[collapsible=icon]:items-center group-data-[collapsible=icon]:gap-1 group-data-[collapsible=icon]:px-1">
          {(projects ?? []).map((project) => {
            const count = waitingByProject.get(project.id) ?? 0;
            const isActive = projectId === String(project.id);
            // Reuse the EXACT existing chip copy for the avatar a11y label.
            const waitingLabel =
              count === 1
                ? "1 agent waiting for input"
                : `${count} agents waiting for input`;
            // The Link name source is its aria-label (tooltip is supplementary);
            // append the waiting copy when present so the rail/row announces both.
            const linkLabel =
              count > 0 ? `${project.name}, ${waitingLabel}` : project.name;
            return (
              <SidebarMenuItem key={project.id}>
                <SidebarMenuButton
                  asChild
                  size="sm"
                  isActive={isActive}
                  className="min-h-7 px-3 text-sm group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-md group-data-[collapsible=icon]:p-0!"
                >
                  <Link
                    to={`/projects/${project.id}`}
                    aria-label={linkLabel}
                    aria-current={isActive ? "page" : undefined}
                  >
                    {/* Collapsed rail (ICON-05/06/07/08/09): the size-6 avatar is
                        centered in a 40px rounded-md icon-mode button that fills
                        the rail row. The active project is highlighted by that
                        button's bg-sidebar-accent fill — the SAME filled-row
                        treatment the expanded selection uses, not a ring on the
                        avatar. Hidden when expanded; the side=right tooltip
                        carries the full project name. */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="hidden group-data-[collapsible=icon]:flex">
                          <ProjectAvatar
                            size="rail"
                            letters={project.icon_letters}
                            color={project.icon_color}
                            waiting={count > 0}
                            waitingLabel={waitingLabel}
                          />
                        </span>
                      </TooltipTrigger>
                      <TooltipContent side="right">
                        {project.name}
                      </TooltipContent>
                    </Tooltip>
                    {/* Expanded row (ICON-10): inline avatar before the unchanged
                        name + the existing amber count chip. No dot here (D-08).
                        Hidden in icon mode so only the rail avatar shows. */}
                    <ProjectAvatar
                      size="inline"
                      letters={project.icon_letters}
                      color={project.icon_color}
                      className="group-data-[collapsible=icon]:hidden"
                    />
                    <span className="min-w-0 flex-1 truncate group-data-[collapsible=icon]:hidden">
                      {project.name}
                    </span>
                    {/* UI-SPEC: chip is static (never pulses) and not
                        independently clickable — the row Link is the action. */}
                    {count > 0 && (
                      <span
                        aria-label={waitingLabel}
                        className="ml-auto inline-flex min-w-[18px] shrink-0 items-center justify-center rounded-full bg-amber-400/10 px-1 text-xs font-medium text-amber-400 tabular-nums group-data-[collapsible=icon]:hidden"
                      >
                        {count}
                      </span>
                    )}
                  </Link>
                </SidebarMenuButton>
                <ProjectMenu project={project} />
              </SidebarMenuItem>
            );
          })}
        </SidebarMenu>
      </SidebarContent>

      <SidebarFooter className="flex-row items-center gap-2 group-data-[collapsible=icon]:hidden">
        <Button
          variant="ghost"
          size="sm"
          className="flex-1 justify-start"
          onClick={() => setAddOpen(true)}
        >
          <Plus />
          Add project
        </Button>
        {/* SET-01: gear → dedicated full-page /settings route. Active state
            reuses the selected-sidebar-item treatment from the project rows. */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="ghost"
              size="icon-sm"
              className={cn(
                location.pathname === "/settings" &&
                  "bg-sidebar-accent text-sidebar-accent-foreground",
              )}
            >
              <Link to="/settings" aria-label="Settings">
                <Settings className="size-4" />
              </Link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Settings</TooltipContent>
        </Tooltip>
      </SidebarFooter>

      <AddProjectDialog open={addOpen} onOpenChange={setAddOpen} />
    </Sidebar>
  );
}

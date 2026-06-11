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
    <Sidebar>
      <SidebarHeader className="flex-row items-center justify-between">
        <span className="px-1 text-sm font-medium">kangent</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <SidebarTrigger aria-label="Toggle sidebar" />
          </TooltipTrigger>
          <TooltipContent side="right">Toggle sidebar (Ctrl+B)</TooltipContent>
        </Tooltip>
      </SidebarHeader>

      <SidebarContent>
        <SidebarMenu className="px-2">
          {(projects ?? []).map((project) => {
            const count = waitingByProject.get(project.id) ?? 0;
            return (
              <SidebarMenuItem key={project.id}>
                <SidebarMenuButton
                  asChild
                  size="sm"
                  isActive={projectId === String(project.id)}
                  className="min-h-7 px-3 text-sm"
                >
                  <Link to={`/projects/${project.id}`}>
                    <span className="min-w-0 flex-1 truncate">
                      {project.name}
                    </span>
                    {/* UI-SPEC: chip is static (never pulses) and not
                        independently clickable — the row Link is the action. */}
                    {count > 0 && (
                      <span
                        aria-label={
                          count === 1
                            ? "1 agent waiting for input"
                            : `${count} agents waiting for input`
                        }
                        className="ml-auto inline-flex min-w-[18px] shrink-0 items-center justify-center rounded-full bg-amber-400/10 px-1 text-xs font-medium text-amber-400 tabular-nums"
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

      <SidebarFooter className="flex-row items-center gap-2">
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

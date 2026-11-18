import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { ScrollArea } from "radix-ui";
import { Link, useLocation, useParams } from "react-router";
import { Plus, Settings, Activity } from "lucide-react";
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
  useSidebar,
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
import { WorkspaceSwitcher } from "@/components/sidebar/WorkspaceSwitcher";
import { NotificationProcessing } from "@/components/inbox/NotificationProcessing";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";

export function ProjectSidebar() {
  const { setOpenMobile, state, isMobile, openMobile } = useSidebar();
  const { data: projects } = useProjects();
  const { data: agentStatuses } = useAgentStatuses();
  const { activeWorkspaceId } = useActiveWorkspace();
  const { projectId } = useParams();
  const location = useLocation();
  const [addOpen, setAddOpen] = useState(false);
  const workspaceProjects = (projects ?? [])
    .filter((project) => project.workspace_id === activeWorkspaceId)
    .sort((a, b) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)));
  const pinnedCount = workspaceProjects.filter((project) => project.pinned).length;
  const activeProjectRef = useRef<HTMLAnchorElement>(null);
  // The mobile sheet mounts its links after the parent's opening effect.
  const attachActiveProject = useCallback((node: HTMLAnchorElement | null) => {
    activeProjectRef.current = node;
    node?.scrollIntoView({ block: "nearest" });
  }, []);

  // Reveal the current project on initial load, navigation, and sidebar toggles.
  // Ordinary scrolling stays under the user's control between these changes.
  useEffect(() => {
    activeProjectRef.current?.scrollIntoView({ block: "nearest" });
  }, [projectId, activeWorkspaceId, projects?.length, state, isMobile, openMobile]);

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
      <SidebarHeader className="shrink-0 flex-row items-center justify-between group-data-[collapsible=icon]:justify-center">
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

      <SidebarContent className="overflow-hidden">
        {/* Keep the expanded-only workspace switcher outside the scrolling list. */}
        <div className="shrink-0 group-data-[collapsible=icon]:hidden">
          <WorkspaceSwitcher />
        </div>
        {/* An overlay scrollbar keeps overflow discoverable without narrowing
            the 48px icon rail or reducing its 40px project click targets. */}
        <ScrollArea.Root asChild type="auto">
          <nav aria-label="Projects" className="relative min-h-0 flex-1 overflow-hidden">
            {/* Radix uses a table for its content wrapper by default. A block
                keeps long project names constrained so row truncation works. */}
            <ScrollArea.Viewport className="size-full overscroll-contain [&>div]:block!">
              <SidebarMenu className="gap-1 px-2 py-1 group-data-[collapsible=icon]:items-center group-data-[collapsible=icon]:gap-1 group-data-[collapsible=icon]:px-0">
                {/* Both sidebar modes share workspace filtering and pinned-first
                    ordering. Preserve the API's name order within each group. */}
                {workspaceProjects.map((project, index) => {
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
                    <Fragment key={project.id}>
                      {pinnedCount > 0 && index === pinnedCount && (
                        <li role="separator" aria-label="Other projects" className="mx-3 my-2 self-stretch border-t border-sidebar-border group-data-[collapsible=icon]:mx-2" />
                      )}
                      <SidebarMenuItem>
                        <SidebarMenuButton
                          asChild
                          size="sm"
                          isActive={isActive}
                          tooltip={project.name}
                          className="min-h-7 px-3 text-sm transition-colors group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-md group-data-[collapsible=icon]:p-0!"
                        >
                          <Link
                            ref={isActive ? attachActiveProject : undefined}
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
                            <span className="hidden group-data-[collapsible=icon]:flex">
                              <ProjectAvatar
                                size="rail"
                                letters={project.icon_letters}
                                color={project.icon_color}
                                waiting={count > 0}
                                waitingLabel={waitingLabel}
                              />
                            </span>
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
                    </Fragment>
                  );
                })}
              </SidebarMenu>
            </ScrollArea.Viewport>
            <ScrollArea.Scrollbar orientation="vertical" className="flex w-1 touch-none select-none py-1">
              <ScrollArea.Thumb className="relative flex-1 rounded-full bg-sidebar-foreground/40 hover:bg-sidebar-foreground/60" />
            </ScrollArea.Scrollbar>
          </nav>
        </ScrollArea.Root>
        <div className="max-h-[40%] shrink-0 overflow-y-auto md:hidden">
          <NotificationProcessing onNavigate={() => setOpenMobile(false)} />
        </div>
      </SidebarContent>

      {/* Phase 11 D-02 wrinkle (Pitfall 1): the footer used to carry
          group-data-[collapsible=icon]:hidden which hid the WHOLE footer in
          the collapsed rail. Activity must stay reachable when collapsed, so
          the footer hide is removed and re-applied per-child: "Add project"
          and the Settings gear keep hiding (their existing behavior); the
          Activity button is left without it and additionally grows to a
          rail-appropriate target size when collapsed. */}
      <SidebarFooter className="shrink-0 flex-row items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          className="flex-1 justify-start group-data-[collapsible=icon]:hidden"
          onClick={() => setAddOpen(true)}
        >
          <Plus />
          Add project
        </Button>
        {/* Phase 11 — Activity entry (D-01/D-02/D-03). Sits next to the
            Settings gear in the expanded footer AND stays reachable as an
            icon in the collapsed rail (the project-row rail/expanded pair
            idiom). Active state derives from location.pathname === "/activity"
            — the Settings-gear idiom (D-01). */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="ghost"
              size="icon-sm"
              className={cn(
                "group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:rounded-md",
                location.pathname === "/activity" &&
                  "bg-sidebar-accent text-sidebar-accent-foreground",
              )}
            >
              <Link to="/activity" aria-label="Activity">
                <Activity className="size-4" />
              </Link>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Activity</TooltipContent>
        </Tooltip>
        {/* SET-01: gear → dedicated full-page /settings route. Active state
            reuses the selected-sidebar-item treatment from the project rows.
            Hides in the collapsed rail (the Activity button above takes the
            rail footer slot). */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="ghost"
              size="icon-sm"
              className={cn(
                "group-data-[collapsible=icon]:hidden",
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

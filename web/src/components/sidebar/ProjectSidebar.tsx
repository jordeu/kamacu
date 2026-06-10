import { useState } from "react";
import { Link, useParams } from "react-router";
import { Plus } from "lucide-react";
import { useProjects } from "@/api/queries";
import { Button } from "@/components/ui/button";
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
  const { projectId } = useParams();
  const [addOpen, setAddOpen] = useState(false);

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
          {(projects ?? []).map((project) => (
            <SidebarMenuItem key={project.id}>
              <SidebarMenuButton
                asChild
                size="sm"
                isActive={projectId === String(project.id)}
                className="min-h-7 px-3 text-sm"
              >
                <Link to={`/projects/${project.id}`}>
                  <span>{project.name}</span>
                </Link>
              </SidebarMenuButton>
              <ProjectMenu project={project} />
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarContent>

      <SidebarFooter>
        <Button
          variant="ghost"
          size="sm"
          className="justify-start"
          onClick={() => setAddOpen(true)}
        >
          <Plus />
          Add project
        </Button>
      </SidebarFooter>

      <AddProjectDialog open={addOpen} onOpenChange={setAddOpen} />
    </Sidebar>
  );
}

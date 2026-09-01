import { ChevronsUpDown } from "lucide-react";
import { useProjects, useWorkspaces } from "@/api/queries";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ActivityScope } from "@/lib/useActivityView";

/**
 * Phase 11 — the Activity scope selector (CONTEXT D-04). A SINGLE dropdown
 * mirroring the WorkspaceSwitcher shape: sectioned (Global / Workspaces /
 * Projects), one active item marked with ✓ via DropdownMenuCheckboxItem,
 * maps 1:1 to the `scope` query param (global | workspace:N | project:N).
 *
 * Selecting an item writes `kamacu.activity.scope` (via the onScopeChange
 * callback wired to useActivityView) and updates the useActivity query key —
 * one refetch, one loading state (Phase 10 D-01). No navigation (the route
 * stays `/activity`).
 */
function useDisplayName(scope: ActivityScope): string {
  const { data: workspaces } = useWorkspaces();
  const { data: projects } = useProjects();
  if (scope === "global") return "Global";
  if (scope.startsWith("workspace:")) {
    const id = Number(scope.slice("workspace:".length));
    return (workspaces ?? []).find((w) => w.id === id)?.name ?? "Global";
  }
  const id = Number(scope.slice("project:".length));
  return (projects ?? []).find((p) => p.id === id)?.name ?? "Global";
}

export function ScopeSelector({
  scope,
  onScopeChange,
}: {
  scope: ActivityScope;
  onScopeChange: (s: ActivityScope) => void;
}) {
  const { data: workspaces } = useWorkspaces();
  const { data: projects } = useProjects();
  const displayName = useDisplayName(scope);

  // Name-sorted so dropdown order is stable regardless of fetch order (mirrors
  // WorkspaceSwitcher.tsx:42-44). Projects arrive name-sorted from the backend
  // already, but sort defensively for the same stability guarantee.
  const sortedWorkspaces = [...(workspaces ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );
  const sortedProjects = [...(projects ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          aria-label={`Scope: ${displayName}`}
          className="gap-2"
        >
          <span className="max-w-[10rem] truncate">{displayName}</span>
          <ChevronsUpDown
            className="size-4 shrink-0 text-muted-foreground"
            aria-hidden
          />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="bottom" align="start">
        <DropdownMenuLabel>{`Global`}</DropdownMenuLabel>
        <DropdownMenuCheckboxItem
          checked={scope === "global"}
          onSelect={() => onScopeChange("global")}
        >
          {`All activity`}
        </DropdownMenuCheckboxItem>

        <DropdownMenuSeparator />
        <DropdownMenuLabel>{`Workspaces`}</DropdownMenuLabel>
        {sortedWorkspaces.map((ws) => (
          <DropdownMenuCheckboxItem
            key={ws.id}
            checked={scope === `workspace:${ws.id}`}
            onSelect={() => onScopeChange(`workspace:${ws.id}`)}
          >
            {ws.name}
          </DropdownMenuCheckboxItem>
        ))}

        <DropdownMenuSeparator />
        <DropdownMenuLabel>{`Projects`}</DropdownMenuLabel>
        {sortedProjects.map((p) => (
          <DropdownMenuCheckboxItem
            key={p.id}
            checked={scope === `project:${p.id}`}
            onSelect={() => onScopeChange(`project:${p.id}`)}
          >
            {p.name}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

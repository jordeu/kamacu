import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useProjects, useTasks } from "@/api/queries";
import { STATUSES } from "@/api/types";
import { Board } from "@/components/board/Board";
import { NewTaskDialog } from "@/components/board/NewTaskDialog";

export default function BoardPage() {
  const params = useParams();
  const projectId = Number(params.projectId);

  const { data: tasks, isLoading, isError, refetch } = useTasks(projectId);
  const { data: projects } = useProjects();
  const project = projects?.find((p) => p.id === projectId);

  const [dialogOpen, setDialogOpen] = useState(false);

  // "n" opens the New Task dialog — board page only, never while typing in an
  // input/textarea/contenteditable or while any dialog is open.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "n" || e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) {
        return;
      }
      const target = e.target;
      if (
        target instanceof HTMLElement &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable)
      ) {
        return;
      }
      if (document.querySelector('[role="dialog"]') !== null) return;
      e.preventDefault();
      setDialogOpen(true);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <h1 className="text-base font-medium">{project?.name ?? ""}</h1>
        {/* The only inverted high-contrast element on the page (UI-SPEC focal point). */}
        <Button onClick={() => setDialogOpen(true)}>New task</Button>
      </header>
      {isLoading ? (
        <div className="flex min-h-0 flex-1 gap-4 overflow-x-auto px-6 pb-6">
          {STATUSES.map((status) => (
            <div
              key={status}
              className="flex min-w-[260px] flex-1 flex-col gap-2 rounded-md bg-[#101013] p-3"
            >
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </div>
          ))}
        </div>
      ) : isError ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3">
          <p className="text-muted-foreground">Couldn't load tasks.</p>
          <Button variant="outline" onClick={() => refetch()}>
            Retry loading
          </Button>
        </div>
      ) : (
        <Board tasks={tasks ?? []} projectId={projectId} />
      )}
      <NewTaskDialog
        projectId={projectId}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </div>
  );
}

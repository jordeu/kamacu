import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, Ellipsis } from "lucide-react";
import { useTask } from "@/api/queries";
import { useUpdateTask } from "@/api/mutations";
import { useCreateWorktree } from "@/api/worktrees";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { DeleteTaskDialog } from "@/components/task/DeleteTaskDialog";
import { DescriptionTab } from "@/components/task/DescriptionTab";
import { TaskTabs } from "@/components/task/TaskTabs";
import { WorktreeMetaLine } from "@/components/task/WorktreeMetaLine";

function isTypingTarget(el: Element | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return (
    el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable
  );
}

export default function TaskPage() {
  const { projectId: projectIdParam, taskId: taskIdParam } = useParams();
  const projectId = Number(projectIdParam);
  const taskId = Number(taskIdParam);
  const navigate = useNavigate();

  // Fetch by id so deep links work without the board cache (TASK-05).
  const { data: task, isPending, isError } = useTask(taskId);
  const updateTask = useUpdateTask(projectId);
  const createWorktree = useCreateWorktree(taskId, projectId);

  const [titleDraft, setTitleDraft] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const cancelTitleEditRef = useRef(false);

  // Esc returns to the board (D-07) — always the explicit board route, never
  // history-back, because deep-linked tabs have no history. Suppressed while
  // typing in inputs/textareas/contenteditable or while any Radix dialog/menu
  // is open.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      if (isTypingTarget(document.activeElement)) return;
      if (e.target instanceof Element && isTypingTarget(e.target)) return;
      if (
        document.querySelector(
          '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"]',
        )
      ) {
        return;
      }
      navigate(`/projects/${projectId}`);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [navigate, projectId]);

  if (isPending) {
    return (
      <div className="mx-auto w-full max-w-[860px] space-y-4 p-4">
        <Skeleton className="h-7 w-2/3" />
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-3/4" />
      </div>
    );
  }

  if (isError || !task) {
    return (
      <div className="mx-auto w-full max-w-[860px] space-y-3 p-4">
        <p className="text-muted-foreground">Task not found.</p>
        <Link
          to={`/projects/${projectId}`}
          className="text-sm underline underline-offset-4 hover:text-foreground"
        >
          Back to board
        </Link>
      </div>
    );
  }

  function commitTitle(value: string) {
    const cancelled = cancelTitleEditRef.current;
    cancelTitleEditRef.current = false;
    setTitleDraft(null);
    if (cancelled || !task) return;
    const trimmed = value.trim();
    // Trimmed-empty reverts without saving (TASK-04).
    if (!trimmed || trimmed === task.title) return;
    updateTask.mutate({ id: task.id, title: trimmed });
  }

  return (
    // Full-width working surface (Layout Contract): terminals get the whole
    // main area; header/meta and Description prose keep 860px islands.
    <div className="w-full space-y-6 p-4">
      <div className="max-w-[860px] space-y-2">
        <header className="flex items-center gap-2">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Back to board"
                  onClick={() => navigate(`/projects/${projectId}`)}
                >
                  <ArrowLeft className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Back to board (Esc)</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          {titleDraft === null ? (
            <button
              type="button"
              className="min-w-0 flex-1 truncate rounded-md px-1 py-0.5 text-left text-base font-medium hover:bg-muted/50"
              title="Edit title"
              onClick={() => setTitleDraft(task.title)}
            >
              {task.title}
            </button>
          ) : (
            <Input
              autoFocus
              value={titleDraft}
              className="h-8 flex-1 text-base font-medium"
              onChange={(e) => setTitleDraft(e.target.value)}
              onBlur={(e) => commitTitle(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.currentTarget.blur();
                } else if (e.key === "Escape") {
                  cancelTitleEditRef.current = true;
                  e.currentTarget.blur();
                }
              }}
            />
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Task actions">
                <Ellipsis className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => setDeleteOpen(true)}
              >
                Delete task
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>

        <WorktreeMetaLine
          task={task}
          onCreate={() => createWorktree.mutate()}
          creating={createWorktree.isPending}
        />
      </div>

      <TaskTabs
        tabs={[
          {
            id: "description",
            label: "Description",
            content: (
              <div className="max-w-[860px]">
                <DescriptionTab task={task} projectId={projectId} />
              </div>
            ),
          },
        ]}
      />

      <DeleteTaskDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        taskId={task.id}
        taskTitle={task.title}
        projectId={projectId}
      />
    </div>
  );
}

import { useTask } from "@/api/queries";
import { useMoveTask } from "@/api/mutations";
import { STATUS_LABELS, type Status } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle,
} from "@/components/ui/dialog";

export function AgentExitDialog({ taskId, projectId, onClose }: {
  taskId: number;
  projectId: number;
  onClose: () => void;
}) {
  const { data: task, isFetchedAfterMount } = useTask(taskId);
  const move = useMoveTask(projectId);
  // Read the current status after navigation, including moves made by the
  // agent during its final turn. TODO/Done and PR reviews need no prompt.
  const open = isFetchedAfterMount && task?.project_id === projectId &&
    task.source === "manual" &&
    (task.status === "in_progress" || task.status === "in_review");

  function moveTo(status: Status) {
    move.mutate({ id: taskId, status, afterId: null }, { onSuccess: onClose });
  }

  return (
    <Dialog open={open} onOpenChange={(value) => { if (!value) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Agent session ended</DialogTitle>
          <DialogDescription className="break-words">
            Update the status of “{task?.title}” or keep it in {task ? STATUS_LABELS[task.status] : "its current column"}.
          </DialogDescription>
        </DialogHeader>
        {move.isError && (
          <p role="alert" className="text-sm text-destructive">
            Couldn't move the task. Try again or keep its current status.
          </p>
        )}
        <DialogFooter className="flex-wrap">
          <Button variant="ghost" disabled={move.isPending} onClick={onClose}>
            Keep {task ? STATUS_LABELS[task.status] : "current status"}
          </Button>
          {task?.status === "in_progress" && (
            <Button variant="outline" disabled={move.isPending} onClick={() => moveTo("done")}>
              Move to Done
            </Button>
          )}
          <Button disabled={move.isPending} onClick={() => moveTo(task?.status === "in_progress" ? "in_review" : "done")}>
            {move.isPending ? "Moving…" : task?.status === "in_progress" ? "Move to In review" : "Move to Done"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
